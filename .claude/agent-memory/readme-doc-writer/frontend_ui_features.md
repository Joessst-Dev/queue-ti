---
name: Frontend UI Features Added
description: Admin UI now displays retry counts, expiry, status badges, DLQ highlighting, and supports Nack/Requeue actions
type: project
---

**When**: 2026-04-24

**What was added**:
- Message table now shows: `retry_count / max_retries`, `expires_at`, status badges (pending/yellow, processing/blue, failed/red, expired/orange)
- DLQ messages highlighted with amber background; displays `original_topic` as sub-label
- Requeue button on DLQ messages (calls `POST /api/messages/:id/requeue`)
- Inline Nack UI on processing messages: expands text input for optional error reason, confirms to call `POST /api/messages/:id/nack`
- Last error stored in `last_error` field shown as tooltip on retry count cell
- `QueueService` methods: `nackMessage(id, error?)` and `requeueMessage(id)`

**Why**: Improves queue observability and operational control; admins can now inspect retry history, requeue failed messages, and signal nacks directly from the UI.

**How to apply**: When documenting new admin UI features, reference these status badges and action buttons; keep architecture section accurate with the new service methods.
