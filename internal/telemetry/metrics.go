package telemetry

import (
	"go.opentelemetry.io/otel/metric"
)

type Metrics struct {
	MessagesPublished metric.Int64Counter
	MessagesConsumed  metric.Int64Counter
	ProcessingTime    metric.Float64Histogram

	OrdersCreated     metric.Int64Counter
	PaymentsConfirmed metric.Int64Counter
	OrderValueCents   metric.Int64Histogram
}

func NewMetrics(meter metric.Meter) (*Metrics, error) {
	published, err := meter.Int64Counter("messages_published_total",
		metric.WithDescription("Total messages published to Kafka"),
		metric.WithUnit("{message}"),
	)
	if err != nil {
		return nil, err
	}

	consumed, err := meter.Int64Counter("messages_consumed_total",
		metric.WithDescription("Total messages consumed from Kafka"),
		metric.WithUnit("{message}"),
	)
	if err != nil {
		return nil, err
	}

	procTime, err := meter.Float64Histogram("message_processing_duration_seconds",
		metric.WithDescription("Duration of message processing"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.05, 0.1, 0.2, 0.3, 0.5, 1.0),
	)
	if err != nil {
		return nil, err
	}

	ordersCreated, err := meter.Int64Counter("orders_created_total",
		metric.WithDescription("Total orders created"),
		metric.WithUnit("{order}"),
	)
	if err != nil {
		return nil, err
	}

	paymentsConfirmed, err := meter.Int64Counter("payments_confirmed_total",
		metric.WithDescription("Total payments confirmed"),
		metric.WithUnit("{payment}"),
	)
	if err != nil {
		return nil, err
	}

	orderValue, err := meter.Int64Histogram("order_value_cents",
		metric.WithDescription("Order value in cents"),
		metric.WithUnit("cents"),
		metric.WithExplicitBucketBoundaries(100, 500, 1000, 5000, 10000, 50000),
	)
	if err != nil {
		return nil, err
	}

	return &Metrics{
		MessagesPublished: published,
		MessagesConsumed:  consumed,
		ProcessingTime:    procTime,
		OrdersCreated:     ordersCreated,
		PaymentsConfirmed: paymentsConfirmed,
		OrderValueCents:   orderValue,
	}, nil
}
