package config_test

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"

	"github.com/Joessst-Dev/queue-ti/internal/config"
)

var _ = Describe("Load", func() {
	BeforeEach(func() {
		viper.Reset()
	})

	Context("when no config file or env vars are present", func() {
		It("returns all default values", func() {
			cfg, err := config.Load()

			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Server.Port).To(Equal(50051))
			Expect(cfg.Server.HTTPPort).To(Equal(8080))
			Expect(cfg.DB.Host).To(Equal("localhost"))
			Expect(cfg.DB.Port).To(Equal(5432))
			Expect(cfg.DB.User).To(Equal("postgres"))
			Expect(cfg.DB.Password).To(Equal("postgres"))
			Expect(cfg.DB.Name).To(Equal("queueti"))
			Expect(cfg.DB.SSLMode).To(Equal("disable"))
			Expect(cfg.Queue.VisibilityTimeout).To(Equal(30 * time.Second))
			Expect(cfg.Queue.MaxRetries).To(Equal(3))
			Expect(cfg.Queue.MessageTTL).To(Equal(24 * time.Hour))
			Expect(cfg.Queue.DLQThreshold).To(Equal(3))
			Expect(cfg.Auth.Enabled).To(BeFalse())
			Expect(cfg.Auth.Username).To(BeEmpty())
			Expect(cfg.Auth.Password).To(BeEmpty())
		})
	})

	Context("when environment variables are set", func() {
		AfterEach(func() {
			os.Unsetenv("QUEUETI_SERVER_PORT")
			os.Unsetenv("QUEUETI_SERVER_HTTP_PORT")
			os.Unsetenv("QUEUETI_DB_HOST")
			os.Unsetenv("QUEUETI_DB_PORT")
			os.Unsetenv("QUEUETI_DB_USER")
			os.Unsetenv("QUEUETI_DB_PASSWORD")
			os.Unsetenv("QUEUETI_DB_NAME")
			os.Unsetenv("QUEUETI_DB_SSLMODE")
			os.Unsetenv("QUEUETI_QUEUE_VISIBILITY_TIMEOUT")
			os.Unsetenv("QUEUETI_QUEUE_MAX_RETRIES")
			os.Unsetenv("QUEUETI_QUEUE_MESSAGE_TTL")
			os.Unsetenv("QUEUETI_QUEUE_DLQ_THRESHOLD")
			os.Unsetenv("QUEUETI_AUTH_ENABLED")
			os.Unsetenv("QUEUETI_AUTH_USERNAME")
			os.Unsetenv("QUEUETI_AUTH_PASSWORD")
		})

		It("overrides server ports", func() {
			os.Setenv("QUEUETI_SERVER_PORT", "9090")
			os.Setenv("QUEUETI_SERVER_HTTP_PORT", "9091")

			cfg, err := config.Load()

			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Server.Port).To(Equal(9090))
			Expect(cfg.Server.HTTPPort).To(Equal(9091))
		})

		It("overrides database connection settings", func() {
			os.Setenv("QUEUETI_DB_HOST", "db.example.com")
			os.Setenv("QUEUETI_DB_PORT", "5433")
			os.Setenv("QUEUETI_DB_USER", "admin")
			os.Setenv("QUEUETI_DB_PASSWORD", "secret")
			os.Setenv("QUEUETI_DB_NAME", "myqueue")
			os.Setenv("QUEUETI_DB_SSLMODE", "require")

			cfg, err := config.Load()

			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.DB.Host).To(Equal("db.example.com"))
			Expect(cfg.DB.Port).To(Equal(5433))
			Expect(cfg.DB.User).To(Equal("admin"))
			Expect(cfg.DB.Password).To(Equal("secret"))
			Expect(cfg.DB.Name).To(Equal("myqueue"))
			Expect(cfg.DB.SSLMode).To(Equal("require"))
		})

		It("overrides the visibility timeout as a duration", func() {
			os.Setenv("QUEUETI_QUEUE_VISIBILITY_TIMEOUT", "2m")

			cfg, err := config.Load()

			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Queue.VisibilityTimeout).To(Equal(2 * time.Minute))
		})

		It("overrides max_retries via QUEUETI_QUEUE_MAX_RETRIES", func() {
			os.Setenv("QUEUETI_QUEUE_MAX_RETRIES", "5")

			cfg, err := config.Load()

			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Queue.MaxRetries).To(Equal(5))
		})

		It("overrides dlq_threshold via QUEUETI_QUEUE_DLQ_THRESHOLD", func() {
			os.Setenv("QUEUETI_QUEUE_DLQ_THRESHOLD", "5")

			cfg, err := config.Load()

			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Queue.DLQThreshold).To(Equal(5))
		})

		It("overrides message_ttl via QUEUETI_QUEUE_MESSAGE_TTL", func() {
			os.Setenv("QUEUETI_QUEUE_MESSAGE_TTL", "48h")

			cfg, err := config.Load()

			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Queue.MessageTTL).To(Equal(48 * time.Hour))
		})

		It("enables auth and sets credentials", func() {
			os.Setenv("QUEUETI_AUTH_ENABLED", "true")
			os.Setenv("QUEUETI_AUTH_USERNAME", "user")
			os.Setenv("QUEUETI_AUTH_PASSWORD", "pass")

			cfg, err := config.Load()

			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Auth.Enabled).To(BeTrue())
			Expect(cfg.Auth.Username).To(Equal("user"))
			Expect(cfg.Auth.Password).To(Equal("pass"))
		})
	})

	Context("when a config file is present", func() {
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "queueti-config-test-*")
			Expect(err).NotTo(HaveOccurred())

			content := `
server:
  port: 7070
  http_port: 7071
db:
  host: filehost
  port: 5434
  user: fileuser
  password: filepass
  name: filedb
  sslmode: verify-full
queue:
  visibility_timeout: 1m
auth:
  enabled: true
  username: admin
  password: hunter2
`
			Expect(os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(content), 0o600)).To(Succeed())

			viper.AddConfigPath(tmpDir)
		})

		AfterEach(func() {
			os.RemoveAll(tmpDir)
		})

		It("loads all values from the file", func() {
			cfg, err := config.Load()

			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Server.Port).To(Equal(7070))
			Expect(cfg.Server.HTTPPort).To(Equal(7071))
			Expect(cfg.DB.Host).To(Equal("filehost"))
			Expect(cfg.DB.Port).To(Equal(5434))
			Expect(cfg.DB.User).To(Equal("fileuser"))
			Expect(cfg.DB.Password).To(Equal("filepass"))
			Expect(cfg.DB.Name).To(Equal("filedb"))
			Expect(cfg.DB.SSLMode).To(Equal("verify-full"))
			Expect(cfg.Queue.VisibilityTimeout).To(Equal(time.Minute))
			Expect(cfg.Auth.Enabled).To(BeTrue())
			Expect(cfg.Auth.Username).To(Equal("admin"))
			Expect(cfg.Auth.Password).To(Equal("hunter2"))
		})
	})
})
