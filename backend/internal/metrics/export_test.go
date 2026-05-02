package metrics

import "github.com/prometheus/client_golang/prometheus"

// NewRecorderForTest creates a Recorder using the internal constructor so that
// test code can access the raw counter fields for assertions.
func NewRecorderForTest(reg prometheus.Registerer) *Recorder { return newRecorder(reg) }

// EnqueuedCounter exposes the internal enqueued counter for test assertions.
func (r *Recorder) EnqueuedCounter() *prometheus.CounterVec { return r.enqueued }

// DequeuedCounter exposes the internal dequeued counter for test assertions.
func (r *Recorder) DequeuedCounter() *prometheus.CounterVec { return r.dequeued }

// AckedCounter exposes the internal acked counter for test assertions.
func (r *Recorder) AckedCounter() *prometheus.CounterVec { return r.acked }

// RequeuedCounter exposes the internal requeued counter for test assertions.
func (r *Recorder) RequeuedCounter() *prometheus.CounterVec { return r.requeued }

// NackedCounter exposes the internal nacked counter for test assertions.
func (r *Recorder) NackedCounter() *prometheus.CounterVec { return r.nacked }

// ExpiredCounter exposes the internal expired counter for test assertions.
func (r *Recorder) ExpiredCounter() prometheus.Counter { return r.expired }
