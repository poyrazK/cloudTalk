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
	Type     string          `json:"type"`               // "message" | "dm" | "typing" | "presence"
	RoomID   string          `json:"room_id,omitempty"`
	SenderID string          `json:"sender_id,omitempty"`
	ToUserID string          `json:"to_user_id,omitempty"`
	Payload  json.RawMessage `json:"payload"`
}

// Producer wraps a Sarama sync producer.
type Producer struct {
	producer sarama.SyncProducer
}

func NewProducer(brokers []string) (*Producer, error) {
	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = true
	cfg.Producer.RequiredAcks = sarama.WaitForLocal
	cfg.Producer.Compression = sarama.CompressionSnappy

	p, err := sarama.NewSyncProducer(brokers, cfg)
	if err != nil {
		return nil, fmt.Errorf("kafka producer: %w", err)
	}
	return &Producer{producer: p}, nil
}

// Publish sends a ChatEvent to the given topic using key as the partition key.
func (p *Producer) Publish(topic, key string, evt ChatEvent) error {
	data, err := json.Marshal(evt)
	if err != nil {
		metrics.KafkaPublishTotal.WithLabelValues(topic, "error").Inc()
		return err
	}
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.ByteEncoder(data),
	}
	_, _, err = p.producer.SendMessage(msg)
	if err != nil {
		metrics.KafkaPublishTotal.WithLabelValues(topic, "error").Inc()
		return err
	}
	metrics.KafkaPublishTotal.WithLabelValues(topic, "ok").Inc()
	return nil
}

// Close gracefully shuts down the producer.
func (p *Producer) Close() error {
	return p.producer.Close()
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
func (c *Consumer) Close() error { return c.group.Close() }

// consumerGroupHandler implements sarama.ConsumerGroupHandler.
type consumerGroupHandler struct {
	handler MessageHandler
}

func (h *consumerGroupHandler) Setup(_ sarama.ConsumerGroupSession) error   { return nil }
func (h *consumerGroupHandler) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }
func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		var evt ChatEvent
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			slog.Error("kafka: unmarshal error", "err", err)
			session.MarkMessage(msg, "")
			continue
		}
		h.handler(msg.Topic, evt)
		metrics.KafkaConsumeTotal.WithLabelValues(msg.Topic).Inc()
		session.MarkMessage(msg, "")
	}
	return nil
}
