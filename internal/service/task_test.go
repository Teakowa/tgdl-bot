package service

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestNewIdempotencyKey(t *testing.T) {
	got := NewIdempotencyKey(123, "https://t.me/c/999/42", "channel_name", true)
	sum := sha256.Sum256([]byte("123|https://t.me/c/999/42|channel_name|drop-caption"))
	want := hex.EncodeToString(sum[:])

	if got != want {
		t.Fatalf("idempotency key mismatch: got %q want %q", got, want)
	}
}

func TestNewIdempotencyKeyVariesByTargetAndCaptionMode(t *testing.T) {
	a := NewIdempotencyKey(123, "https://t.me/c/999/42", "channel_a", false)
	b := NewIdempotencyKey(123, "https://t.me/c/999/42", "channel_b", false)
	c := NewIdempotencyKey(123, "https://t.me/c/999/42", "channel_a", true)

	if a == b {
		t.Fatal("expected different target peers to produce different keys")
	}
	if a == c {
		t.Fatal("expected different caption modes to produce different keys")
	}
}

func TestStatusValid(t *testing.T) {
	cases := map[Status]bool{
		StatusQueued:       true,
		StatusRunning:      true,
		StatusDone:         true,
		StatusFailed:       true,
		StatusRetrying:     true,
		StatusDeadLettered: true,
		Status("bogus"):    false,
	}

	for status, want := range cases {
		if got := status.Valid(); got != want {
			t.Fatalf("status %q validity mismatch: got %v want %v", status, got, want)
		}
	}

	if !IsValidStatus("done") {
		t.Fatal("expected done to be valid")
	}
	if IsValidStatus("not-a-status") {
		t.Fatal("expected invalid status to be rejected")
	}
}
