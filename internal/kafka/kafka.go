package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/poyrazk/cloudtalk/internal/metrics"
	apptrace "github.com/poyrazk/cloudtalk/internal/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	TopicRoomMessages = "chat.room.messages"
	TopicDMMessages   = "chat.dm.messages"
	TopicPresence     = "chat.presence"
)

func RequiredTopics() []string {
	return []string{TopicRoomMessages, TopicDMMessages, TopicPresence}
}

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

type metadataClient interface {
	RefreshMetadata(topics ...string) error
	Topics() ([]string, error)
	Partitions(topic string) ([]int32, error)
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
func (p *Producer) Publish(ctx context.Context, topic, key string, evt ChatEvent) error {
	ctx, span := otel.Tracer("cloudtalk/kafka").Start(ctx, "kafka.produce",
		trace.WithAttributes(
			attribute.String("messaging.system", "kafka"),
			attribute.String("messaging.destination.name", topic),
			attribute.String("messaging.operation", "publish"),
		),
	)
	defer span.End()

	start := time.Now()
	data, err := json.Marshal(evt)
	if err != nil {
		span.RecordError(err)
		metrics.KafkaPublishTotal.WithLabelValues(topic, "error").Inc()
		metrics.KafkaPublishDuration.WithLabelValues(topic, "error").Observe(time.Since(start).Seconds())
		return fmt.Errorf("marshal chat event: %w", err)
	}
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.ByteEncoder(data),
	}
	carrier := producerMessageCarrier{msg: msg}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	_, _, err = p.producer.SendMessage(msg)
	if err != nil {
		span.RecordError(err)
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

func (p *Producer) VerifyTopics(ctx context.Context, topics []string) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- verifyTopics(p.client, topics)
	}()

	select {
	case <-ctx.Done():
		metrics.KafkaTopicVerificationTotal.WithLabelValues("error").Inc()
		return fmt.Errorf("verify kafka topics: %w", ctx.Err())
	case err := <-errCh:
		if err != nil {
			metrics.KafkaTopicVerificationTotal.WithLabelValues("error").Inc()
			return fmt.Errorf("verify kafka topics: %w", err)
		}
		metrics.KafkaTopicVerificationTotal.WithLabelValues("ok").Inc()
		return nil
	}
}

func verifyTopics(client metadataClient, topics []string) error {
	if len(topics) == 0 {
		return nil
	}
	if err := client.RefreshMetadata(topics...); err != nil {
		return fmt.Errorf("refresh metadata: %w", err)
	}
	existing, err := client.Topics()
	if err != nil {
		return fmt.Errorf("list topics: %w", err)
	}
	existingSet := make(map[string]struct{}, len(existing))
	for _, topic := range existing {
		existingSet[topic] = struct{}{}
	}

	missing := make([]string, 0)
	for _, topic := range topics {
		if _, ok := existingSet[topic]; !ok {
			missing = append(missing, topic)
			continue
		}
		partitions, err := client.Partitions(topic)
		if err != nil {
			return fmt.Errorf("topic %s partitions: %w", topic, err)
		}
		if len(partitions) == 0 {
			missing = append(missing, topic)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("missing required topics: %v", missing)
	}
	return nil
}

// --- Consumer ---

// MessageHandler is called for each Kafka message.
type MessageHandler func(ctx context.Context, topic string, evt ChatEvent)

// Consumer wraps a Sarama consumer group.
type Consumer struct {
	group   sarama.ConsumerGroup
	topics  []string
	handler MessageHandler

	mu       sync.RWMutex
	readyErr error
}

func NewConsumer(brokers []string, groupID string, topics []string, handler MessageHandler) (*Consumer, error) {
	cfg := sarama.NewConfig()
	cfg.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}
	cfg.Consumer.Offsets.Initial = sarama.OffsetNewest

	g, err := sarama.NewConsumerGroup(brokers, groupID, cfg)
	if err != nil {
		return nil, fmt.Errorf("kafka consumer: %w", err)
	}
	consumer := &Consumer{group: g, topics: topics, handler: handler}
	consumer.setReadyError(errors.New("consumer has not completed an initial session yet"))
	return consumer, nil
}

