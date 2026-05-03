package users_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/Joessst-Dev/queue-ti/internal/users"
)

var _ = Describe("MatchesPattern", func() {
	Context("when the pattern is the wildcard *", func() {
		It("should match any topic", func() {
			Expect(users.MatchesPattern("*", "orders")).To(BeTrue())
			Expect(users.MatchesPattern("*", "orders.new")).To(BeTrue())
			Expect(users.MatchesPattern("*", "payments.refund.initiated")).To(BeTrue())
		})
	})

	Context("when the pattern uses a .* suffix", func() {
		It("should match topics that start with the prefix followed by a dot", func() {
			Expect(users.MatchesPattern("orders.*", "orders.new")).To(BeTrue())
			Expect(users.MatchesPattern("orders.*", "orders.cancelled")).To(BeTrue())
		})

		It("should match topics with multiple segments after the prefix", func() {
			Expect(users.MatchesPattern("orders.*", "orders.a.b")).To(BeTrue())
		})

		It("should NOT match the bare prefix without a following dot", func() {
			Expect(users.MatchesPattern("orders.*", "orders")).To(BeFalse())
		})

		It("should NOT match a different top-level prefix", func() {
			Expect(users.MatchesPattern("orders.*", "payments.new")).To(BeFalse())
		})

		It("should NOT match a topic that merely contains the prefix as a substring", func() {
			Expect(users.MatchesPattern("orders.*", "my-orders.new")).To(BeFalse())
		})
	})

	Context("when the pattern is an exact topic name", func() {
		It("should match only that exact topic", func() {
			Expect(users.MatchesPattern("orders", "orders")).To(BeTrue())
		})

		It("should NOT match a different topic", func() {
			Expect(users.MatchesPattern("orders", "payments")).To(BeFalse())
		})

		It("should NOT match a topic that has the pattern as a prefix", func() {
			Expect(users.MatchesPattern("orders", "orders.new")).To(BeFalse())
		})
	})
})

var _ = Describe("HasDequeueAccess", func() {
	Context("when the grants list is empty", func() {
		It("should return false", func() {
			Expect(users.HasDequeueAccess(nil, "orders")).To(BeFalse())
			Expect(users.HasDequeueAccess([]users.Grant{}, "orders")).To(BeFalse())
		})
	})

	Context("when the user has a write grant on the topic", func() {
		It("should return true", func() {
			grants := []users.Grant{
				{Action: "write", TopicPattern: "orders"},
			}
			Expect(users.HasDequeueAccess(grants, "orders")).To(BeTrue())
		})
	})

	Context("when the user has a consume grant on the topic", func() {
		It("should return true", func() {
			grants := []users.Grant{
				{Action: "consume", TopicPattern: "orders", ConsumerGroup: "group-a"},
			}
			Expect(users.HasDequeueAccess(grants, "orders")).To(BeTrue())
		})
	})

	Context("when the user only has a read grant on the topic", func() {
		It("should return false", func() {
			grants := []users.Grant{
				{Action: "read", TopicPattern: "orders"},
			}
			Expect(users.HasDequeueAccess(grants, "orders")).To(BeFalse())
		})
	})

	Context("when the user has a write grant on a wildcard pattern", func() {
		It("should return true for any matching topic", func() {
			grants := []users.Grant{
				{Action: "write", TopicPattern: "*"},
			}
			Expect(users.HasDequeueAccess(grants, "orders")).To(BeTrue())
			Expect(users.HasDequeueAccess(grants, "payments")).To(BeTrue())
		})
	})

	Context("when the user has write access to a different topic", func() {
		It("should return false for the unmatched topic", func() {
			grants := []users.Grant{
				{Action: "write", TopicPattern: "payments"},
			}
			Expect(users.HasDequeueAccess(grants, "orders")).To(BeFalse())
		})
	})
})

