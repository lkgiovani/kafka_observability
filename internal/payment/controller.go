package payment

import (
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Controller struct {
	useCase *UseCase
	log     *zap.Logger
	tracer  trace.Tracer
}

func NewController(useCase *UseCase, log *zap.Logger, tracer trace.Tracer) *Controller {
	return &Controller{useCase: useCase, log: log, tracer: tracer}
}

type confirmPaymentRequest struct {
	OrderID    string `json:"order_id"`
	CustomerID string `json:"customer_id"`
	TotalCents int64  `json:"total_cents"`
}

func (ct *Controller) Confirm(c *fiber.Ctx) error {
	ctx, span := ct.tracer.Start(c.UserContext(), "Controller.ConfirmPayment",
		trace.WithSpanKind(trace.SpanKindServer),
	)
	defer span.End()

	var req confirmPaymentRequest
	if err := c.BodyParser(&req); err != nil {
		span.SetStatus(codes.Error, "invalid body")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	if req.OrderID == "" {
		span.SetStatus(codes.Error, "order_id is required")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "order_id is required"})
	}

	payment, err := ct.useCase.ConfirmPayment(ctx, req.OrderID, req.CustomerID, req.TotalCents)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		ct.log.Error("failed to confirm payment", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}

	span.SetStatus(codes.Ok, "")
	return c.Status(fiber.StatusOK).JSON(payment)
}
