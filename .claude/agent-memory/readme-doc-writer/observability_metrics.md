---
name: Prometheus Metrics & Observability
description: queue-ti Prometheus metrics, endpoints, and observability patterns
type: reference
---

## Metrics Implementation

- **Metrics package**: `internal/metrics/metrics.go` defines all metric definitions and collectors
- **Recorder interface**: `internal/queue/recorder.go` defines `MetricsRecorder` interface
- **HTTP route**: `/metrics` endpoint in `internal/server/http.go` (port 8080, unauthenticated)
- **Stats API**: `GET /api/stats` (authenticated) returns per-topic-status counts from database, used by admin UI

## Prometheus Metrics Exposed

**Counters** (with topic label):
- `queueti_enqueued_total` (topic)
- `queueti_dequeued_total` (topic)
- `queueti_acked_total` (topic)
- `queueti_nacked_total` (topic, outcome: retry|failed|dlq)
- `queueti_requeued_total` (topic)
- `queueti_expired_total` (no labels)

**Gauge** (custom collector):
- `queueti_queue_depth` (topic, status) — queried from DB on each scrape

## Key Architectural Facts

- `/metrics` is intentionally unauthenticated for operator access (protect at proxy level)
- `queueti_queue_depth` uses a custom collector that queries the database every scrape
- Nack outcome labels: "retry" (auto-retry), "failed" (failed status), "dlq" (promoted to DLQ)
- All counters have namespace prefix "queueti" (e.g., `queueti_enqueued_total`)
