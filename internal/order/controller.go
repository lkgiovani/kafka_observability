package order

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/baggage"
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

type createOrderRequest struct {
	CustomerID string   `json:"customer_id"`
	Items      []string `json:"items"`
	TotalCents int64    `json:"total_cents"`
}

func (ct *Controller) Create(c *fiber.Ctx) error {
	ctx, span := ct.tracer.Start(c.UserContext(), "Controller.CreateOrder",
		trace.WithSpanKind(trace.SpanKindServer),
	)
	defer span.End()

	var req createOrderRequest
	if err := c.BodyParser(&req); err != nil {
		span.SetStatus(codes.Error, "invalid body")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	if req.CustomerID == "" || len(req.Items) == 0 || req.TotalCents <= 0 {
		span.SetStatus(codes.Error, "missing required fields")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "customer_id, items and total_cents are required"})
	}

	member, _ := baggage.NewMember("customer_id", req.CustomerID)
	bag, _ := baggage.New(member)
	ctx = baggage.ContextWithBaggage(ctx, bag)

	order, err := ct.useCase.PlaceOrder(ctx, req.CustomerID, req.Items, req.TotalCents)
	if err != nil {
		if errors.Is(err, ErrPaymentDeclined) {
			span.SetStatus(codes.Error, "payment declined")
			ct.log.Warn("payment declined", zap.String("customer_id", req.CustomerID))
			return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{"error": "payment declined"})
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		ct.log.Error("failed to place order", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}

	span.SetStatus(codes.Ok, "")
	return c.Status(fiber.StatusCreated).JSON(order)
}
