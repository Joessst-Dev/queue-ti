package metrics_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/Joessst-Dev/queue-ti/internal/metrics"
)

var _ = Describe("Recorder", func() {
	var (
		reg      *prometheus.Registry
		recorder *metrics.Recorder
	)

	BeforeEach(func() {
		reg = prometheus.NewRegistry()
		recorder = metrics.NewRecorderForTest(reg)
	})

	Describe("RecordEnqueue", func() {
		Context("when called once for a topic", func() {
			It("increments queueti_enqueued_total for the given topic", func() {
				recorder.RecordEnqueue("orders")

				Expect(testutil.ToFloat64(recorder.EnqueuedCounter().WithLabelValues("orders"))).To(Equal(1.0))
			})
		})

		Context("when called multiple times for different topics", func() {
			It("tracks each topic independently", func() {
				recorder.RecordEnqueue("orders")
				recorder.RecordEnqueue("orders")
				recorder.RecordEnqueue("payments")

				Expect(testutil.ToFloat64(recorder.EnqueuedCounter().WithLabelValues("orders"))).To(Equal(2.0))
				Expect(testutil.ToFloat64(recorder.EnqueuedCounter().WithLabelValues("payments"))).To(Equal(1.0))
			})
		})
	})

	Describe("RecordDequeue", func() {
		Context("when called once for a topic", func() {
			It("increments queueti_dequeued_total for the given topic", func() {
				recorder.RecordDequeue("invoices")

				Expect(testutil.ToFloat64(recorder.DequeuedCounter().WithLabelValues("invoices"))).To(Equal(1.0))
			})
		})
	})

	Describe("RecordAck", func() {
		Context("when called once for a topic", func() {
			It("increments queueti_acked_total for the given topic", func() {
				recorder.RecordAck("shipments")

				Expect(testutil.ToFloat64(recorder.AckedCounter().WithLabelValues("shipments"))).To(Equal(1.0))
			})
		})
	})

	Describe("RecordRequeue", func() {
		Context("when called once for a topic", func() {
			It("increments queueti_requeued_total for the given topic", func() {
				recorder.RecordRequeue("returns")

				Expect(testutil.ToFloat64(recorder.RequeuedCounter().WithLabelValues("returns"))).To(Equal(1.0))
			})
		})
	})

	Describe("RecordNack", func() {
		Context(`when called with outcome "retry"`, func() {
			It(`increments queueti_nacked_total{topic="orders",outcome="retry"}`, func() {
				recorder.RecordNack("orders", "retry")

				Expect(testutil.ToFloat64(recorder.NackedCounter().WithLabelValues("orders", "retry"))).To(Equal(1.0))
			})
		})

		Context(`when called with outcome "failed"`, func() {
			It(`increments queueti_nacked_total{topic="orders",outcome="failed"}`, func() {
				recorder.RecordNack("orders", "failed")

				Expect(testutil.ToFloat64(recorder.NackedCounter().WithLabelValues("orders", "failed"))).To(Equal(1.0))
			})
		})

		Context(`when called with outcome "dlq"`, func() {
			It(`increments queueti_nacked_total{topic="orders",outcome="dlq"}`, func() {
				recorder.RecordNack("orders", "dlq")

				Expect(testutil.ToFloat64(recorder.NackedCounter().WithLabelValues("orders", "dlq"))).To(Equal(1.0))
			})
		})

		Context("when called with different outcomes for the same topic", func() {
			It("tracks each outcome label independently", func() {
				recorder.RecordNack("orders", "retry")
				recorder.RecordNack("orders", "retry")
				recorder.RecordNack("orders", "dlq")

				Expect(testutil.ToFloat64(recorder.NackedCounter().WithLabelValues("orders", "retry"))).To(Equal(2.0))
				Expect(testutil.ToFloat64(recorder.NackedCounter().WithLabelValues("orders", "dlq"))).To(Equal(1.0))
				Expect(testutil.ToFloat64(recorder.NackedCounter().WithLabelValues("orders", "failed"))).To(Equal(0.0))
			})
		})
	})

	Describe("RecordExpired", func() {
		Context("when called with a positive count", func() {
			It("adds n to queueti_expired_total", func() {
				recorder.RecordExpired(5)

				Expect(testutil.ToFloat64(recorder.ExpiredCounter())).To(Equal(5.0))
			})
		})

		Context("when called multiple times", func() {
			It("accumulates the total across all calls", func() {
				recorder.RecordExpired(3)
				recorder.RecordExpired(7)

				Expect(testutil.ToFloat64(recorder.ExpiredCounter())).To(Equal(10.0))
			})
		})
	})
})
