package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"kafka-go-study/internal/kafka"
	"kafka-go-study/internal/models"
	"kafka-go-study/internal/telemetry"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const (
	orderTopic   = "orders"
	paymentTopic = "payments"
	groupID      = "order-processor"
)

func brokerAddr() string {
	if b := os.Getenv("KAFKA_BROKER"); b != "" {
		return b
	}
	return "localhost:9092"
}

func paymentAPIAddr() string {
	if a := os.Getenv("PAYMENT_API_ADDR"); a != "" {
		return a
	}
	return "http://localhost:8081"
}

var (
	log             *zap.Logger
	paymentProducer *kafka.Producer
	metrics         *telemetry.Metrics
	tracer          trace.Tracer
	httpClient      *http.Client
	propagator      propagation.TextMapPropagator
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		meter    metric.Meter
		shutdown func(context.Context)
		err      error
	)

	log, tracer, meter, shutdown, err = telemetry.Setup(ctx, "consumer")
	if err != nil {
		panic("failed to initialize telemetry: " + err.Error())
	}
	defer shutdown(context.Background())

	metrics, err = telemetry.NewMetrics(meter)
	if err != nil {
		panic("failed to create metrics: " + err.Error())
	}

	httpClient = &http.Client{Timeout: 5 * time.Second}
	propagator = propagation.TraceContext{}

	addr := brokerAddr()

	if err := kafka.CreateTopic(ctx, addr, paymentTopic, 3, 1); err != nil {
		log.Warn("failed to create topic payments (may already exist)", zap.Error(err))
	}

	paymentProducer = kafka.NewProducer([]string{addr}, paymentTopic)
	defer paymentProducer.Close()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Info("shutting down consumer...")
		cancel()
	}()

	orderConsumer := kafka.NewConsumer([]string{addr}, orderTopic, groupID)
	defer orderConsumer.Close()

	paymentConsumer := kafka.NewConsumer([]string{addr}, paymentTopic, "payment-processor")
	defer paymentConsumer.Close()

	log.Info("consumers started",
		zap.String("order_topic", orderTopic),
		zap.String("payment_topic", paymentTopic),
	)

	go func() {
		if err := paymentConsumer.Listen(ctx, processPayment); err != nil {
			log.Error("payment consumer error", zap.Error(err))
		}
	}()

	if err := orderConsumer.Listen(ctx, processOrder); err != nil {
		log.Error("order consumer error", zap.Error(err))
	}
}

func processOrder(ctx context.Context, key, value []byte) error {
	ctx, span := tracer.Start(ctx, "ProcessOrder",
		trace.WithSpanKind(trace.SpanKindConsumer),
	)
	defer span.End()

	var order models.Order
	if err := json.Unmarshal(value, &order); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to unmarshal order")
		return err
	}

	span.SetAttributes(
		attribute.String("order.id", order.ID),
		attribute.String("order.customer_id", order.CustomerID),
		attribute.Int64("order.total_cents", order.TotalCents),
	)

	bag := baggage.FromContext(ctx)
	customerID := order.CustomerID
	if cid := bag.Member("customer_id").Value(); cid != "" {
		customerID = cid
	}

	_, sleepSpan := tracer.Start(ctx, "SimulateProcessingDelay")
	delay := time.Duration(50+rand.IntN(451)) * time.Millisecond
	sleepSpan.SetAttributes(attribute.Int64("delay_ms", delay.Milliseconds()))
	time.Sleep(delay)
	sleepSpan.End()

	attrs := metric.WithAttributes(attribute.String("customer_id", customerID))
	metrics.MessagesConsumed.Add(ctx, 1, attrs)
	metrics.ProcessingTime.Record(ctx, delay.Seconds(), attrs)

	payment := models.Payment{
		OrderID:     order.ID,
		CustomerID:  customerID,
		TotalCents:  order.TotalCents,
		Status:      "confirmed",
		ConfirmedAt: time.Now(),
	}

	if err := paymentProducer.Publish(ctx, order.ID, payment); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetStatus(codes.Ok, "")
	log.Info("order processed, payment published",
		zap.String("order_id", order.ID),
		zap.String("customer_id", customerID),
		zap.Int64("total_cents", order.TotalCents),
		zap.Duration("processing_delay", delay),
	)

	return nil
}

func processPayment(ctx context.Context, key, value []byte) error {
	ctx, span := tracer.Start(ctx, "ProcessPayment",
		trace.WithSpanKind(trace.SpanKindConsumer),
	)
	defer span.End()

	var payment models.Payment
	if err := json.Unmarshal(value, &payment); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to unmarshal payment")
		return err
	}

	span.SetAttributes(
		attribute.String("payment.order_id", payment.OrderID),
		attribute.String("payment.customer_id", payment.CustomerID),
		attribute.Int64("payment.total_cents", payment.TotalCents),
	)

	body, _ := json.Marshal(map[string]any{
		"order_id":    payment.OrderID,
		"customer_id": payment.CustomerID,
		"total_cents": payment.TotalCents,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		paymentAPIAddr()+"/payments/confirm", bytes.NewReader(body))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	propagator.Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("payment-api returned %d", resp.StatusCode)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
	span.SetStatus(codes.Ok, "")
	log.Info("payment confirmed via payment-api",
		zap.String("order_id", payment.OrderID),
		zap.String("customer_id", payment.CustomerID),
	)

	return nil
}
