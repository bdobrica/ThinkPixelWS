package security

import (
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
)

type cursorClock struct{ now time.Time }

func (c *cursorClock) Now() time.Time { return c.now }

type cursorPayload struct {
	After string `json:"after"`
}

func TestCursorRoundTripAndAuthentication(t *testing.T) {
	clock := &cursorClock{now: time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)}
	codec, err := NewCursorCodec([]byte("0123456789abcdef0123456789abcdef"), clock)
	if err != nil {
		t.Fatal(err)
	}
	token, err := codec.Encode("tenant-a:workspaces", cursorPayload{After: "next"}, clock.now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(token, "next") {
		t.Fatal("cursor payload is not opaque")
	}
	var got cursorPayload
	if err := codec.Decode(token, "tenant-a:workspaces", &got); err != nil || got.After != "next" {
		t.Fatalf("decode = %+v, %v", got, err)
	}

	replacement := byte('A')
	if token[len(token)-1] == replacement {
		replacement = 'B'
	}
	tampered := token[:len(token)-1] + string(replacement)
	if err := codec.Decode(tampered, "tenant-a:workspaces", &got); shared.ErrorCodeOf(err) != shared.CodeInvalidArgument {
		t.Fatalf("tampered code = %q", shared.ErrorCodeOf(err))
	}
	if err := codec.Decode(token, "tenant-b:workspaces", &got); err == nil {
		t.Fatal("cursor was reusable across scope")
	}
}

func TestCursorExpiryAndLimits(t *testing.T) {
	clock := &cursorClock{now: time.Unix(100, 0).UTC()}
	codec, err := NewCursorCodec(make([]byte, 32), clock)
	if err != nil {
		t.Fatal(err)
	}
	token, err := codec.Encode("scope", cursorPayload{}, clock.now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	clock.now = clock.now.Add(time.Second)
	if err := codec.Decode(token, "scope", &cursorPayload{}); shared.ErrorCodeOf(err) != shared.CodeExpired {
		t.Fatalf("expired code = %q", shared.ErrorCodeOf(err))
	}
	if _, err := NewCursorCodec(make([]byte, 31), clock); err == nil {
		t.Fatal("accepted short authentication key")
	}
	if _, err := codec.Encode("scope", strings.Repeat("x", maxCursorPayload+1), clock.now.Add(time.Second)); err == nil {
		t.Fatal("accepted oversized payload")
	}
}
