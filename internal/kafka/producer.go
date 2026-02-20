package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/segmentio/kafka-go"
)

type Producer struct {
	writer *kafka.Writer
	topic  string
	tracer trace.Tracer
}

func NewProducer(brokers []string, topic string) *Producer {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		WriteTimeout: 10 * time.Second,
		ReadTimeout:  10 * time.Second,
		RequiredAcks: kafka.RequireOne,
	}

	return &Producer{
		writer: writer,
		topic:  topic,
		tracer: otel.Tracer("kafka/producer"),
	}
}

func (p *Producer) Publish(ctx context.Context, key string, value any) error {
	ctx, span := p.tracer.Start(ctx, fmt.Sprintf("publish %s", p.topic),
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			semconv.MessagingSystemKafka,
			semconv.MessagingDestinationName(p.topic),
			attribute.String("messaging.kafka.message.key", key),
		),
	)
	defer span.End()

	data, err := json.Marshal(value)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	headers := make([]kafka.Header, 0)
	otel.GetTextMapPropagator().Inject(ctx, &kafkaHeaderCarrier{headers: &headers})

	msg := kafka.Message{
		Key:     []byte(key),
		Value:   data,
		Time:    time.Now(),
		Headers: headers,
	}

	if err := p.writer.WriteMessages(ctx, msg); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("failed to publish message: %w", err)
	}

	span.SetStatus(codes.Ok, "")
	return nil
}

func (p *Producer) Close() error {
	return p.writer.Close()
}

type kafkaHeaderCarrier struct {
	headers *[]kafka.Header
}

func (c *kafkaHeaderCarrier) Get(key string) string {
	for _, h := range *c.headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

func (c *kafkaHeaderCarrier) Set(key, value string) {
	*c.headers = append(*c.headers, kafka.Header{Key: key, Value: []byte(value)})
}

func (c *kafkaHeaderCarrier) Keys() []string {
	keys := make([]string, len(*c.headers))
	for i, h := range *c.headers {
		keys[i] = h.Key
	}
	return keys
}

func init() {
	otel.SetTextMapPropagator(propagation.TraceContext{})
}
