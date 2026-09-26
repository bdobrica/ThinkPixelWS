package kubernetes

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/bdobrica/ThinkPixelWS/internal/app/materialization"
	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMaterializationStatusObservesWithoutMutating(t *testing.T) {
	for _, scenario := range []string{"unbound", "pending", "bound-preparing", "ready", "active", "checkpointing", "lost", "deleting", "released", "failed", "fenced", "missing", "replacement", "unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			p, api, m := providerFixture(t)
			if scenario != "unbound" {
				storage, err := p.Allocate(context.Background(), m)
				if err != nil {
					t.Fatal(err)
				}
				m.Handle = storage.Handle
			}
			wantPhase := ports.WorkingStoragePending
			var wantErr error
			switch scenario {
			case "unbound":
				m.State = domain.MaterializationRequested
			case "bound-preparing", "ready", "active", "checkpointing", "failed", "fenced":
				api.pvc.Status.Phase = corev1.ClaimBound
				wantPhase = ports.WorkingStorageBound
				switch scenario {
				case "ready":
					m.State = domain.MaterializationReady
				case "active":
					m.State = domain.MaterializationActive
				case "checkpointing":
					m.State = domain.MaterializationCheckpointing
				case "failed":
					m.State = domain.MaterializationFailed
				case "fenced":
					m.State = domain.MaterializationFenced
				}
			case "lost":
				api.pvc.Status.Phase = corev1.ClaimLost
				wantPhase = ports.WorkingStorageLost
			case "deleting":
				m.State = domain.MaterializationReleasing
				now := metav1.Now()
				api.pvc.DeletionTimestamp = &now
				wantPhase = ports.WorkingStorageReleasing
			case "released":
				m.State = domain.MaterializationReleased
				api.pvc = nil
				wantErr = ports.ErrWorkingStorageNotFound
			case "missing":
				api.pvc = nil
				wantErr = ports.ErrWorkingStorageNotFound
			case "replacement":
				api.pvc.UID = "replacement"
				wantErr = ports.ErrWorkingStorageConflict
			case "unavailable":
				api.discovery = false
				wantErr = ports.ErrWorkingStorageUnavailable
			}
			store := &preparationStore{m: m}
			reader := materialization.StatusReader{Repository: store, Storage: p}
			beforePVC := api.pvc.DeepCopy()
			beforeCalls := api.discoveryCalls
			for i := 0; i < 2; i++ {
				result, err := reader.Status(context.Background(), m.TenantID, m.ID)
				if !errors.Is(err, wantErr) || result.Materialization != m {
					t.Fatalf("status: %+v, %v", result, err)
				}
				if wantErr != nil || scenario == "unbound" {
					if result.Storage != nil {
						t.Fatal("reported unobserved storage")
					}
				} else if result.Storage == nil || result.Storage.Phase != wantPhase || result.Storage.Handle != m.Handle {
					t.Fatalf("storage: %+v", result.Storage)
				}
			}
			if store.m != m || api.deletes != 0 {
				t.Fatal("poll changed lifecycle or deleted storage")
			}
			if scenario == "unbound" && (api.pvc != nil || api.discoveryCalls != beforeCalls) {
				t.Fatal("unbound poll called provider")
			}
			if !reflect.DeepEqual(api.pvc, beforePVC) {
				t.Fatal("poll mutated PVC")
			}
		})
	}
}

// Any attempted allocation/release panics through the nil embedded interface.
// This also allows lifecycle changes precisely during the provider observation.
type statusStorage struct {
	ports.WorkingStorageProvider
	observe func(context.Context, domain.Materialization) (ports.WorkingStorage, error)
}

func (s statusStorage) Status(ctx context.Context, m domain.Materialization) (ports.WorkingStorage, error) {
	return s.observe(ctx, m)
}

func TestMaterializationStatusRejectsInvalidOrStaleObservations(t *testing.T) {
	_, _, m := providerFixture(t)
	m.Handle = "opaque-handle"
	for _, scenario := range []string{"foreign-tenant", "wrong-id", "canceled", "fenced-during-read", "wrong-handle", "unknown-phase"} {
		t.Run(scenario, func(t *testing.T) {
			store := &preparationStore{m: m}
			calls := 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reader := materialization.StatusReader{Repository: store, Storage: statusStorage{observe: func(context.Context, domain.Materialization) (ports.WorkingStorage, error) {
				calls++
				s := ports.WorkingStorage{Handle: m.Handle, Phase: ports.WorkingStorageBound}
				switch scenario {
				case "fenced-during-read":
					store.m.State = domain.MaterializationFenced
					store.m.StateVersion++
				case "wrong-handle":
					s.Handle = "other"
				case "unknown-phase":
					s.Phase = "unknown"
				}
				return s, nil
			}}}
			tenant, id := m.TenantID, m.ID
			want := ports.ErrMaterializationNotFound
			switch scenario {
			case "foreign-tenant":
				tenant = uuid.Must(uuid.NewV7())
			case "wrong-id":
				id = uuid.Must(uuid.NewV7())
			case "canceled":
				cancel()
				want = context.Canceled
			case "fenced-during-read":
				want = ports.ErrMaterializationStateConflict
			case "wrong-handle":
				want = ports.ErrWorkingStorageConflict
			case "unknown-phase":
				want = ports.ErrWorkingStorageUnavailable
			}
			result, err := reader.Status(ctx, tenant, id)
			if !errors.Is(err, want) || result.Storage != nil {
				t.Fatalf("status: %+v, %v", result, err)
			}
			if scenario == "foreign-tenant" || scenario == "wrong-id" || scenario == "canceled" {
				if calls != 0 || result.Materialization != (domain.Materialization{}) {
					t.Fatal("invalid request observed storage or exposed metadata")
				}
			}
			if scenario == "fenced-during-read" && result.Materialization != (domain.Materialization{}) {
				t.Fatal("returned stale lifecycle")
			}
		})
	}
}
