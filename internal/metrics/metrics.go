// Package metrics exposes Prometheus metrics for cloudTalk.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ActiveWSConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cloudtalk_ws_connections_active",
		Help: "Number of currently active WebSocket connections.",
	})

	KafkaPublishTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cloudtalk_kafka_publish_total",
		Help: "Total Kafka messages published.",
	}, []string{"topic", "status"}) // status: ok | error

	KafkaConsumeTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cloudtalk_kafka_consume_total",
		Help: "Total Kafka messages consumed.",
	}, []string{"topic"})

	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "cloudtalk_http_request_duration_seconds",
		Help:    "HTTP request latency.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path", "status"})
)
