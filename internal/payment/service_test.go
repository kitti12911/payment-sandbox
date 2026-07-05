package payment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	orm "github.com/kitti12911/lib-orm/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"github.com/kitti12911/payment-sandbox/internal/messaging"
)

// newTestService wires the Service onto a sqlmock-backed bun DB so every SQL
// statement is scripted. bun emits the inserts as INSERT ... RETURNING (the
// models carry default:now() columns), so inserts are Query expectations whose
// returned row count doubles as RowsAffected. Ordered expectations also prove
// what the transaction does NOT run (e.g. applyEffect on duplicates).
func newTestService(t *testing.T) (*Service, sqlmock.Sqlmock) {
	t.Helper()
	sqldb, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqldb.Close() })

	db := orm.Wrap(bun.NewDB(sqldb, pgdialect.New()))
	return New(db), mock
}

func captureCmd() messaging.Command {
	return messaging.Command{
		MsgID:          "m1",
		SagaID:         "s1",
		IdempotencyKey: "ik1",
		Type:           messaging.TypeCapture,
		Payload:        messaging.PaymentPayload{AccountID: "acc1", Amount: 1500, Currency: "THB"},
	}
}

func inboxInserted(mock sqlmock.Sqlmock, rows int) {
	returned := sqlmock.NewRows([]string{"received_at"})
	if rows > 0 {
		returned.AddRow(time.Now())
	}
	mock.ExpectQuery(`INSERT INTO "payment_inbox" .* ON CONFLICT \(msg_id\) DO NOTHING RETURNING`).
		WillReturnRows(returned)
}

func paymentInserted(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`INSERT INTO "payments" .* ON CONFLICT \(idempotency_key\) DO NOTHING RETURNING`).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(time.Now(), time.Now()))
}

func replyEnqueued(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`INSERT INTO "payment_outbox" .* ON CONFLICT \(msg_id\) DO NOTHING RETURNING`).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(time.Now()))
}

func TestHandleFirstDeliveryCaptures(t *testing.T) {
	svc, mock := newTestService(t)

	mock.ExpectBegin()
	inboxInserted(mock, 1)
	paymentInserted(mock)
	replyEnqueued(mock)
	mock.ExpectCommit()

	require.NoError(t, svc.Handle(context.Background(), captureCmd()))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestHandleDuplicateSkipsEffectButEnqueuesReply(t *testing.T) {
	svc, mock := newTestService(t)

	mock.ExpectBegin()
	// inbox conflict -> zero returned rows -> duplicate delivery
	inboxInserted(mock, 0)
	// no payments statement: the ordered expectations jump straight to the outbox
	replyEnqueued(mock)
	mock.ExpectCommit()

	require.NoError(t, svc.Handle(context.Background(), captureCmd()))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestHandleRefundUpdatesCapturedPayment(t *testing.T) {
	svc, mock := newTestService(t)
	cmd := captureCmd()
	cmd.Type = messaging.TypeRefund

	mock.ExpectBegin()
	inboxInserted(mock, 1)
	mock.ExpectExec(`UPDATE "payments" AS "payment" SET status = .* WHERE \(idempotency_key = .*\) AND \(status = .*\)`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	replyEnqueued(mock)
	mock.ExpectCommit()

	require.NoError(t, svc.Handle(context.Background(), cmd))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestHandleInboxInsertError(t *testing.T) {
	svc, mock := newTestService(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "payment_inbox"`).WillReturnError(errors.New("boom"))
	mock.ExpectRollback()

	err := svc.Handle(context.Background(), captureCmd())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "handle payment command")
	assert.Contains(t, err.Error(), "inbox insert")
}

func TestHandleCaptureError(t *testing.T) {
	svc, mock := newTestService(t)

	mock.ExpectBegin()
	inboxInserted(mock, 1)
	mock.ExpectQuery(`INSERT INTO "payments"`).WillReturnError(errors.New("boom"))
	mock.ExpectRollback()

	err := svc.Handle(context.Background(), captureCmd())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "capture payment")
}

func TestHandleRefundError(t *testing.T) {
	svc, mock := newTestService(t)
	cmd := captureCmd()
	cmd.Type = messaging.TypeRefund

	mock.ExpectBegin()
	inboxInserted(mock, 1)
	mock.ExpectExec(`UPDATE "payments"`).WillReturnError(errors.New("boom"))
	mock.ExpectRollback()

	err := svc.Handle(context.Background(), cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refund payment")
}

func TestHandleReplyOutboxError(t *testing.T) {
	svc, mock := newTestService(t)

	mock.ExpectBegin()
	inboxInserted(mock, 1)
	paymentInserted(mock)
	mock.ExpectQuery(`INSERT INTO "payment_outbox"`).WillReturnError(errors.New("boom"))
	mock.ExpectRollback()

	err := svc.Handle(context.Background(), captureCmd())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reply outbox insert")
}
