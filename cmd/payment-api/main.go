package main

import (
	"context"
	"kafka-go-study/internal/payment"
	"kafka-go-study/internal/telemetry"
	"os"
	"os/signal"
	"syscall"

	"github.com/gofiber/contrib/otelfiber"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log, tracer, meter, shutdown, err := telemetry.Setup(ctx, "payment-api")
	if err != nil {
		panic("failed to initialize telemetry: " + err.Error())
	}
	defer shutdown(context.Background())

	metrics, err := telemetry.NewMetrics(meter)
	if err != nil {
		panic("failed to create metrics: " + err.Error())
	}

	uc := payment.NewUseCase(metrics, log, tracer)
	ctrl := payment.NewController(uc, log, tracer)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(otelfiber.Middleware())

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})
	app.Post("/payments/confirm", ctrl.Confirm)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Info("shutting down payment-api...")
		_ = app.Shutdown()
		cancel()
	}()

	log.Info("payment-api listening", zap.String("addr", ":8081"))
	if err := app.Listen(":8081"); err != nil {
		log.Error("server error", zap.Error(err))
	}
}