var _ = Describe("HasConsumerGroupAccess", func() {
	Context("when the user has no consume grants for the topic", func() {
		It("should return true unconditionally (opt-in restriction)", func() {
			Expect(users.HasConsumerGroupAccess(nil, "orders", "group-a")).To(BeTrue())
			Expect(users.HasConsumerGroupAccess([]users.Grant{}, "orders", "group-a")).To(BeTrue())
		})

		It("should return true even when the user has non-consume grants on the topic", func() {
			grants := []users.Grant{
				{Action: "write", TopicPattern: "orders"},
				{Action: "read", TopicPattern: "orders"},
			}
			Expect(users.HasConsumerGroupAccess(grants, "orders", "group-a")).To(BeTrue())
		})

		It("should return true when consume grants exist for a different topic only", func() {
			grants := []users.Grant{
				{Action: "consume", TopicPattern: "payments", ConsumerGroup: "group-a"},
			}
			Expect(users.HasConsumerGroupAccess(grants, "orders", "group-a")).To(BeTrue())
		})
	})

	Context("when the user has at least one consume grant for the topic", func() {
		It("should return true only for the explicitly granted group", func() {
			grants := []users.Grant{
				{Action: "consume", TopicPattern: "orders", ConsumerGroup: "group-a"},
			}
			Expect(users.HasConsumerGroupAccess(grants, "orders", "group-a")).To(BeTrue())
			Expect(users.HasConsumerGroupAccess(grants, "orders", "group-b")).To(BeFalse())
		})

		It("should return true for any of multiple granted groups", func() {
			grants := []users.Grant{
				{Action: "consume", TopicPattern: "orders", ConsumerGroup: "group-a"},
				{Action: "consume", TopicPattern: "orders", ConsumerGroup: "group-b"},
			}
			Expect(users.HasConsumerGroupAccess(grants, "orders", "group-a")).To(BeTrue())
			Expect(users.HasConsumerGroupAccess(grants, "orders", "group-b")).To(BeTrue())
			Expect(users.HasConsumerGroupAccess(grants, "orders", "group-c")).To(BeFalse())
		})

		It("should match consume grants using wildcard topic patterns", func() {
			grants := []users.Grant{
				{Action: "consume", TopicPattern: "orders.*", ConsumerGroup: "group-a"},
			}
			Expect(users.HasConsumerGroupAccess(grants, "orders.new", "group-a")).To(BeTrue())
			Expect(users.HasConsumerGroupAccess(grants, "orders.new", "group-b")).To(BeFalse())
			// A consume grant on orders.* does not activate restriction for payments
			Expect(users.HasConsumerGroupAccess(grants, "payments", "group-a")).To(BeTrue())
		})
	})
})

var _ = Describe("HasGrant", func() {
	Context("when the grants list is empty", func() {
		It("should return false for any action and topic", func() {
			Expect(users.HasGrant(nil, "read", "orders")).To(BeFalse())
			Expect(users.HasGrant([]users.Grant{}, "write", "payments")).To(BeFalse())
		})
	})

	Context("when a matching grant exists", func() {
		It("should return true when action and topic both match", func() {
			grants := []users.Grant{
				{Action: "read", TopicPattern: "orders"},
			}
			Expect(users.HasGrant(grants, "read", "orders")).To(BeTrue())
		})

		It("should return true when the grant uses a wildcard pattern that covers the topic", func() {
			grants := []users.Grant{
				{Action: "write", TopicPattern: "*"},
			}
			Expect(users.HasGrant(grants, "write", "anything")).To(BeTrue())
		})

		It("should return true when the grant uses a prefix pattern that covers the topic", func() {
			grants := []users.Grant{
				{Action: "read", TopicPattern: "orders.*"},
			}
			Expect(users.HasGrant(grants, "read", "orders.new")).To(BeTrue())
		})
	})

	Context("when no grant matches", func() {
		It("should return false when the action does not match", func() {
			grants := []users.Grant{
				{Action: "read", TopicPattern: "orders"},
			}
			Expect(users.HasGrant(grants, "write", "orders")).To(BeFalse())
		})

		It("should return false when the topic does not match the pattern", func() {
			grants := []users.Grant{
				{Action: "read", TopicPattern: "orders.*"},
			}
			Expect(users.HasGrant(grants, "read", "payments.new")).To(BeFalse())
		})

		It("should return false when a grant exists for a different user's topic", func() {
			grants := []users.Grant{
				{Action: "read", TopicPattern: "orders"},
			}
			Expect(users.HasGrant(grants, "read", "payments")).To(BeFalse())
		})
	})
})
