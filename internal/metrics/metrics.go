package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	opsProcessed   prometheus.Counter
	TopicCount     prometheus.Counter
	QueueLen       prometheus.Summary
	MsgIn          prometheus.Counter
	MsgAck         prometheus.Counter
	MsgNck         prometheus.Counter
	MsgDlq         prometheus.Counter
	HTTPLatencySec *prometheus.SummaryVec
	HTTPCount      *prometheus.CounterVec
)

func RegisterMetrics() {
	opsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "vfmp_processed_ops_total",
		Help: "The total number of processed events",
	})
	TopicCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "vfmp_topic_total",
		Help: "The total number of topics created",
	})
	QueueLen = promauto.NewSummary(prometheus.SummaryOpts{
		Name: "vfmp_queue_len",
		Help: "The number of messages across queues",
		Objectives: map[float64]float64{
			0.5:  0.05,
			0.95: 0.01,
		},
		MaxAge:     10 * time.Minute,
		AgeBuckets: 5,
	})
	MsgIn = promauto.NewCounter(prometheus.CounterOpts{
		Name: "vfmp_msg_in_total",
		Help: "The total number of messages received",
	})
	MsgAck = promauto.NewCounter(prometheus.CounterOpts{
		Name: "vfmp_msg_ack_count_total",
		Help: "The total number of messages received",
	})
	MsgNck = promauto.NewCounter(prometheus.CounterOpts{
		Name: "vfmp_msg_nck_count_total",
		Help: "The total number of messages received",
	})
	MsgDlq = promauto.NewCounter(prometheus.CounterOpts{
		Name: "vfmp_msg_dlq_count_total",
		Help: "The total number of messages received",
	})

	HTTPCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "vfmp_http_request_count_total",
		Help: "The total number of HTTP requests received",
	}, []string{"type"})
	HTTPLatencySec = promauto.NewSummaryVec(prometheus.SummaryOpts{
		Name: "http_req_latency_seconds",
		Help: "The latency for http requests in seconds",
		Objectives: map[float64]float64{
			0.5:  0.05,
			0.95: 0.01,
		},
		MaxAge:     10 * time.Minute,
		AgeBuckets: 5,
	}, []string{"method", "endpoint"})

	go func() {
		for {
			opsProcessed.Inc()
			time.Sleep(2 * time.Second)
		}
	}()
}
