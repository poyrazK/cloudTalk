package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/IBM/sarama"
	"github.com/poyrazk/cloudtalk/internal/metrics"
)

const (
	TopicRoomMessages = "chat.room.messages"
	TopicDMMessages   = "chat.dm.messages"
	TopicPresence     = "chat.presence"
)

// ChatEvent is the message envelope published to Kafka.
type ChatEvent struct {
	Type     string          `json:"type"` // "message" | "dm" | "typing" | "typing_dm" | "presence"
	RoomID   string          `json:"room_id,omitempty"`
	SenderID string          `json:"sender_id,omitempty"`
	ToUserID string          `json:"to_user_id,omitempty"`
	Payload  json.RawMessage `json:"payload"`
}

// Producer wraps a Sarama sync producer.
type Producer struct {
	producer sarama.SyncProducer
	client   sarama.Client
}

func NewProducer(brokers []string) (*Producer, error) {
	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = true
	cfg.Producer.RequiredAcks = sarama.WaitForLocal
	cfg.Producer.Compression = sarama.CompressionSnappy

	client, err := sarama.NewClient(brokers, cfg)
	if err != nil {
		return nil, fmt.Errorf("kafka client: %w", err)
	}
	p, err := sarama.NewSyncProducerFromClient(client)
	if err != nil {
		if closeErr := client.Close(); closeErr != nil {
			slog.Error("close kafka client after producer init failure", "err", closeErr)
		}
		return nil, fmt.Errorf("kafka producer: %w", err)
	}
	return &Producer{producer: p, client: client}, nil
}

// Publish sends a ChatEvent to the given topic using key as the partition key.
func (p *Producer) Publish(topic, key string, evt ChatEvent) error {
	start := time.Now()
	data, err := json.Marshal(evt)
	if err != nil {
		metrics.KafkaPublishTotal.WithLabelValues(topic, "error").Inc()
		metrics.KafkaPublishDuration.WithLabelValues(topic, "error").Observe(time.Since(start).Seconds())
		return fmt.Errorf("marshal chat event: %w", err)
	}
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.ByteEncoder(data),
	}
	_, _, err = p.producer.SendMessage(msg)
	if err != nil {
		metrics.KafkaPublishTotal.WithLabelValues(topic, "error").Inc()
		metrics.KafkaPublishDuration.WithLabelValues(topic, "error").Observe(time.Since(start).Seconds())
		return fmt.Errorf("send kafka message: %w", err)
	}
	metrics.KafkaPublishTotal.WithLabelValues(topic, "ok").Inc()
	metrics.KafkaPublishDuration.WithLabelValues(topic, "ok").Observe(time.Since(start).Seconds())
	return nil
}

// Close gracefully shuts down the producer.
func (p *Producer) Close() error {
	if err := p.producer.Close(); err != nil {
		return fmt.Errorf("close producer: %w", err)
	}
	if err := p.client.Close(); err != nil {
		return fmt.Errorf("close producer client: %w", err)
	}
	return nil
}

func (p *Producer) Ping(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- p.client.RefreshMetadata()
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("ping kafka producer: %w", ctx.Err())
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("ping kafka producer: %w", err)
		}
		return nil
	}
}

// --- Consumer ---

// MessageHandler is called for each Kafka message.
type MessageHandler func(topic string, evt ChatEvent)

// Consumer wraps a Sarama consumer group.
type Consumer struct {
	group   sarama.ConsumerGroup
	topics  []string
	handler MessageHandler
}

func NewConsumer(brokers []string, groupID string, topics []string, handler MessageHandler) (*Consumer, error) {
	cfg := sarama.NewConfig()
	cfg.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}
	cfg.Consumer.Offsets.Initial = sarama.OffsetNewest

	g, err := sarama.NewConsumerGroup(brokers, groupID, cfg)
	if err != nil {
		return nil, fmt.Errorf("kafka consumer: %w", err)
	}
	return &Consumer{group: g, topics: topics, handler: handler}, nil
}

// Start begins consuming in a background goroutine. Cancel ctx to stop.
// Uses exponential backoff (1s→30s) on consecutive errors.
func (c *Consumer) Start(ctx context.Context) {
	go func() {
		backoff := time.Second
		const maxBackoff = 30 * time.Second
		for {
			if err := c.group.Consume(ctx, c.topics, &consumerGroupHandler{handler: c.handler}); err != nil {
				slog.Error("kafka consumer error", "err", err, "retry_in", backoff)
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				continue
			}
			backoff = time.Second // reset on clean session
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()
}

// Close shuts down the consumer group.
func (c *Consumer) Close() error {
	if err := c.group.Close(); err != nil {
		return fmt.Errorf("close consumer group: %w", err)
	}
	return nil
}

// consumerGroupHandler implements sarama.ConsumerGroupHandler.
type consumerGroupHandler struct {
	handler MessageHandler
}

func (h *consumerGroupHandler) Setup(_ sarama.ConsumerGroupSession) error   { return nil }
func (h *consumerGroupHandler) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }
func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		start := time.Now()
		var evt ChatEvent
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			slog.Error("kafka: unmarshal error", "err", err)
			metrics.KafkaConsumeDuration.WithLabelValues(msg.Topic).Observe(time.Since(start).Seconds())
			session.MarkMessage(msg, "")
			continue
		}
		h.handler(msg.Topic, evt)
		metrics.KafkaConsumeTotal.WithLabelValues(msg.Topic).Inc()
		metrics.KafkaConsumeDuration.WithLabelValues(msg.Topic).Observe(time.Since(start).Seconds())
		session.MarkMessage(msg, "")
	}
	return nil
}
