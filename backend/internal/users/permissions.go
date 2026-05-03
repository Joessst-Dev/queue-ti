package users

import "strings"

func MatchesPattern(pattern, topic string) bool {
	if pattern == "*" {
		return true
	}
	if prefix, ok := strings.CutSuffix(pattern, ".*"); ok {
		return strings.HasPrefix(topic, prefix+".")
	}
	return pattern == topic
}

func HasGrant(grants []Grant, action, topic string) bool {
	for _, g := range grants {
		if g.Action == action && MatchesPattern(g.TopicPattern, topic) {
			return true
		}
	}
	return false
}

// HasDequeueAccess returns true when the user holds a "write" or "consume"
// grant that covers the given topic. Both actions permit consuming messages.
func HasDequeueAccess(grants []Grant, topic string) bool {
	return HasGrant(grants, "write", topic) || HasGrant(grants, "consume", topic)
}

// HasConsumerGroupAccess returns true when the user may consume from the given
// consumer group on the given topic.
//
// The restriction is opt-in: if the user has no "consume" grants at all for the
// topic, all groups are permitted. Once any "consume" grant exists for the topic
// the user is restricted to explicitly granted groups only.
func HasConsumerGroupAccess(grants []Grant, topic, consumerGroup string) bool {
	var hasAnyConsumeForTopic bool
	for _, g := range grants {
		if g.Action != "consume" || !MatchesPattern(g.TopicPattern, topic) {
			continue
		}
		hasAnyConsumeForTopic = true
		if g.ConsumerGroup == consumerGroup {
			return true
		}
	}
	// No consume grants for this topic — restriction is not active.
	return !hasAnyConsumeForTopic
}
