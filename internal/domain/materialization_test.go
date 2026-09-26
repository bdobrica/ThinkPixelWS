package domain

import (
	"errors"
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

func TestMaterializationLifecycle(t *testing.T) {
	edges := map[MaterializationState][]MaterializationState{
		MaterializationRequested:     {MaterializationPreparing, MaterializationFailed},
		MaterializationPreparing:     {MaterializationReady, MaterializationFailed},
		MaterializationReady:         {MaterializationActive, MaterializationReleasing},
		MaterializationActive:        {MaterializationCheckpointing, MaterializationReleasing, MaterializationFenced},
		MaterializationCheckpointing: {MaterializationActive, MaterializationFailed, MaterializationFenced},
		MaterializationReleasing:     {MaterializationReleased, MaterializationFailed},
	}
	states := []MaterializationState{MaterializationRequested, MaterializationPreparing, MaterializationReady, MaterializationActive, MaterializationCheckpointing, MaterializationReleasing, MaterializationReleased, MaterializationFailed, MaterializationFenced, "UNKNOWN"}
	original, err := materializationInput().Materialization(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	now := original.UpdatedAt.Add(time.Second).In(time.FixedZone("local", 7200))
	for _, from := range states {
		for _, to := range states {
			t.Run(string(from)+"/"+string(to), func(t *testing.T) {
				m := original
				m.State = from
				allowed := false
				for _, target := range edges[from] {
					allowed = allowed || target == to
				}
				got, err := m.TransitionState(to, m.StateVersion, now)
				if !allowed {
					if err == nil {
						t.Fatal("invalid edge accepted")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				want := m
				want.State, want.StateVersion, want.UpdatedAt = to, m.StateVersion+1, now.UTC()
				if got != want || m.State != from {
					t.Fatalf("unexpected mutation: %#v", got)
				}
			})
		}
	}
}

func TestMaterializationTransitionGuards(t *testing.T) {
	m, err := materializationInput().Materialization(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.TransitionState(MaterializationPreparing, 2, m.UpdatedAt); !errors.Is(err, ErrMaterializationStateVersionConflict) {
		t.Fatalf("expected version conflict: %v", err)
	}
	if _, err := m.TransitionState(MaterializationActive, 1, m.UpdatedAt); !errors.Is(err, ErrInvalidMaterializationStateTransition) {
		t.Fatalf("expected invalid transition: %v", err)
	}
	for _, now := range []time.Time{{}, m.UpdatedAt.Add(-time.Second)} {
		if _, err := m.TransitionState(MaterializationPreparing, 1, now); err == nil {
			t.Fatal("invalid time accepted")
		}
	}
	m.StateVersion = math.MaxInt64
	if _, err := m.TransitionState(MaterializationPreparing, m.StateVersion, m.UpdatedAt); err == nil {
		t.Fatal("version overflow accepted")
	}
}
