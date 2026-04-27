package queue

// MetricsRecorder is satisfied by metrics.Recorder and NoopRecorder.
// Keeping it in the queue package avoids an import cycle.
type MetricsRecorder interface {
	RecordEnqueue(topic string)
	RecordDequeue(topic string)
	RecordAck(topic string)
	RecordNack(topic, outcome string) // outcome: "retry" | "failed" | "dlq"
	RecordRequeue(topic string)
	RecordExpired(n int64)
	RecordDeleted(n int64)
}

// NoopRecorder is a MetricsRecorder that discards all observations.
// It is used when no metrics backend is configured.
type NoopRecorder struct{}

func (NoopRecorder) RecordEnqueue(string)   {}
func (NoopRecorder) RecordDequeue(string)   {}
func (NoopRecorder) RecordAck(string)       {}
func (NoopRecorder) RecordNack(_, _ string) {}
func (NoopRecorder) RecordRequeue(string)   {}
func (NoopRecorder) RecordExpired(int64)    {}
func (NoopRecorder) RecordDeleted(int64)    {}
