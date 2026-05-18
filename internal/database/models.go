package database

import (
	"encoding/json"
	"time"

	"github.com/uptrace/bun"
)

// Inbox dedups consumed command messages by their msg_id.
type Inbox struct {
	bun.BaseModel `bun:"table:payment_inbox"`

	MsgID      string    `bun:"msg_id,pk"`
	SagaID     string    `bun:"saga_id,notnull"`
	ReceivedAt time.Time `bun:"received_at,nullzero,notnull,default:now()"`
}

// Payment is the idempotent business record, unique on idempotency_key.
type Payment struct {
	bun.BaseModel `bun:"table:payments"`

	ID             string    `bun:"id,pk"`
	IdempotencyKey string    `bun:"idempotency_key,notnull"`
	SagaID         string    `bun:"saga_id,notnull"`
	AccountID      string    `bun:"account_id,notnull"`
	Amount         int64     `bun:"amount,notnull"`
	Currency       string    `bun:"currency,notnull"`
	Status         string    `bun:"status,notnull"`
	CreatedAt      time.Time `bun:"created_at,nullzero,notnull,default:now()"`
	UpdatedAt      time.Time `bun:"updated_at,nullzero,notnull,default:now()"`
}

// Outbox holds reply messages to be published by the relay.
type Outbox struct {
	bun.BaseModel `bun:"table:payment_outbox"`

	ID        string          `bun:"id,pk"`
	Topic     string          `bun:"topic,notnull"`
	MsgID     string          `bun:"msg_id,notnull"`
	Payload   json.RawMessage `bun:"payload,type:jsonb,notnull"`
	Status    string          `bun:"status,notnull"`
	CreatedAt time.Time       `bun:"created_at,nullzero,notnull,default:now()"`
	SentAt    *time.Time      `bun:"sent_at"`
}

// Models returns all bun models registered with the ORM.
func Models() []any {
	return []any{(*Inbox)(nil), (*Payment)(nil), (*Outbox)(nil)}
}

// Payment statuses persisted in the payments table.
const (
	PaymentCaptured = "CAPTURED"
	PaymentRefunded = "REFUNDED"
)

// Outbox row statuses.
const (
	OutboxPending = "PENDING"
	OutboxSent    = "SENT"
)
