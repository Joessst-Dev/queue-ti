package metrics

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

const ns = "queueti"

// Recorder implements queue.MetricsRecorder using Prometheus counters.
type Recorder struct {
	enqueued *prometheus.CounterVec
	dequeued *prometheus.CounterVec
	acked    *prometheus.CounterVec
	nacked   *prometheus.CounterVec
	requeued *prometheus.CounterVec
	expired  prometheus.Counter
	deleted  prometheus.Counter
}

// New creates a Recorder and a DepthCollector, registers both with reg.
func New(pool *pgxpool.Pool, reg prometheus.Registerer) *Recorder {
	r := newRecorder(reg)
	newDepthCollector(pool, reg)
	return r
}

// newRecorder creates and registers a Recorder without the depth collector.
// It is used by New and by test helpers in export_test.go.
func newRecorder(reg prometheus.Registerer) *Recorder {
	r := &Recorder{
		enqueued: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "enqueued_total",
			Help:      "Total messages enqueued.",
		}, []string{"topic"}),
		dequeued: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "dequeued_total",
			Help:      "Total messages dequeued.",
		}, []string{"topic"}),
		acked: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "acked_total",
			Help:      "Total messages acknowledged.",
		}, []string{"topic"}),
		nacked: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "nacked_total",
			Help:      "Total messages nacked.",
		}, []string{"topic", "outcome"}),
		requeued: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "requeued_total",
			Help:      "Total messages requeued from DLQ.",
		}, []string{"topic"}),
		expired: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "expired_total",
			Help:      "Total messages expired by the reaper.",
		}),
		deleted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "messages_deleted_total",
			Help:      "Total messages permanently deleted by the delete reaper.",
		}),
	}
	reg.MustRegister(r.enqueued, r.dequeued, r.acked, r.nacked, r.requeued, r.expired, r.deleted)
	return r
}

func (r *Recorder) RecordEnqueue(topic string)       { r.enqueued.WithLabelValues(topic).Inc() }
func (r *Recorder) RecordDequeue(topic string)       { r.dequeued.WithLabelValues(topic).Inc() }
func (r *Recorder) RecordAck(topic string)           { r.acked.WithLabelValues(topic).Inc() }
func (r *Recorder) RecordNack(topic, outcome string) { r.nacked.WithLabelValues(topic, outcome).Inc() }
func (r *Recorder) RecordRequeue(topic string)       { r.requeued.WithLabelValues(topic).Inc() }
func (r *Recorder) RecordExpired(n int64)            { r.expired.Add(float64(n)) }
func (r *Recorder) RecordDeleted(n int64)            { r.deleted.Add(float64(n)) }

type depthCollector struct {
	pool *pgxpool.Pool
	desc *prometheus.Desc
}

func newDepthCollector(pool *pgxpool.Pool, reg prometheus.Registerer) {
	c := &depthCollector{
		pool: pool,
		desc: prometheus.NewDesc(
			ns+"_queue_depth",
			"Number of messages per topic and status.",
			[]string{"topic", "status"},
			nil,
		),
	}
	reg.MustRegister(c)
}

func (c *depthCollector) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }

func (c *depthCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := c.pool.Query(ctx, `SELECT topic, status, COUNT(*) FROM messages GROUP BY topic, status`)
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.desc, err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var topic, status string
		var count float64
		if err := rows.Scan(&topic, &status, &count); err != nil {
			ch <- prometheus.NewInvalidMetric(c.desc, err)
			return
		}
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, count, topic, status)
	}
}
