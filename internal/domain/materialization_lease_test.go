package domain

import (
	"github.com/google/uuid"
	"math"
	"strings"
	"testing"
	"time"
)

func TestMaterializationLease(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.FixedZone("local", 7200))
	m, err := materializationInput().Materialization(now)
	if err != nil {
		t.Fatal(err)
	}
	input := NewMaterializationLease{ID: uuid.Must(uuid.NewV7()), FencingToken: 1, Holder: "execution-ref"}
	m.Mode = MaterializationReadOnly
	if _, err := input.MaterializationLease(m, now); err == nil {
		t.Fatal("read-only lease accepted")
	}
	m.Mode = MaterializationReadWrite
	lease, err := input.MaterializationLease(m, now)
	if err != nil {
		t.Fatal(err)
	}
	if lease.TenantID != m.TenantID || lease.WorkspaceID != m.WorkspaceID || lease.MaterializationID != m.ID ||
		lease.ID != input.ID || lease.FencingToken != 1 || lease.Holder != input.Holder || lease.ReleasedAt != nil ||
		!lease.IssuedAt.Equal(now) || lease.RenewedAt != lease.IssuedAt || lease.IssuedAt.Location() != time.UTC ||
		lease.ExpiresAt.Sub(lease.IssuedAt) != 60*time.Second || DefaultMaterializationLeaseRenewalInterval != 20*time.Second {
		t.Fatalf("unexpected lease: %#v", lease)
	}
	if _, err := input.MaterializationLease(m, time.Time{}); err == nil {
		t.Fatal("zero issue time accepted")
	}
	for name, mutate := range map[string]func(*MaterializationLease){
		"tenant":          func(l *MaterializationLease) { l.TenantID = uuid.Nil },
		"id":              func(l *MaterializationLease) { l.ID = uuid.New() },
		"workspace":       func(l *MaterializationLease) { l.WorkspaceID = uuid.Nil },
		"materialization": func(l *MaterializationLease) { l.MaterializationID = uuid.Nil },
		"zero fence":      func(l *MaterializationLease) { l.FencingToken = 0 },
		"overflow fence":  func(l *MaterializationLease) { l.FencingToken = math.MaxInt64 + 1 },
		"empty holder":    func(l *MaterializationLease) { l.Holder = "" },
		"padded holder":   func(l *MaterializationLease) { l.Holder = " padded" },
		"control holder":  func(l *MaterializationLease) { l.Holder = "control\n" },
		"long holder":     func(l *MaterializationLease) { l.Holder = strings.Repeat("x", 257) },
		"invalid utf8":    func(l *MaterializationLease) { l.Holder = string([]byte{0xff}) },
		"zero issued":     func(l *MaterializationLease) { l.IssuedAt = time.Time{} },
		"early renewal":   func(l *MaterializationLease) { l.RenewedAt = now.Add(-time.Second) },
		"expiry boundary": func(l *MaterializationLease) { l.ExpiresAt = l.RenewedAt },
		"early release":   func(l *MaterializationLease) { at := now.Add(-time.Second); l.ReleasedAt = &at },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := lease
			mutate(&invalid)
			if invalid.Validate() == nil {
				t.Fatal("invalid lease accepted")
			}
		})
	}
	lease.Holder = strings.Repeat("é", 256)
	lease.FencingToken = math.MaxInt64
	lease.RenewedAt = lease.IssuedAt.Add(20 * time.Second)
	lease.ExpiresAt = lease.RenewedAt.Add(DefaultMaterializationLeaseDuration)
	released := lease.ExpiresAt.Add(time.Second)
	lease.ReleasedAt = &released
	if err := lease.Validate(); err != nil {
		t.Fatalf("valid historical lease: %v", err)
	}
}
