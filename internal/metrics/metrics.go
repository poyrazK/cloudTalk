// Package metrics exposes Prometheus metrics for cloudTalk.
package metrics

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ActiveWSConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cloudtalk_ws_connections_active",
		Help: "Number of currently active WebSocket connections.",
	})

	WSConnectionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cloudtalk_ws_connections_total",
		Help: "Total WebSocket connection lifecycle events.",
	}, []string{"event"}) // event: connect | disconnect

	WSThrottledEventsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cloudtalk_ws_throttled_events_total",
		Help: "Total throttled WebSocket client events.",
	}, []string{"type", "action"}) // action: drop | reject

	KafkaPublishTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cloudtalk_kafka_publish_total",
		Help: "Total Kafka messages published.",
	}, []string{"topic", "status"}) // status: ok | error

	KafkaPublishDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "cloudtalk_kafka_publish_duration_seconds",
		Help:    "Kafka publish latency.",
		Buckets: prometheus.DefBuckets,
	}, []string{"topic", "status"})

	KafkaConsumeTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cloudtalk_kafka_consume_total",
		Help: "Total Kafka messages consumed.",
	}, []string{"topic"})

	KafkaConsumeDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "cloudtalk_kafka_consume_duration_seconds",
		Help:    "Kafka consumer handler latency.",
		Buckets: prometheus.DefBuckets,
	}, []string{"topic"})

	KafkaConsumerErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cloudtalk_kafka_consumer_errors_total",
		Help: "Total Kafka consumer loop errors.",
	}, []string{"topic"})

	KafkaDecodeErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cloudtalk_kafka_decode_errors_total",
		Help: "Total Kafka message decode errors.",
	}, []string{"topic"})

	KafkaTopicVerificationTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cloudtalk_kafka_topic_verification_total",
		Help: "Total Kafka topic verification attempts.",
	}, []string{"status"}) // status: ok | error

	HTTPRequestTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cloudtalk_http_requests_total",
		Help: "Total HTTP requests processed.",
	}, []string{"method", "path", "status"})

	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "cloudtalk_http_request_duration_seconds",
		Help:    "HTTP request latency.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path", "status"})

	HTTPThrottledRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cloudtalk_http_throttled_requests_total",
		Help: "Total throttled HTTP requests.",
	}, []string{"group", "scope"})

	DBPoolConnections = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cloudtalk_db_pool_connections",
		Help: "Database pool connection counts.",
	}, []string{"state"}) // state: total | idle | acquired | max
)

func UpdateDBPoolStats(stats *pgxpool.Stat) {
	DBPoolConnections.WithLabelValues("total").Set(float64(stats.TotalConns()))
	DBPoolConnections.WithLabelValues("idle").Set(float64(stats.IdleConns()))
	DBPoolConnections.WithLabelValues("acquired").Set(float64(stats.AcquiredConns()))
	DBPoolConnections.WithLabelValues("max").Set(float64(stats.MaxConns()))
}
