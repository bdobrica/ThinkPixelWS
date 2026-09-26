package kubernetes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/app/materialization"
	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
)

func releaseFixture(t *testing.T) (materialization.Releaser, *preparationStore, *storageFixture) {
	t.Helper()
	p, api, m := providerFixture(t)
	storage, err := p.Allocate(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	m.Handle = storage.Handle
	m.State = domain.MaterializationActive
	store := &preparationStore{m: m}
	return materialization.Releaser{Repository: store, Storage: p, Clock: preparationClock{m.CreatedAt.Add(time.Second)}}, store, api
}

func TestReleaseMaterialization(t *testing.T) {
	for _, mode := range []domain.MaterializationMode{domain.MaterializationReadOnly, domain.MaterializationReadWrite} {
		t.Run(string(mode), func(t *testing.T) {
			r, store, api := releaseFixture(t)
			store.m.Mode = mode
			api.pvc.Labels["thinkpixel.io/mode"] = string(mode)
			if mode == domain.MaterializationReadOnly {
				store.m.State = domain.MaterializationReady
			}
			before := store.m
			api.delayDelete = true
			for i := 0; i < 2; i++ {
				m, err := r.Release(context.Background(), before.TenantID, before.ID)
				if err != nil || m.State != domain.MaterializationReleasing || m.StateVersion != before.StateVersion+1 || api.deletes != 1 {
					t.Fatalf("pending: %+v, %v, deletes=%d", m, err, api.deletes)
				}
			}
			// Simulate Kubernetes completing deletion after finalizers clear.
			api.mu.Lock()
			api.pvc = nil
			api.mu.Unlock()
			m, err := r.Release(context.Background(), before.TenantID, before.ID)
			if err != nil || m.State != domain.MaterializationReleased || m.StateVersion != before.StateVersion+2 {
				t.Fatalf("complete: %+v, %v", m, err)
			}
			// Release changes only lifecycle/version/time, retaining all durable
			// identity, generation and storage references. There is no canonical
			// content or Workspace repository dependency in the release path.
			expected := before
			expected.State, expected.StateVersion, expected.UpdatedAt = m.State, m.StateVersion, m.UpdatedAt
			if m != expected || store.m != m {
				t.Fatal("release changed non-lifecycle metadata")
			}
			calls := api.discoveryCalls
			replay, err := r.Release(context.Background(), before.TenantID, before.ID)
			if err != nil || replay != m || api.discoveryCalls != calls || api.deletes != 1 {
				t.Fatalf("replay: %+v, %v", replay, err)
			}
		})
	}
}

func TestReleaseRejectsUnsafeRequests(t *testing.T) {
	for _, failure := range []string{"tenant", "id", "canceled", "requested", "preparing", "checkpointing", "failed", "fenced", "unbound", "replacement", "foreign-pvc", "unavailable", "delete-conflict"} {
		t.Run(failure, func(t *testing.T) {
			r, store, api := releaseFixture(t)
			tenant, id := store.m.TenantID, store.m.ID
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := ports.ErrMaterializationStateConflict
			switch failure {
			case "tenant":
				tenant, want = uuid.Must(uuid.NewV7()), ports.ErrMaterializationNotFound
			case "id":
				id, want = uuid.Must(uuid.NewV7()), ports.ErrMaterializationNotFound
			case "canceled":
				cancel()
				want = context.Canceled
			case "requested":
				store.m.State, store.m.Handle = domain.MaterializationRequested, ""
			case "preparing":
				store.m.State = domain.MaterializationPreparing
			case "checkpointing":
				store.m.State = domain.MaterializationCheckpointing
			case "failed":
				store.m.State = domain.MaterializationFailed
			case "fenced":
				store.m.State = domain.MaterializationFenced
			case "unbound":
				store.m.Handle, want = "", ports.ErrWorkingStorageConflict
			case "replacement":
				api.pvc.UID, want = "replacement", ports.ErrWorkingStorageConflict
			case "foreign-pvc":
				api.pvc.Labels["thinkpixel.io/tenant"], want = uuid.Must(uuid.NewV7()).String(), ports.ErrWorkingStorageConflict
			case "unavailable":
				api.discovery, want = false, ports.ErrWorkingStorageUnavailable
			case "delete-conflict":
				api.failDelete, want = true, ports.ErrWorkingStorageConflict
			}
			before := store.m
			_, err := r.Release(ctx, tenant, id)
			if !errors.Is(err, want) || api.deletes != 0 || api.pvc == nil || store.m.State == domain.MaterializationReleased {
				t.Fatalf("unsafe result: %+v, %v", store.m, err)
			}
			if failure == "unavailable" || failure == "delete-conflict" {
				api.discovery, api.failDelete = true, false
				m, err := r.Release(context.Background(), tenant, id)
				if err != nil || m.State != domain.MaterializationReleased || m.StateVersion != before.StateVersion+2 {
					t.Fatalf("retry: %+v, %v", m, err)
				}
			}
		})
	}
}

type releaseRepository struct {
	*preparationStore
	failState domain.MaterializationState
}

func (s *releaseRepository) TransitionState(ctx context.Context, tenant, id uuid.UUID, current, next domain.MaterializationState, version uint64, now time.Time) error {
	if next == s.failState {
		return ports.ErrMaterializationStateConflict
	}
	return s.preparationStore.TransitionState(ctx, tenant, id, current, next, version, now)
}

func TestReleasePersistenceRetry(t *testing.T) {
	for _, state := range []domain.MaterializationState{domain.MaterializationReleasing, domain.MaterializationReleased} {
		t.Run(string(state), func(t *testing.T) {
			r, store, api := releaseFixture(t)
			repo := &releaseRepository{preparationStore: store, failState: state}
			r.Repository = repo
			before := store.m
			_, err := r.Release(context.Background(), before.TenantID, before.ID)
			if !errors.Is(err, ports.ErrMaterializationStateConflict) {
				t.Fatal(err)
			}
			if state == domain.MaterializationReleasing && (api.deletes != 0 || store.m != before) {
				t.Fatal("deleted before persisting release intent")
			}
			if state == domain.MaterializationReleased && (api.deletes != 1 || store.m.State != domain.MaterializationReleasing) {
				t.Fatal("lost partial progress")
			}
			repo.failState = ""
			m, err := r.Release(context.Background(), before.TenantID, before.ID)
			if err != nil || m.State != domain.MaterializationReleased || api.deletes != 1 {
				t.Fatalf("retry: %+v, %v", m, err)
			}
		})
	}
}

type releaseStorage struct {
	ports.WorkingStorageProvider
	afterRelease func()
}

func (s releaseStorage) Release(ctx context.Context, m domain.Materialization) error {
	err := s.WorkingStorageProvider.Release(ctx, m)
	s.afterRelease()
	return err
}

func TestReleaseDoesNotReviveConcurrentFence(t *testing.T) {
	r, store, api := releaseFixture(t)
	r.Storage = releaseStorage{r.Storage, func() {
		store.mu.Lock()
		defer store.mu.Unlock()
		store.m.State = domain.MaterializationFenced
		store.m.StateVersion++
	}}
	_, err := r.Release(context.Background(), store.m.TenantID, store.m.ID)
	if !errors.Is(err, ports.ErrMaterializationStateConflict) || store.m.State != domain.MaterializationFenced || api.deletes != 1 {
		t.Fatalf("concurrent fence: %+v, %v", store.m, err)
	}
}

func TestReleaseRequiresConfirmedAbsence(t *testing.T) {
	for _, failure := range []string{"unavailable", "canceled", "replacement"} {
		t.Run(failure, func(t *testing.T) {
			r, store, api := releaseFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			original := api.pvc.DeepCopy()
			want := ports.ErrWorkingStorageUnavailable
			r.Storage = releaseStorage{r.Storage, func() {
				api.mu.Lock()
				defer api.mu.Unlock()
				switch failure {
				case "unavailable":
					api.discovery = false
				case "canceled":
					cancel()
					want = context.Canceled
				case "replacement":
					original.UID = "replacement"
					api.pvc = original
					want = ports.ErrWorkingStorageConflict
				}
			}}
			_, err := r.Release(ctx, store.m.TenantID, store.m.ID)
			if !errors.Is(err, want) || store.m.State != domain.MaterializationReleasing || api.deletes != 1 {
				t.Fatalf("unconfirmed release: %+v, %v", store.m, err)
			}
		})
	}
}
