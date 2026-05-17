// Package database wires the lib-orm connection for the payment participant.
// Schema migrations are owned by migration-sandbox and run out of band; this
// service never runs migrations itself.
package database

import (
	"context"
	"fmt"

	"github.com/kitti12911/payment-sandbox/internal/config"

	orm "github.com/kitti12911/lib-orm/v3"
)

// New opens the payment database connection with models registered.
func New(ctx context.Context, cfg *config.Config) (*orm.DB, error) {
	db, err := orm.New(
		ctx,
		cfg.Database,
		orm.WithApplicationName(cfg.Service.Name),
		orm.WithModels(Models()...),
		orm.WithTracing(cfg.Tracing.Enabled),
	)
	if err != nil {
		return nil, fmt.Errorf("open payment database: %w", err)
	}
	return db, nil
}
