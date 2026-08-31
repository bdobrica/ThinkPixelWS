package shared

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func TestUUIDv7UsesClockAndCanonicalRoundTrip(t *testing.T) {
	wantTime := time.Date(2026, 8, 31, 12, 34, 56, 789_000_000, time.FixedZone("test", 3*60*60))
	id, err := NewUUIDv7From(fixedClock{wantTime}, bytes.NewReader(make([]byte, 10)))
	if err != nil {
		t.Fatal(err)
	}
	if got := id.Time(); !got.Equal(wantTime) {
		t.Fatalf("timestamp = %v, want %v", got, wantTime)
	}
	if id[6]>>4 != 7 || id[8]>>6 != 2 {
		t.Fatalf("invalid version or variant in %s", id)
	}
	parsed, err := ParseUUIDv7(id.String())
	if err != nil || parsed != id {
		t.Fatalf("round trip = %v, %v", parsed, err)
	}
	if _, err := ParseUUIDv7("550e8400-e29b-41d4-a716-446655440000"); err == nil {
		t.Fatal("accepted non-v7 UUID")
	}
}

func TestBoundedStringValidation(t *testing.T) {
	if got, err := NewBoundedString("café", 1, 4); err != nil || got.String() != "café" {
		t.Fatalf("valid string = %q, %v", got, err)
	}
	for _, value := range []string{"cafe\u0301", "a\n", "too-long"} {
		if _, err := NewBoundedString(value, 1, 4); err == nil {
			t.Fatalf("accepted invalid value %q", value)
		}
	}
}

func TestSHA256DigestCanonicalRoundTrip(t *testing.T) {
	digest := DigestBytes([]byte("thinkpixel"))
	parsed, err := ParseSHA256Digest(digest.String())
	if err != nil || parsed != digest {
		t.Fatalf("round trip = %v, %v", parsed, err)
	}
	if _, err := ParseSHA256Digest("sha256:ABC"); err == nil {
		t.Fatal("accepted non-canonical digest")
	}
}

func TestTypedErrorSupportsWrapping(t *testing.T) {
	cause := errors.New("database detail")
	err := fmt.Errorf("outer: %w", WrapError(CodeConflict, "head changed", cause))
	if got := ErrorCodeOf(err); got != CodeConflict {
		t.Fatalf("code = %q", got)
	}
	if !errors.Is(err, cause) {
		t.Fatal("cause is not discoverable")
	}
	if got := err.Error(); got != "outer: conflict: head changed" {
		t.Fatalf("safe error = %q", got)
	}
}
