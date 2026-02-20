package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"kafka-go-study/internal/telemetry"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
)

var customers = []string{"alice", "bob", "carol", "dave", "eve"}
var items = [][]string{
	{"notebook", "pen"},
	{"keyboard", "mouse"},
	{"headphones"},
	{"monitor", "hdmi-cable", "desk-lamp"},
	{"coffee-mug"},
}

func orderAPIAddr() string {
	if v := os.Getenv("ORDER_API_ADDR"); v != "" {
		return v
	}
	return "http://localhost:8080"
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log, _, _, shutdown, err := telemetry.Setup(ctx, "load-gen")
	if err != nil {
		panic("failed to initialize telemetry: " + err.Error())
	}
	defer shutdown(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Info("shutting down load-gen...")
		cancel()
	}()

	interval := 2 * time.Second
	if v := os.Getenv("INTERVAL_MS"); v != "" {
		if ms, err := time.ParseDuration(v + "ms"); err == nil {
			interval = ms
		}
	}

	addr := orderAPIAddr()
	client := &http.Client{Timeout: 5 * time.Second}

	log.Info("load-gen started",
		zap.String("target", addr),
		zap.Duration("interval", interval),
	)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			placeOrder(ctx, client, addr, log)
		}
	}
}

func placeOrder(ctx context.Context, client *http.Client, addr string, log *zap.Logger) {
	i := rand.IntN(len(customers))
	customerID := customers[i]
	selectedItems := items[i%len(items)]
	totalCents := int64(500 + rand.IntN(49500))

	body, _ := json.Marshal(map[string]any{
		"customer_id": customerID,
		"items":       selectedItems,
		"total_cents": totalCents,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, addr+"/orders", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Warn("request failed", zap.Error(err))
		return
	}
	defer resp.Body.Close()

	status := "ok"
	if resp.StatusCode == http.StatusPaymentRequired {
		status = "declined"
	} else if resp.StatusCode >= 500 {
		status = "error"
	}

	log.Info("order sent",
		zap.String("customer_id", customerID),
		zap.Int64("total_cents", totalCents),
		zap.String("status", status),
		zap.Int("http_status", resp.StatusCode),
		zap.String("items", fmt.Sprintf("%v", selectedItems)),
	)
}
