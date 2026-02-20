package main

import (
	"context"
	"kafka-go-study/internal/kafka"
	"kafka-go-study/internal/order"
	"kafka-go-study/internal/telemetry"
	"os"
	"os/signal"
	"syscall"

	"github.com/gofiber/contrib/otelfiber"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

func brokerAddr() string {
	if b := os.Getenv("KAFKA_BROKER"); b != "" {
		return b
	}
	return "localhost:9092"
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log, tracer, meter, shutdown, err := telemetry.Setup(ctx, "order-api")
	if err != nil {
		panic("failed to initialize telemetry: " + err.Error())
	}
	defer shutdown(context.Background())

	metrics, err := telemetry.NewMetrics(meter)
	if err != nil {
		panic("failed to create metrics: " + err.Error())
	}

	addr := brokerAddr()
	if err := kafka.CreateTopic(ctx, addr, "orders", 3, 1); err != nil {
		log.Warn("failed to create topic orders (may already exist)", zap.Error(err))
	}

	producer := kafka.NewProducer([]string{addr}, "orders")
	defer producer.Close()

	uc := order.NewUseCase(producer, metrics, log, tracer)
	ctrl := order.NewController(uc, log, tracer)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(otelfiber.Middleware())

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})
	app.Post("/orders", ctrl.Create)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Info("shutting down order-api...")
		_ = app.Shutdown()
		cancel()
	}()

	log.Info("order-api listening", zap.String("addr", ":8080"))
	if err := app.Listen(":8080"); err != nil {
		log.Error("server error", zap.Error(err))
	}
}
