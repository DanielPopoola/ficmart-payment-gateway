package config

import (
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/go-playground/validator"
	_ "github.com/joho/godotenv/autoload"
	"github.com/knadh/koanf"
	"github.com/knadh/koanf/providers/env"
)

type Config struct {
	Primary    Primary        `koanf:"primary"`
	Server     ServerConfig   `koanf:"server"`
	Database   DatabaseConfig `koanf:"database"`
	BankClient BankConfig     `koanf:"bank_client"`
	Retry      RetryConfig    `koanf:"retry"`
	Logger     LoggerConfig   `koanf:"logger"`
	Worker     WorkerConfig   `koanf:"worker"`
}

type WorkerConfig struct {
	Interval  time.Duration `koanf:"interval" validate:"required"`
	BatchSize int           `koanf:"batch_size" validate:"required"`
}

type Primary struct {
	Env string `koanf:"env" validate:"required"`
}

type ServerConfig struct {
	Port         string        `koanf:"port" validate:"required"`
	ReadTimeout  time.Duration `koanf:"read_timeout" validate:"required"`
	WriteTimeout time.Duration `koanf:"write_timeout" validate:"required"`
	IdleTimeout  time.Duration `koanf:"idle_timeout" validate:"required"`
}

type DatabaseConfig struct {
	Host            string        `koanf:"host" validate:"required"`
	Port            int           `koanf:"port" validate:"required"`
	User            string        `koanf:"user" validate:"required"`
	Password        string        `koanf:"password" validate:"required"`
	Name            string        `koanf:"name" validate:"required"`
	SSLMode         string        `koanf:"ssl_mode" validate:"required"`
	MaxOpenConns    int           `koanf:"max_open_conns" validate:"required"`
	MaxIdleConns    int           `koanf:"max_idle_conns" validate:"required"`
	ConnMaxLifetime time.Duration `koanf:"conn_max_lifetime" validate:"required"`
	ConnMaxIdleTime time.Duration `koanf:"conn_max_idle_time" validate:"required"`
}

type BankConfig struct {
	BankBaseURL     string        `koanf:"bank_base_url" validate:"required"`
	BankConnTimeout time.Duration `koanf:"bank_conn_timeout" validate:"required"`
}

type RetryConfig struct {
	BaseDelay  int32 `koanf:"base_delay"`
	MaxRetries int32 `koanf:"max_retries"`
}

type LoggerConfig struct {
	Level string `koanf:"level"`
}

func LoadConfig() (*Config, error) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	k := koanf.New(".")

	err := k.Load(env.Provider("GATEWAY_", ".", func(s string) string {
		return strings.ReplaceAll(
			strings.ToLower(strings.TrimPrefix(s, "GATEWAY_")),
			"__",
			".",
		)
	}), nil)
	if err != nil {
		logger.Error("failed to load environment variables", "error", err)
		return nil, err
	}

	mainConfig := &Config{}

	err = k.Unmarshal("", mainConfig)
	if err != nil {
		logger.Error("could not unmarshal main config", "error", err)
		return nil, err
	}

	validate := validator.New()

	err = validate.Struct(mainConfig)
	if err != nil {
		logger.Error("config validation failed", "error", err)
		return nil, err
	}

	return mainConfig, nil
}