// Start begins consuming in a background goroutine. Cancel ctx to stop.
// Uses exponential backoff (1s→30s) on consecutive errors.
func (c *Consumer) Start(ctx context.Context) {
	go func() {
		backoff := time.Second
		const maxBackoff = 30 * time.Second
		for {
			if err := c.group.Consume(ctx, c.topics, &consumerGroupHandler{handler: c.handler, onSetup: func() { c.setReadyError(nil) }}); err != nil {
				c.setReadyError(err)
				for _, topic := range c.topics {
					metrics.KafkaConsumerErrorsTotal.WithLabelValues(topic).Inc()
				}
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

func (c *Consumer) ReadyError() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.readyErr
}

func (c *Consumer) setReadyError(err error) {
	c.mu.Lock()
	c.readyErr = err
	c.mu.Unlock()
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
	onSetup func()
}

func (h *consumerGroupHandler) Setup(_ sarama.ConsumerGroupSession) error {
	if h.onSetup != nil {
		h.onSetup()
	}
	return nil
}
func (h *consumerGroupHandler) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }
func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		start := time.Now()
		var evt ChatEvent
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			metrics.KafkaDecodeErrorsTotal.WithLabelValues(msg.Topic).Inc()
			slog.Error("kafka: unmarshal error", "err", err, "topic", msg.Topic, "partition", msg.Partition, "offset", msg.Offset)
			metrics.KafkaConsumeDuration.WithLabelValues(msg.Topic).Observe(time.Since(start).Seconds())
			session.MarkMessage(msg, "")
			continue
		}
		ctx := otel.GetTextMapPropagator().Extract(context.Background(), consumerMessageCarrier{headers: msg.Headers})
		ctx, span := apptrace.Tracer("cloudtalk/kafka").Start(ctx, "kafka.consume",
			trace.WithAttributes(
				attribute.String("messaging.system", "kafka"),
				attribute.String("messaging.destination.name", msg.Topic),
				attribute.Int64("messaging.kafka.partition", int64(msg.Partition)),
				attribute.Int64("messaging.kafka.offset", msg.Offset),
			),
		)
		h.handler(ctx, msg.Topic, evt)
		span.End()
		metrics.KafkaConsumeTotal.WithLabelValues(msg.Topic).Inc()
		metrics.KafkaConsumeDuration.WithLabelValues(msg.Topic).Observe(time.Since(start).Seconds())
		session.MarkMessage(msg, "")
	}
	return nil
}

type producerMessageCarrier struct {
	msg *sarama.ProducerMessage
}

func (c producerMessageCarrier) Get(key string) string {
	for _, header := range c.msg.Headers {
		if string(header.Key) == key {
			return string(header.Value)
		}
	}
	return ""
}

func (c producerMessageCarrier) Set(key, value string) {
	for i, header := range c.msg.Headers {
		if string(header.Key) == key {
			c.msg.Headers[i].Value = []byte(value)
			return
		}
	}
	c.msg.Headers = append(c.msg.Headers, sarama.RecordHeader{Key: []byte(key), Value: []byte(value)})
}

func (c producerMessageCarrier) Keys() []string {
	keys := make([]string, 0, len(c.msg.Headers))
	for _, header := range c.msg.Headers {
		keys = append(keys, string(header.Key))
	}
	return keys
}

type consumerMessageCarrier struct {
	headers []*sarama.RecordHeader
}

func (c consumerMessageCarrier) Get(key string) string {
	for _, header := range c.headers {
		if string(header.Key) == key {
			return string(header.Value)
		}
	}
	return ""
}

func (c consumerMessageCarrier) Set(_, _ string) {}

func (c consumerMessageCarrier) Keys() []string {
	keys := make([]string, 0, len(c.headers))
	for _, header := range c.headers {
		keys = append(keys, string(header.Key))
	}
	return keys
}

var (
	_ propagation.TextMapCarrier = producerMessageCarrier{}
	_ propagation.TextMapCarrier = consumerMessageCarrier{}
)
