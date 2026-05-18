package messaging

import (
	"encoding/json"
	"testing"
)

func TestReplySubject(t *testing.T) {
	t.Parallel()

	if got := ReplySubject(TypeCapture); got != SubjectCaptureReply {
		t.Fatalf("capture: got %q, want %q", got, SubjectCaptureReply)
	}
	if got := ReplySubject(TypeRefund); got != SubjectRefundReply {
		t.Fatalf("refund: got %q, want %q", got, SubjectRefundReply)
	}
	if got := ReplySubject("anything-else"); got != SubjectCaptureReply {
		t.Fatalf("default: got %q, want %q", got, SubjectCaptureReply)
	}
}

func TestCommandRoundTrip(t *testing.T) {
	t.Parallel()

	in := Command{
		MsgID:          "m-1",
		SagaID:         "s-1",
		IdempotencyKey: "idem-1",
		Type:           TypeCapture,
		Payload:        PaymentPayload{AccountID: "acc-1", Amount: 1500, Currency: "THB"},
	}

	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out Command
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != in {
		t.Fatalf("round trip mismatch: got %+v, want %+v", out, in)
	}
}
