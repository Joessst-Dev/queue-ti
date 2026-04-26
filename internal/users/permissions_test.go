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
