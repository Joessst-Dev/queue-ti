package config

import (
	"errors"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig
	DB       DBConfig
	Queue    QueueConfig
	Auth     AuthConfig
	LogLevel string `mapstructure:"log_level"`
}

type ServerConfig struct {
	Port     int
	HTTPPort int `mapstructure:"http_port"`
}

type DBConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	SSLMode  string
}

type QueueConfig struct {
	VisibilityTimeout        time.Duration `mapstructure:"visibility_timeout"`
	MaxRetries               int           `mapstructure:"max_retries"`
	MessageTTL               time.Duration `mapstructure:"message_ttl"`
	DLQThreshold             int           `mapstructure:"dlq_threshold"`
	RequireTopicRegistration bool          `mapstructure:"require_topic_registration"`
	DeleteReaperSchedule     string        `mapstructure:"delete_reaper_schedule"`
}

type AuthConfig struct {
	Enabled   bool
	Username  string
	Password  string
	JWTSecret string `mapstructure:"jwt_secret"`
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/etc/queueti/")

	viper.SetEnvPrefix("QUEUETI")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	viper.SetDefault("server.port", 50051)
	viper.SetDefault("server.http_port", 8080)
	viper.SetDefault("db.host", "localhost")
	viper.SetDefault("db.port", 5432)
	viper.SetDefault("db.user", "postgres")
	viper.SetDefault("db.password", "postgres")
	viper.SetDefault("db.name", "queueti")
	viper.SetDefault("db.sslmode", "disable")
	viper.SetDefault("queue.visibility_timeout", "30s")
	viper.SetDefault("queue.max_retries", 3)
	viper.SetDefault("queue.message_ttl", "24h")
	viper.SetDefault("queue.dlq_threshold", 3)
	viper.SetDefault("queue.require_topic_registration", false)
	viper.SetDefault("queue.delete_reaper_schedule", "")
	viper.SetDefault("auth.enabled", false)
	viper.SetDefault("auth.username", "")
	viper.SetDefault("auth.password", "")
	viper.SetDefault("auth.jwt_secret", "")
	viper.SetDefault("log_level", "info")

	// Explicitly bind environment variables to config keys
	_ = viper.BindEnv("server.port")
	_ = viper.BindEnv("server.http_port")
	_ = viper.BindEnv("db.host")
	_ = viper.BindEnv("db.port")
	_ = viper.BindEnv("db.user")
	_ = viper.BindEnv("db.password")
	_ = viper.BindEnv("db.name")
	_ = viper.BindEnv("db.sslmode")
	_ = viper.BindEnv("queue.visibility_timeout")
	_ = viper.BindEnv("queue.max_retries")
	_ = viper.BindEnv("queue.message_ttl")
	_ = viper.BindEnv("queue.dlq_threshold")
	_ = viper.BindEnv("queue.require_topic_registration")
	_ = viper.BindEnv("queue.delete_reaper_schedule")
	_ = viper.BindEnv("auth.enabled")
	_ = viper.BindEnv("auth.username")
	_ = viper.BindEnv("auth.password")
	_ = viper.BindEnv("auth.jwt_secret")
	_ = viper.BindEnv("log_level")

	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return nil, err
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
