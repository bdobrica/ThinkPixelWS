package clock

import (
	"testing"
	"time"
)

func TestSystemReturnsUTC(t *testing.T) {
	before := time.Now().UTC()
	got := (System{}).Now()
	after := time.Now().UTC()
	if got.Location() != time.UTC || got.Before(before) || got.After(after) {
		t.Fatalf("Now() = %v", got)
	}
}
