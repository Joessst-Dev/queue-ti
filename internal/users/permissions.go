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
