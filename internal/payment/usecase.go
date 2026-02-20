package payment

import (
	"context"
	"kafka-go-study/internal/models"
	"kafka-go-study/internal/telemetry"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type UseCase struct {
	metrics *telemetry.Metrics
	log     *zap.Logger
	tracer  trace.Tracer
}

func NewUseCase(metrics *telemetry.Metrics, log *zap.Logger, tracer trace.Tracer) *UseCase {
	return &UseCase{metrics: metrics, log: log, tracer: tracer}
}

func (uc *UseCase) ConfirmPayment(ctx context.Context, orderID string, customerID string, totalCents int64) (*models.Payment, error) {
	ctx, span := uc.tracer.Start(ctx, "ConfirmPayment",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	bag := baggage.FromContext(ctx)
	if cid := bag.Member("customer_id").Value(); cid != "" {
		customerID = cid
	}

	span.SetAttributes(
		attribute.String("payment.order_id", orderID),
		attribute.String("payment.customer_id", customerID),
		attribute.Int64("payment.total_cents", totalCents),
		attribute.String("payment.status", "confirmed"),
	)

	payment := &models.Payment{
		OrderID:     orderID,
		CustomerID:  customerID,
		TotalCents:  totalCents,
		Status:      "confirmed",
		ConfirmedAt: time.Now(),
	}

	uc.metrics.PaymentsConfirmed.Add(ctx, 1)
	span.SetStatus(codes.Ok, "")

	uc.log.Info("payment confirmed",
		zap.String("order_id", orderID),
		zap.String("customer_id", customerID),
		zap.Int64("total_cents", totalCents),
	)

	return payment, nil
}
