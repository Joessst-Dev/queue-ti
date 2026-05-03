# Observability

## Prometheus Metrics

queue-ti exposes Prometheus metrics on the HTTP server at the `/metrics` endpoint (port 8080) in Prometheus text format. Metrics are exported in real time and require no additional configuration.

> **Note**: The `/metrics` endpoint is **unauthenticated** even when `auth.enabled: true`. This is by design — operators typically protect this endpoint at the network or reverse proxy level.

### Metrics Endpoint

```bash
GET http://localhost:8080/metrics
```

### Prometheus Scrape Configuration

Add this to your Prometheus configuration (`prometheus.yml`):

```yaml
scrape_configs:
  - job_name: queue-ti
    static_configs:
      - targets: ['localhost:8080']
    scrape_interval: 15s
```

### Exported Metrics

**Counters** (cumulative, monotonically increasing):

| Metric | Labels | Description |
|--------|--------|-------------|
| `queueti_enqueued_total` | `topic` | Total messages enqueued |
| `queueti_dequeued_total` | `topic` | Total messages dequeued |
| `queueti_acked_total` | `topic` | Total messages acknowledged (deleted) |
| `queueti_nacked_total` | `topic`, `outcome` | Total messages nacked; outcome: `retry`, `failed`, or `dlq` |
| `queueti_requeued_total` | `topic` | Total messages requeued from DLQ to original topic |
| `queueti_expired_total` | — | Total messages expired by the automatic reaper |

**Gauge** (sampled from database on each scrape):

| Metric | Labels | Description |
|--------|--------|-------------|
| `queueti_queue_depth` | `topic`, `status` | Current number of messages per topic and status |

### Example Scrape Output

```
# HELP queueti_enqueued_total Total messages enqueued
# TYPE queueti_enqueued_total counter
queueti_enqueued_total{topic="orders"} 1042
queueti_enqueued_total{topic="payments"} 523

# HELP queueti_queue_depth Number of messages per topic and status
# TYPE queueti_queue_depth gauge
queueti_queue_depth{status="pending",topic="orders"} 5
queueti_queue_depth{status="processing",topic="orders"} 2
queueti_queue_depth{status="deleted",topic="orders"} 1028
```

### Recommended Alerts

Consider setting up these Prometheus alerts for production deployments:

```yaml
groups:
  - name: queue-ti
    rules:
      # Alert if queue depth grows unbounded
      - alert: QueueTIHighQueueDepth
        expr: queueti_queue_depth{status="pending"} > 1000
        for: 5m
        annotations:
          summary: "High queue depth on {{ $labels.topic }}"

      # Alert on high nack rate (potential consumer issue)
      - alert: QueueTIHighNackRate
        expr: rate(queueti_nacked_total[5m]) > 10
        for: 5m
        annotations:
          summary: "High nack rate on {{ $labels.topic }}"

      # Alert if DLQ is accumulating messages
      - alert: QueueTIHighDLQPromotion
        expr: increase(queueti_nacked_total{outcome="dlq"}[1h]) > 50
        for: 5m
        annotations:
          summary: "DLQ accumulation on {{ $labels.topic }}"
```

## Admin UI Queue Depth

The admin UI displays live queue depth for each topic and status in the **Messages** table. Use the **Stats** tab or the `/api/stats` endpoint to query queue depth programmatically:

```bash
curl http://localhost:8080/api/stats
```

**Response:**

```json
{
  "topics": [
    {"topic": "orders", "status": "pending", "count": 5},
    {"topic": "orders", "status": "processing", "count": 2}
  ]
}
```
