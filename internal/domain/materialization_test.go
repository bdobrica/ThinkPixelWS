package domain

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewMaterialization(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.FixedZone("local", 7200))
	for _, mode := range []MaterializationMode{MaterializationReadOnly, MaterializationReadWrite} {
		input := materializationInput()
		input.Mode = mode
		m, err := input.Materialization(now)
		if err != nil {
			t.Fatal(err)
		}
		if err := m.Validate(); err != nil {
			t.Fatal(err)
		}
		if m.State != MaterializationRequested || m.StateVersion != 1 || m.CreatedAt.Location() != time.UTC || !m.CreatedAt.Equal(now) || !m.UpdatedAt.Equal(now) {
			t.Fatalf("unexpected initial metadata: %#v", m)
		}
	}
}

func TestMaterializationValidation(t *testing.T) {
	tests := map[string]func(*Materialization){
		"missing tenant":      func(m *Materialization) { m.TenantID = uuid.Nil },
		"v4 identity":         func(m *Materialization) { m.ID = uuid.New() },
		"invalid variant":     func(m *Materialization) { m.ID[8] = 0 },
		"missing workspace":   func(m *Materialization) { m.WorkspaceID = uuid.Nil },
		"zero generation":     func(m *Materialization) { m.BaseGeneration = 0 },
		"generation overflow": func(m *Materialization) { m.BaseGeneration = math.MaxUint64 },
		"provider":            func(m *Materialization) { m.Provider = "provider/name" },
		"target":              func(m *Materialization) { m.Target.ID = "" },
		"region whitespace":   func(m *Materialization) { m.Target.Region = " eu" },
		"storage control":     func(m *Materialization) { m.Target.StorageClass = "ssd\n" },
		"architecture length": func(m *Materialization) { m.Target.Architecture = strings.Repeat("x", 33) },
		"mode":                func(m *Materialization) { m.Mode = "writable" },
		"state":               func(m *Materialization) { m.State = "COMPLETE" },
		"version":             func(m *Materialization) { m.StateVersion = 0 },
		"version overflow":    func(m *Materialization) { m.StateVersion = math.MaxUint64 },
		"creation":            func(m *Materialization) { m.CreatedAt = time.Time{} },
		"update":              func(m *Materialization) { m.UpdatedAt = m.CreatedAt.Add(-time.Second) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			m, err := materializationInput().Materialization(time.Now())
			if err != nil {
				t.Fatal(err)
			}
			mutate(&m)
			if m.Validate() == nil {
				t.Fatal("invalid metadata accepted")
			}
		})
	}
}

func materializationInput() NewMaterialization {
	return NewMaterialization{
		TenantID: uuid.Must(uuid.NewV7()), ID: uuid.Must(uuid.NewV7()), WorkspaceID: uuid.Must(uuid.NewV7()),
		BaseGeneration: 1, Provider: "kubernetes", Mode: MaterializationReadOnly,
		Target: MaterializationTarget{ID: "test-target", Region: "eu", StorageClass: "ssd", Architecture: "arm64"},
	}
}
