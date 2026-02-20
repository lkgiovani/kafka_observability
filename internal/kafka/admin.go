package kafka

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/segmentio/kafka-go"
)

func CreateTopic(ctx context.Context, broker, topic string, partitions, replication int) error {
	conn, err := kafka.DialContext(ctx, "tcp", broker)
	if err != nil {
		return fmt.Errorf("failed to connect to broker: %w", err)
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("failed to get controller: %w", err)
	}

	controllerConn, err := kafka.DialContext(ctx, "tcp", net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		return fmt.Errorf("failed to connect to controller: %w", err)
	}
	defer controllerConn.Close()

	err = controllerConn.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     partitions,
		ReplicationFactor: replication,
	})
	if err != nil {
		return fmt.Errorf("failed to create topic: %w", err)
	}

	return nil
}
