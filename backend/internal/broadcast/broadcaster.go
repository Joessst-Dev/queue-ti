package broadcast

import "context"

// Broadcaster publishes and receives text notifications across service instances.
// The PostgreSQL implementation uses LISTEN/NOTIFY; a future Redis implementation
// will satisfy the same interface, making PG the fallback when Redis is disabled.
type Broadcaster interface {
	// Publish sends payload on channel to all subscribers across all instances.
	Publish(ctx context.Context, channel, payload string) error
	// Subscribe registers interest in channel. The returned channel receives
	// payloads until cancel is called. The channel is closed after cancel.
	Subscribe(ctx context.Context, channel string) (<-chan string, context.CancelFunc)
	// Close releases any persistent resources held by the implementation.
	Close() error
}

const (
	ChannelSchemaChanged = "queue_ti_schema_changed"
	ChannelConfigChanged = "queue_ti_config_changed"
)
