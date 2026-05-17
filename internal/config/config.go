package config

import (
	"time"

	async "github.com/kitti12911/lib-async"
	"github.com/kitti12911/lib-monitor/profiling"
	"github.com/kitti12911/lib-monitor/tracing"
	liborm "github.com/kitti12911/lib-orm/v3"
	"github.com/kitti12911/lib-util/v3/logger"
)

// Config is the payment participant configuration.
type Config struct {
	Service   Service          `mapstructure:"service"   validate:"required"`
	Logging   logger.Config    `mapstructure:"logging"`
	Tracing   tracing.Config   `mapstructure:"tracing"`
	Profiling profiling.Config `mapstructure:"profiling"`
	Database  liborm.Config    `mapstructure:"database"  validate:"required"`
	NATS      async.NATSConfig `mapstructure:"nats"      validate:"required"`
	Relay     Relay            `mapstructure:"relay"`
}

// Service holds process-level settings.
type Service struct {
	Name            string        `mapstructure:"name"             env:"SERVICE_NAME"      validate:"required"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout" env:"SHUTDOWN_TIMEOUT"`
}

// Relay controls the outbox publishing loop.
type Relay struct {
	Interval  time.Duration `mapstructure:"interval"   env:"RELAY_INTERVAL"`
	BatchSize int           `mapstructure:"batch_size" env:"RELAY_BATCH_SIZE"`
}
