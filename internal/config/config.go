package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"

	"service/internal/kafka"
	"service/internal/postgres"
	"service/internal/redis"
)

type Config struct {
	ServiceName     string        `mapstructure:"service_name"`
	HTTPPort        string        `mapstructure:"http_port"`
	LogLevel        string        `mapstructure:"log_level"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`

	TracingEnabled bool   `mapstructure:"tracing_enabled"`
	OTLPEndpoint   string `mapstructure:"otlp_endpoint"`

	Postgres postgres.Config      `mapstructure:"postgres"`
	Redis    redis.Config         `mapstructure:"redis"`
	Kafka    kafka.ProducerConfig `mapstructure:"kafka"`
}

func Load() Config {
	v := viper.New()

	v.SetDefault("service_name", "service")
	v.SetDefault("http_port", "8080")
	v.SetDefault("log_level", "info")
	v.SetDefault("shutdown_timeout", 15*time.Second)
	v.SetDefault("tracing_enabled", true)
	v.SetDefault("otlp_endpoint", "localhost:4317")

	v.SetDefault("postgres.dsn", "postgres://postgres:postgres@localhost:5432/service?sslmode=disable")
	v.SetDefault("postgres.max_conns", 10)
	v.SetDefault("postgres.min_conns", 2)

	v.SetDefault("redis.addr", "localhost:6379")
	v.SetDefault("redis.db", 0)
	v.SetDefault("redis.pool_size", 10)

	v.SetDefault("kafka.brokers", []string{"localhost:9092"})

	configName := "config.local"
	if name := os.Getenv("CONFIG_NAME"); name != "" {
		configName = name
	}

	v.SetConfigName(configName)
	v.SetConfigType("yaml")
	v.AddConfigPath("./configs")
	v.AddConfigPath(".")
	v.AddConfigPath("/etc/service/")

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) {
			slog.Warn("config file not found, using defaults and env vars", "config", configName)
		} else {
			panic(fmt.Sprintf("failed to read config file: %s", err))
		}
	}

	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		panic("failed to unmarshal config: " + err.Error())
	}

	return cfg
}
