package kafka

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/segmentio/kafka-go"
)

type HandlerFunc func(ctx context.Context, key, value []byte) error

type Consumer struct {
	reader  *kafka.Reader
	groupID string
	topic   string
	tracer  trace.Tracer
}

func NewConsumer(brokers []string, topic, groupID string) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        brokers,
		Topic:          topic,
		GroupID:        groupID,
		MinBytes:       1,
		MaxBytes:       10e6,
		CommitInterval: time.Second,
		StartOffset:    kafka.FirstOffset,
	})

	return &Consumer{
		reader:  reader,
		groupID: groupID,
		topic:   topic,
		tracer:  otel.Tracer("kafka/consumer"),
	}
}

func (c *Consumer) Listen(ctx context.Context, handler HandlerFunc) error {
	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("failed to fetch message: %w", err)
		}

		carrier := &kafkaHeaderCarrier{headers: &msg.Headers}
		msgCtx := otel.GetTextMapPropagator().Extract(ctx, carrier)

		msgCtx, span := c.tracer.Start(msgCtx, fmt.Sprintf("receive %s", c.topic),
			trace.WithSpanKind(trace.SpanKindConsumer),
			trace.WithAttributes(
				semconv.MessagingSystemKafka,
				semconv.MessagingDestinationName(c.topic),
				attribute.String("messaging.kafka.message.key", string(msg.Key)),
				attribute.Int("messaging.kafka.partition", msg.Partition),
				attribute.Int64("messaging.kafka.offset", msg.Offset),
				attribute.String("messaging.kafka.consumer.group", c.groupID),
			),
		)

		if err := handler(msgCtx, msg.Key, msg.Value); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.End()
			continue
		}

		span.SetStatus(codes.Ok, "")
		span.End()

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			return fmt.Errorf("failed to commit offset: %w", err)
		}
	}
}

func (c *Consumer) Close() error {
	return c.reader.Close()
}
