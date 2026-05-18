package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	async "github.com/kitti12911/lib-async"
	"github.com/kitti12911/lib-monitor/profiling"
	"github.com/kitti12911/lib-monitor/tracing"
	libconfig "github.com/kitti12911/lib-util/v3/config"
	"github.com/kitti12911/lib-util/v3/logger"

	"github.com/kitti12911/payment-sandbox/internal/config"
	"github.com/kitti12911/payment-sandbox/internal/database"
	"github.com/kitti12911/payment-sandbox/internal/messaging"
	"github.com/kitti12911/payment-sandbox/internal/payment"
	"github.com/kitti12911/payment-sandbox/internal/relay"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx := context.Background()

	cfg, err := libconfig.Load[config.Config]("config.yml")
	if err != nil {
		slog.ErrorContext(ctx, "failed to load config", "error", err)
		return 1
	}

	if cfg.Service.ShutdownTimeout == 0 {
		cfg.Service.ShutdownTimeout = 10 * time.Second
	}

	logger.NewFromConfig(cfg.Logging, cfg.Service.Name)

	profiler, err := profiling.NewFromConfig(cfg.Service.Name, cfg.Profiling)
	if err != nil {
		slog.ErrorContext(ctx, "failed to init profiling", "error", err)
		return 1
	}
	defer func() {
		if shutdownErr := profiling.Shutdown(profiler); shutdownErr != nil {
			slog.ErrorContext(ctx, "failed to stop profiling", "error", shutdownErr)
		}
	}()

	tp, err := tracing.NewFromConfig(ctx, cfg.Service.Name, cfg.Tracing)
	if err != nil {
		slog.ErrorContext(ctx, "failed to init tracing", "error", err)
		return 1
	}
	defer func() {
		if shutdownErr := tracing.Shutdown(ctx, tp); shutdownErr != nil {
			slog.ErrorContext(ctx, "failed to stop tracing", "error", shutdownErr)
		}
	}()

	db, err := database.New(ctx, cfg)
	if err != nil {
		slog.ErrorContext(ctx, "failed to init database", "error", err)
		return 1
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			slog.ErrorContext(ctx, "failed to close database", "error", closeErr)
		}
	}()

	bus, err := async.NewNATS(cfg.NATS, nil)
	if err != nil {
		slog.ErrorContext(ctx, "failed to connect to nats", "error", err)
		return 1
	}
	defer func() {
		if closeErr := bus.Close(); closeErr != nil {
			slog.ErrorContext(ctx, "failed to close nats bus", "error", closeErr)
		}
	}()

	svc := payment.New(db)
	rly := relay.New(db, bus, cfg.Relay.Interval, cfg.Relay.BatchSize)

	workerCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errs := make(chan error, 3)
	go func() { errs <- consume(workerCtx, bus, messaging.SubjectCaptureCmd, svc) }()
	go func() { errs <- consume(workerCtx, bus, messaging.SubjectRefundCmd, svc) }()
	go func() { errs <- rly.Run(workerCtx) }()

	slog.InfoContext(ctx, "payment participant started",
		"capture", messaging.SubjectCaptureCmd, "refund", messaging.SubjectRefundCmd)

	select {
	case <-workerCtx.Done():
	case err := <-errs:
		if err != nil {
			slog.ErrorContext(ctx, "payment participant stopped with error", "error", err)
			stop()
			return 1
		}
	}

	slog.InfoContext(ctx, "shutting down payment participant")
	stop()

	shutdownCtx, cancel := context.WithTimeout(ctx, cfg.Service.ShutdownTimeout)
	defer cancel()
	<-shutdownCtx.Done()

	slog.InfoContext(ctx, "payment participant stopped")
	return 0
}

func consume(ctx context.Context, bus *async.Bus, subject string, svc *payment.Service) error {
	err := async.Consume(
		ctx,
		bus.Subscriber(),
		async.JSONCodec{},
		subject,
		func(ctx context.Context, msg async.Envelope[messaging.Command]) error {
			return svc.Handle(ctx, msg.Payload)
		},
		async.WithErrorHandler(func(ctx context.Context, msg async.Envelope[[]byte], handlerErr error) {
			slog.ErrorContext(ctx, "failed to process payment command",
				"subject", subject, "message_uuid", msg.UUID, "error", handlerErr)
		}),
	)
	if err != nil {
		return fmt.Errorf("consume %s: %w", subject, err)
	}
	return nil
}
