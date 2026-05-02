package users_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Joessst-Dev/queue-ti/internal/db"
	"github.com/Joessst-Dev/queue-ti/internal/users"
)

var (
	usersTestPool *pgxpool.Pool
	usersTestCtx  context.Context
	pgContainer   *tcpostgres.PostgresContainer
)

var _ = BeforeSuite(func() {
	usersTestCtx = context.Background()

	var err error
	pgContainer, err = tcpostgres.Run(usersTestCtx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("queueti_users_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	Expect(err).NotTo(HaveOccurred())

	dsn, err := pgContainer.ConnectionString(usersTestCtx, "sslmode=disable")
	Expect(err).NotTo(HaveOccurred())

	usersTestPool, err = pgxpool.New(usersTestCtx, dsn)
	Expect(err).NotTo(HaveOccurred())

	err = db.Migrate(usersTestCtx, usersTestPool)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	if usersTestPool != nil {
		usersTestPool.Close()
	}
	if pgContainer != nil {
		_ = pgContainer.Terminate(usersTestCtx)
	}
})

var _ = Describe("Store", func() {
	var store *users.Store

	BeforeEach(func() {
		_, err := usersTestPool.Exec(usersTestCtx, "DELETE FROM user_grants")
		Expect(err).NotTo(HaveOccurred())
		_, err = usersTestPool.Exec(usersTestCtx, "DELETE FROM users")
		Expect(err).NotTo(HaveOccurred())

		store = users.NewStore(usersTestPool)
	})

	// -------------------------------------------------------------------------
	// Store.Create
	// -------------------------------------------------------------------------

	Describe("Create", func() {
		Context("when the username and password are valid", func() {
			It("should persist the user and return a populated User struct", func() {
				u, err := store.Create(usersTestCtx, "alice", "s3cr3t", false)

				Expect(err).NotTo(HaveOccurred())
				Expect(u.ID).NotTo(BeEmpty())
				Expect(u.Username).To(Equal("alice"))
				Expect(u.IsAdmin).To(BeFalse())
				Expect(u.CreatedAt).NotTo(BeZero())
				Expect(u.UpdatedAt).NotTo(BeZero())
			})

			It("should store a bcrypt hash, not the plain-text password", func() {
				_, err := store.Create(usersTestCtx, "bob", "plaintext", false)
				Expect(err).NotTo(HaveOccurred())

				_, hash, err := store.GetByUsername(usersTestCtx, "bob")
				Expect(err).NotTo(HaveOccurred())
				Expect(hash).NotTo(Equal("plaintext"))
				Expect(hash).To(HavePrefix("$2"))
			})
		})

		Context("when the same username is created twice", func() {
			It("should return ErrDuplicate on the second call", func() {
				_, err := store.Create(usersTestCtx, "dup-user", "pass1", false)
				Expect(err).NotTo(HaveOccurred())

				_, err = store.Create(usersTestCtx, "dup-user", "pass2", true)
				Expect(err).To(MatchError(users.ErrDuplicate))
			})
		})
	})

	// -------------------------------------------------------------------------
	// Store.GetByUsername
	// -------------------------------------------------------------------------

	Describe("GetByUsername", func() {
		Context("when the user exists", func() {
			It("should return the user and a non-empty password hash", func() {
				created, err := store.Create(usersTestCtx, "charlie", "mypass", true)
				Expect(err).NotTo(HaveOccurred())

				u, hash, err := store.GetByUsername(usersTestCtx, "charlie")
				Expect(err).NotTo(HaveOccurred())
				Expect(u.ID).To(Equal(created.ID))
				Expect(u.Username).To(Equal("charlie"))
				Expect(u.IsAdmin).To(BeTrue())
				Expect(hash).NotTo(BeEmpty())
			})
		})

		Context("when the username does not exist", func() {
			It("should return ErrNotFound", func() {
				_, _, err := store.GetByUsername(usersTestCtx, "ghost")
				Expect(err).To(MatchError(users.ErrNotFound))
			})
		})
	})

	// -------------------------------------------------------------------------
	// Store.GetByID
	// -------------------------------------------------------------------------

	Describe("GetByID", func() {
		Context("when the user exists", func() {
			It("should return the user by ID", func() {
				created, err := store.Create(usersTestCtx, "diana", "pass", false)
				Expect(err).NotTo(HaveOccurred())

				u, err := store.GetByID(usersTestCtx, created.ID)
				Expect(err).NotTo(HaveOccurred())
				Expect(u.ID).To(Equal(created.ID))
				Expect(u.Username).To(Equal("diana"))
			})
		})

		Context("when the ID does not exist", func() {
			It("should return ErrNotFound", func() {
				_, err := store.GetByID(usersTestCtx, "00000000-0000-0000-0000-000000000000")
				Expect(err).To(MatchError(users.ErrNotFound))
			})
		})
	})

	// -------------------------------------------------------------------------
	// Store.List
	// -------------------------------------------------------------------------

	Describe("List", func() {
		Context("when multiple users exist", func() {
			It("should return all users ordered by username", func() {
				_, err := store.Create(usersTestCtx, "zara", "p", false)
				Expect(err).NotTo(HaveOccurred())
				_, err = store.Create(usersTestCtx, "anna", "p", false)
				Expect(err).NotTo(HaveOccurred())

				list, err := store.List(usersTestCtx)
				Expect(err).NotTo(HaveOccurred())
				Expect(list).To(HaveLen(2))
				Expect(list[0].Username).To(Equal("anna"))
				Expect(list[1].Username).To(Equal("zara"))
			})
		})

		Context("when no users exist", func() {
			It("should return an empty (nil) slice without error", func() {
				list, err := store.List(usersTestCtx)
				Expect(err).NotTo(HaveOccurred())
				Expect(list).To(BeEmpty())
			})
		})
	})

	// -------------------------------------------------------------------------
	// Store.Update
	// -------------------------------------------------------------------------

	Describe("Update", func() {
		var existing *users.User

		BeforeEach(func() {
			var err error
			existing, err = store.Create(usersTestCtx, "eve", "original", false)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when changing the username", func() {
			It("should update only the username field", func() {
				newName := "eve-renamed"
				u, err := store.Update(usersTestCtx, existing.ID, &newName, nil, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(u.Username).To(Equal("eve-renamed"))
				Expect(u.IsAdmin).To(BeFalse())
			})
		})

		Context("when changing the password", func() {
			It("should store a new bcrypt hash", func() {
				_, oldHash, err := store.GetByUsername(usersTestCtx, "eve")
				Expect(err).NotTo(HaveOccurred())

				newPass := "new-secret"
				_, err = store.Update(usersTestCtx, existing.ID, nil, &newPass, nil)
				Expect(err).NotTo(HaveOccurred())

				_, newHash, err := store.GetByUsername(usersTestCtx, "eve")
				Expect(err).NotTo(HaveOccurred())
				Expect(newHash).NotTo(Equal(oldHash))
			})
		})

		Context("when promoting to admin", func() {
			It("should flip the is_admin flag", func() {
				admin := true
				u, err := store.Update(usersTestCtx, existing.ID, nil, nil, &admin)
				Expect(err).NotTo(HaveOccurred())
				Expect(u.IsAdmin).To(BeTrue())
			})
		})

		Context("when passing no fields to update", func() {
			It("should return the unchanged user", func() {
				u, err := store.Update(usersTestCtx, existing.ID, nil, nil, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(u.Username).To(Equal("eve"))
			})
		})

		Context("when the user does not exist", func() {
			It("should return ErrNotFound", func() {
				newName := "nobody"
				_, err := store.Update(usersTestCtx, "00000000-0000-0000-0000-000000000000", &newName, nil, nil)
				Expect(err).To(MatchError(users.ErrNotFound))
			})
		})

		Context("when the new username conflicts with an existing one", func() {
			It("should return ErrDuplicate", func() {
				_, err := store.Create(usersTestCtx, "frank", "pass", false)
				Expect(err).NotTo(HaveOccurred())

				frank := "frank"
				_, err = store.Update(usersTestCtx, existing.ID, &frank, nil, nil)
				Expect(err).To(MatchError(users.ErrDuplicate))
			})
		})
	})

	// -------------------------------------------------------------------------
	// Store.Delete
	// -------------------------------------------------------------------------

	Describe("Delete", func() {
		Context("when deleting another user", func() {
			It("should remove the user from the database", func() {
				target, err := store.Create(usersTestCtx, "grace", "pass", false)
				Expect(err).NotTo(HaveOccurred())
				caller, err := store.Create(usersTestCtx, "admin-user", "pass", true)
				Expect(err).NotTo(HaveOccurred())

				err = store.Delete(usersTestCtx, target.ID, caller.ID)
				Expect(err).NotTo(HaveOccurred())

				_, err = store.GetByID(usersTestCtx, target.ID)
				Expect(err).To(MatchError(users.ErrNotFound))
			})
		})

		Context("when attempting to delete your own account", func() {
			It("should return ErrCannotDeleteSelf", func() {
				u, err := store.Create(usersTestCtx, "self-user", "pass", false)
				Expect(err).NotTo(HaveOccurred())

				err = store.Delete(usersTestCtx, u.ID, u.ID)
				Expect(err).To(MatchError(users.ErrCannotDeleteSelf))
			})
		})

		Context("when the user does not exist", func() {
			It("should return ErrNotFound", func() {
				err := store.Delete(usersTestCtx, "00000000-0000-0000-0000-000000000000", "other-id")
				Expect(err).To(MatchError(users.ErrNotFound))
			})
		})
	})

	// -------------------------------------------------------------------------
	// Store.AddGrant / ListGrants / DeleteGrant
	// -------------------------------------------------------------------------

	Describe("Grant CRUD", func() {
		var owner *users.User

		BeforeEach(func() {
			var err error
			owner, err = store.Create(usersTestCtx, "grant-owner", "pass", false)
			Expect(err).NotTo(HaveOccurred())
		})

		Describe("AddGrant", func() {
			Context("when adding a valid grant", func() {
				It("should return a populated Grant", func() {
					g, err := store.AddGrant(usersTestCtx, owner.ID, "read", "orders.*")
					Expect(err).NotTo(HaveOccurred())
					Expect(g.ID).NotTo(BeEmpty())
					Expect(g.UserID).To(Equal(owner.ID))
					Expect(g.Action).To(Equal("read"))
					Expect(g.TopicPattern).To(Equal("orders.*"))
					Expect(g.CreatedAt).NotTo(BeZero())
				})
			})
		})

		Describe("ListGrants", func() {
			Context("when the user has multiple grants", func() {
				It("should return all grants for that user in creation order", func() {
					_, err := store.AddGrant(usersTestCtx, owner.ID, "read", "orders.*")
					Expect(err).NotTo(HaveOccurred())
					_, err = store.AddGrant(usersTestCtx, owner.ID, "write", "payments")
					Expect(err).NotTo(HaveOccurred())

					grants, err := store.ListGrants(usersTestCtx, owner.ID)
					Expect(err).NotTo(HaveOccurred())
					Expect(grants).To(HaveLen(2))
				})
			})

			Context("when the user has no grants", func() {
				It("should return an empty slice without error", func() {
					grants, err := store.ListGrants(usersTestCtx, owner.ID)
					Expect(err).NotTo(HaveOccurred())
					Expect(grants).To(BeEmpty())
				})
			})
		})

		Describe("DeleteGrant", func() {
			Context("when the grant exists for that user", func() {
				It("should remove it so it no longer appears in ListGrants", func() {
					g, err := store.AddGrant(usersTestCtx, owner.ID, "read", "*")
					Expect(err).NotTo(HaveOccurred())

					err = store.DeleteGrant(usersTestCtx, g.ID, owner.ID)
					Expect(err).NotTo(HaveOccurred())

					grants, err := store.ListGrants(usersTestCtx, owner.ID)
					Expect(err).NotTo(HaveOccurred())
					Expect(grants).To(BeEmpty())
				})
			})

			Context("when the grant does not exist", func() {
				It("should return ErrNotFound", func() {
					err := store.DeleteGrant(usersTestCtx, "00000000-0000-0000-0000-000000000000", owner.ID)
					Expect(err).To(MatchError(users.ErrNotFound))
				})
			})

			Context("when the grant belongs to a different user", func() {
				It("should return ErrNotFound (grant/user mismatch)", func() {
					other, err := store.Create(usersTestCtx, "other-owner", "pass", false)
					Expect(err).NotTo(HaveOccurred())

					g, err := store.AddGrant(usersTestCtx, other.ID, "read", "*")
					Expect(err).NotTo(HaveOccurred())

					// Attempt to delete it using owner.ID — should not find the row.
					err = store.DeleteGrant(usersTestCtx, g.ID, owner.ID)
					Expect(err).To(MatchError(users.ErrNotFound))
				})
			})
		})
	})
})
