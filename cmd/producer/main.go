package main

import (
	"context"
	"fmt"
	"kafka-go-study/internal/kafka"
	"kafka-go-study/internal/models"
	"kafka-go-study/internal/telemetry"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const topic = "events"

func brokerAddr() string {
	if b := os.Getenv("KAFKA_BROKER"); b != "" {
		return b
	}
	return "localhost:9092"
}

var (
	log     *zap.Logger
	tracer  trace.Tracer
	metrics *telemetry.Metrics
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		meter    metric.Meter
		shutdown func(context.Context)
		err      error
	)

	log, tracer, meter, shutdown, err = telemetry.Setup(ctx, "producer")
	if err != nil {
		panic("failed to initialize telemetry: " + err.Error())
	}
	defer shutdown(context.Background())

	metrics, err = telemetry.NewMetrics(meter)
	if err != nil {
		panic("failed to create metrics: " + err.Error())
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Info("shutting down producer...")
		cancel()
	}()

	addr := brokerAddr()

	if err := kafka.CreateTopic(ctx, addr, topic, 3, 1); err != nil {
		log.Warn("failed to create topic (may already exist)", zap.Error(err))
	}

	producer := kafka.NewProducer([]string{addr}, topic)
	defer producer.Close()

	log.Info("producer started", zap.String("broker", addr), zap.String("topic", topic))

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	eventTypes := []string{"user.created", "order.placed", "payment.processed"}
	i := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			publishEvent(ctx, producer, eventTypes[i%len(eventTypes)], i)
			i++
		}
	}
}

func publishEvent(ctx context.Context, producer *kafka.Producer, eventType string, i int) {
	ctx, span := tracer.Start(ctx, "publish-event")
	defer span.End()

	event := models.Event{
		ID:        uuid.NewString(),
		Type:      eventType,
		Payload:   fmt.Sprintf(`{"index": %d, "info": "sample data"}`, i),
		CreatedAt: time.Now(),
	}

	if err := producer.Publish(ctx, event.ID, event); err != nil {
		log.Error("failed to publish message", zap.Error(err))
		return
	}

	metrics.MessagesPublished.Add(ctx, 1,
		metric.WithAttributes(attribute.String("event_type", eventType)),
	)

	log.Info("message published",
		zap.String("id", event.ID),
		zap.String("type", event.Type),
	)
}
