package order

import (
	"context"
	"errors"
	"kafka-go-study/internal/kafka"
	"kafka-go-study/internal/models"
	"kafka-go-study/internal/telemetry"
	"math/rand/v2"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/google/uuid"
)

var ErrPaymentDeclined = errors.New("payment declined")

type UseCase struct {
	producer *kafka.Producer
	metrics  *telemetry.Metrics
	log      *zap.Logger
	tracer   trace.Tracer
}

func NewUseCase(producer *kafka.Producer, metrics *telemetry.Metrics, log *zap.Logger, tracer trace.Tracer) *UseCase {
	return &UseCase{producer: producer, metrics: metrics, log: log, tracer: tracer}
}

func (uc *UseCase) PlaceOrder(ctx context.Context, customerID string, items []string, totalCents int64) (*models.Order, error) {
	ctx, span := uc.tracer.Start(ctx, "PlaceOrder",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("order.customer_id", customerID),
			attribute.Int64("order.total_cents", totalCents),
			attribute.Int("order.items_count", len(items)),
		),
	)
	defer span.End()

	_, validateSpan := uc.tracer.Start(ctx, "ValidatePayment")
	declined := rand.Float64() < 0.2
	validateSpan.SetAttributes(attribute.Bool("payment.declined", declined))
	if declined {
		validateSpan.SetStatus(codes.Error, "payment declined")
		validateSpan.End()
		span.SetStatus(codes.Error, "payment declined")
		uc.metrics.OrdersCreated.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "declined")))
		return nil, ErrPaymentDeclined
	}
	validateSpan.SetStatus(codes.Ok, "")
	validateSpan.End()

	order := &models.Order{
		ID:         uuid.NewString(),
		CustomerID: customerID,
		Items:      items,
		TotalCents: totalCents,
		CreatedAt:  time.Now(),
	}
	span.SetAttributes(attribute.String("order.id", order.ID))

	if err := uc.producer.Publish(ctx, order.ID, order); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		uc.metrics.OrdersCreated.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
		return nil, err
	}

	uc.metrics.OrdersCreated.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "ok")))
	uc.metrics.OrderValueCents.Record(ctx, totalCents)

	span.SetStatus(codes.Ok, "")
	uc.log.Info("order placed",
		zap.String("order_id", order.ID),
		zap.String("customer_id", customerID),
		zap.Int64("total_cents", totalCents),
	)

	return order, nil
}
