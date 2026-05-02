package queue

import (
	"encoding/json"
	"fmt"
)

// hydrateMessage fills LastError from the nullable pointer and unmarshals the
// JSON metadata blob into msg.Metadata. Shared by Dequeue, DequeueN, and List
// to avoid duplicating post-scan boilerplate.
func hydrateMessage(msg *Message, metaJSON []byte, lastError *string) error {
	if lastError != nil {
		msg.LastError = *lastError
	}
	if metaJSON != nil {
		if err := json.Unmarshal(metaJSON, &msg.Metadata); err != nil {
			return fmt.Errorf("unmarshal metadata: %w", err)
		}
	}
	return nil
}
