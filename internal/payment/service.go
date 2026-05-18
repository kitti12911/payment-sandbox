// Package payment implements the saga participant: it consumes capture/refund
// commands, applies the effect exactly once, and emits a reply via the outbox.
package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	orm "github.com/kitti12911/lib-orm/v3"

	"github.com/kitti12911/payment-sandbox/internal/database"
	"github.com/kitti12911/payment-sandbox/internal/messaging"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Service applies payment commands with effectively-once semantics:
// at-least-once delivery + inbox dedup + idempotent business effect.
type Service struct {
	db *orm.DB
}

// New returns a payment Service bound to the given database.
func New(db *orm.DB) *Service {
	return &Service{db: db}
}

// Handle processes one capture/refund command inside a single transaction:
// inbox dedup + idempotent payment effect + reply outbox row are all atomic.
func (s *Service) Handle(ctx context.Context, cmd messaging.Command) error {
	err := s.db.Transaction(ctx, func(ctx context.Context) error {
		idb := s.db.IDB(ctx)

		firstDelivery, err := insertInbox(ctx, idb, cmd)
		if err != nil {
			return err
		}

		if firstDelivery {
			if applyErr := applyEffect(ctx, idb, cmd); applyErr != nil {
				return applyErr
			}
		}

		// Always (re-)ensure a reply row exists so a lost reply is
		// redelivered by the relay even when the command is a duplicate.
		if replyErr := enqueueReply(ctx, idb, cmd); replyErr != nil {
			return replyErr
		}

		slog.InfoContext(ctx, "payment command handled",
			"saga_id", cmd.SagaID, "type", cmd.Type,
			"msg_id", cmd.MsgID, "first_delivery", firstDelivery)
		return nil
	})
	if err != nil {
		return fmt.Errorf("handle payment command: %w", err)
	}
	return nil
}

// insertInbox records the message id; a conflict means a duplicate delivery.
func insertInbox(ctx context.Context, idb bun.IDB, cmd messaging.Command) (bool, error) {
	res, err := idb.NewInsert().
		Model(&database.Inbox{MsgID: cmd.MsgID, SagaID: cmd.SagaID}).
		On("CONFLICT (msg_id) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("inbox insert: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inbox rows affected: %w", err)
	}
	return affected == 1, nil
}

// applyEffect performs the idempotent business mutation for the command.
func applyEffect(ctx context.Context, idb bun.IDB, cmd messaging.Command) error {
	switch cmd.Type {
	case messaging.TypeRefund:
		_, err := idb.NewUpdate().
			Model((*database.Payment)(nil)).
			Set("status = ?", database.PaymentRefunded).
			Set("updated_at = now()").
			Where("idempotency_key = ?", cmd.IdempotencyKey).
			Where("status = ?", database.PaymentCaptured).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("refund payment: %w", err)
		}
		return nil
	default:
		_, err := idb.NewInsert().
			Model(&database.Payment{
				ID:             uuid.NewString(),
				IdempotencyKey: cmd.IdempotencyKey,
				SagaID:         cmd.SagaID,
				AccountID:      cmd.Payload.AccountID,
				Amount:         cmd.Payload.Amount,
				Currency:       cmd.Payload.Currency,
				Status:         database.PaymentCaptured,
			}).
			On("CONFLICT (idempotency_key) DO NOTHING").
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("capture payment: %w", err)
		}
		return nil
	}
}

// enqueueReply writes the reply into the outbox idempotently (deterministic
// reply msg id keyed off the command msg id).
func enqueueReply(ctx context.Context, idb bun.IDB, cmd messaging.Command) error {
	replyID := "r-" + cmd.MsgID
	reply := messaging.Reply{
		MsgID:     replyID,
		SagaID:    cmd.SagaID,
		InReplyTo: cmd.MsgID,
		Status:    messaging.StatusOK,
	}
	payload, err := json.Marshal(reply)
	if err != nil {
		return fmt.Errorf("marshal reply: %w", err)
	}

	_, err = idb.NewInsert().
		Model(&database.Outbox{
			ID:      uuid.NewString(),
			Topic:   messaging.ReplySubject(cmd.Type),
			MsgID:   replyID,
			Payload: payload,
			Status:  database.OutboxPending,
		}).
		On("CONFLICT (msg_id) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("reply outbox insert: %w", err)
	}
	return nil
}
