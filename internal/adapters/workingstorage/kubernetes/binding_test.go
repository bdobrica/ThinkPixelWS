package kubernetes

import (
	"context"
	"encoding/json"
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

func TestMaterializationBinding(t *testing.T) {
	for _, scenario := range []string{"read-only", "read-write", "active", "preparing", "fenced", "checkpointing", "released", "unbound", "pending", "lost", "deleting", "missing", "replacement", "foreign-tenant", "wrong-id", "canceled", "unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			p, api, m := providerFixture(t)
			if scenario == "read-only" {
				m.Mode = domain.MaterializationReadOnly
			}
			storage, err := p.Allocate(context.Background(), m)
			if err != nil {
				t.Fatal(err)
			}
			m.Handle = storage.Handle
			m.State = domain.MaterializationReady
			api.pvc.Status.Phase = corev1.ClaimBound
			tenant, id := m.TenantID, m.ID
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var want error
			switch scenario {
			case "active":
				m.State = domain.MaterializationActive
			case "preparing":
				m.State = domain.MaterializationPreparing
				want = ports.ErrMaterializationStateConflict
			case "fenced":
				m.State = domain.MaterializationFenced
				want = ports.ErrMaterializationStateConflict
			case "checkpointing":
				m.State = domain.MaterializationCheckpointing
				want = ports.ErrMaterializationStateConflict
			case "released":
				m.State = domain.MaterializationReleased
				want = ports.ErrMaterializationStateConflict
			case "unbound":
				m.Handle = ""
				want = ports.ErrMaterializationStateConflict
			case "pending":
				api.pvc.Status.Phase = corev1.ClaimPending
				want = ports.ErrWorkingStorageConflict
			case "lost":
				api.pvc.Status.Phase = corev1.ClaimLost
				want = ports.ErrWorkingStorageConflict
			case "deleting":
				now := metav1.Now()
				api.pvc.DeletionTimestamp = &now
				want = ports.ErrWorkingStorageConflict
			case "missing":
				api.pvc = nil
				want = ports.ErrWorkingStorageNotFound
			case "replacement":
				api.pvc.UID = "replacement"
				want = ports.ErrWorkingStorageConflict
			case "foreign-tenant":
				tenant = uuid.Must(uuid.NewV7())
				want = ports.ErrMaterializationNotFound
			case "wrong-id":
				id = uuid.Must(uuid.NewV7())
				want = ports.ErrMaterializationNotFound
			case "canceled":
				cancel()
				want = context.Canceled
			case "unavailable":
				api.discovery = false
				want = ports.ErrWorkingStorageUnavailable
			}
			store := &preparationStore{m: m}
			reader := materialization.BindingReader{Repository: store, Storage: p}
			before := api.pvc.DeepCopy()
			for i := 0; i < 2; i++ {
				got, err := reader.Binding(ctx, tenant, id)
				if !errors.Is(err, want) {
					t.Fatalf("binding error %v, want %v", err, want)
				}
				if want != nil {
					if !reflect.DeepEqual(got, materialization.Binding{}) {
						t.Fatal("exposed binding on error")
					}
					continue
				}
				wire, err := json.Marshal(got)
				if err != nil {
					t.Fatal(err)
				}
				var decoded materialization.Binding
				if err := json.Unmarshal(wire, &decoded); err != nil {
					t.Fatal(err)
				}
				var ref PVCBinding
				if err := json.Unmarshal(decoded.Storage.Reference, &ref); err != nil {
					t.Fatal(err)
				}
				if decoded.TenantID != m.TenantID || decoded.WorkspaceID != m.WorkspaceID || decoded.MaterializationID != m.ID || decoded.Generation != m.BaseGeneration || decoded.StateVersion != m.StateVersion || decoded.TargetID != m.Target.ID || decoded.AccessMode != m.Mode || decoded.Storage.Handle != m.Handle || decoded.Storage.Kind != PVCBindingKind {
					t.Fatalf("identity: %+v", decoded)
				}
				expected := PVCBinding{Namespace: p.client.Namespace, ClaimName: api.pvc.Name, ClaimUID: string(api.pvc.UID), MountPath: "/workspace", ReadOnly: m.Mode == domain.MaterializationReadOnly}
				if ref != expected {
					t.Fatalf("reference: %+v", ref)
				}
			}
			if store.m != m || !reflect.DeepEqual(api.pvc, before) || api.deletes != 0 {
				t.Fatal("binding mutated storage or lifecycle")
			}
		})
	}
}

type bindingStorage struct {
	resolve func(context.Context, domain.Materialization) (ports.WorkingStorageBinding, error)
}

func (s bindingStorage) Binding(ctx context.Context, m domain.Materialization) (ports.WorkingStorageBinding, error) {
	return s.resolve(ctx, m)
}

func TestMaterializationBindingRejectsConcurrentChange(t *testing.T) {
	p, api, m := providerFixture(t)
	s, err := p.Allocate(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	m.Handle = s.Handle
	m.State = domain.MaterializationReady
	api.pvc.Status.Phase = corev1.ClaimBound
	store := &preparationStore{m: m}
	reader := materialization.BindingReader{Repository: store, Storage: bindingStorage{resolve: func(ctx context.Context, m domain.Materialization) (ports.WorkingStorageBinding, error) {
		result, err := p.Binding(ctx, m)
		store.m.State = domain.MaterializationFenced
		store.m.StateVersion++
		return result, err
	}}}
	got, err := reader.Binding(context.Background(), m.TenantID, m.ID)
	if !errors.Is(err, ports.ErrMaterializationStateConflict) || !reflect.DeepEqual(got, materialization.Binding{}) {
		t.Fatalf("stale binding: %+v %v", got, err)
	}
}
