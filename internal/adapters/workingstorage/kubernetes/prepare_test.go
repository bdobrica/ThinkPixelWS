package kubernetes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/adapters/workingstorage/mounted"
	"github.com/bdobrica/ThinkPixelWS/internal/app/materialization"
	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
)

// This store implements the existing repository CAS contract; actual PostgreSQL
// CAS behavior is covered in the repository integration tests.
type preparationStore struct {
	mu       sync.Mutex
	m        domain.Materialization
	failBind bool
}

func (s *preparationStore) Create(context.Context, uuid.UUID, domain.Materialization) error {
	panic("unexpected create")
}
func (s *preparationStore) Get(ctx context.Context, tenant, id uuid.UUID) (domain.Materialization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return domain.Materialization{}, err
	}
	if tenant != s.m.TenantID || id != s.m.ID {
		return domain.Materialization{}, ports.ErrMaterializationNotFound
	}
	return s.m, nil
}
func (s *preparationStore) Bind(ctx context.Context, tenant, id uuid.UUID, handle domain.MaterializationHandle, version uint64, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.failBind {
		return errors.New("binding persistence unavailable")
	}
	if tenant != s.m.TenantID || id != s.m.ID || version != s.m.StateVersion || s.m.State != domain.MaterializationPreparing {
		return ports.ErrMaterializationStateConflict
	}
	m, err := s.m.Bind(handle, version, now)
	if err == nil {
		s.m = m
	}
	return err
}
func (s *preparationStore) TransitionState(ctx context.Context, tenant, id uuid.UUID, current, next domain.MaterializationState, version uint64, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if tenant != s.m.TenantID || id != s.m.ID || current != s.m.State || version != s.m.StateVersion {
		return ports.ErrMaterializationStateConflict
	}
	m, err := s.m.TransitionState(next, version, now)
	if err == nil {
		s.m = m
	}
	return err
}

type preparationClock struct{ now time.Time }

func (c preparationClock) Now() time.Time { return c.now }

type preparationFixture struct {
	service  materialization.Preparer
	store    *preparationStore
	api      *storageFixture
	content  mounted.Preparer
	dir      string
	restores int
}

func newPreparationFixture(t *testing.T) *preparationFixture {
	t.Helper()
	p, api, m := providerFixture(t)
	m.State = domain.MaterializationRequested
	m.CreatedAt = m.CreatedAt.Truncate(time.Microsecond)
	m.UpdatedAt = m.CreatedAt
	f := &preparationFixture{store: &preparationStore{m: m}, api: api, dir: t.TempDir()}
	var mount sync.Mutex
	f.content.WithRoot = func(ctx context.Context, m domain.Materialization, use func(*os.Root) error) (bool, error) {
		mount.Lock()
		defer mount.Unlock()
		// Emulate a consumer binding a WaitForFirstConsumer PVC. No Pod or
		// storage driver is simulated; only the provider API boundary is real HTTP.
		api.mu.Lock()
		if api.pvc == nil || storageResult(api.pvc).Handle != m.Handle {
			api.mu.Unlock()
			return false, ports.ErrWorkingStorageConflict
		}
		api.pvc.Status.Phase = corev1.ClaimBound
		api.mu.Unlock()
		root, err := os.OpenRoot(f.dir)
		if err != nil {
			return false, err
		}
		defer root.Close()
		err = use(root)
		return err == nil, err
	}
	component, err := (domain.NewWorkspaceComponent{TenantID: m.TenantID, WorkspaceID: m.WorkspaceID, ID: uuid.Must(uuid.NewV7()), Name: "repo", Kind: domain.WorkspaceComponentRepository}).WorkspaceComponent(m.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	f.content.Restore = func(ctx context.Context, request domain.Materialization, root *os.Root) ([]domain.WorkspaceComponent, error) {
		f.restores++
		if request.BaseGeneration != 1 || request.TenantID != component.TenantID || request.WorkspaceID != component.WorkspaceID {
			return nil, errors.New("wrong generation")
		}
		if err := root.Mkdir("repo", 0755); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if err := root.WriteFile("repo/README.md", []byte("generation one\n"), 0644); err != nil {
			return nil, err
		}
		return []domain.WorkspaceComponent{component}, nil
	}
	f.service = materialization.Preparer{Repository: f.store, Storage: p, Content: f.content, Clock: preparationClock{m.CreatedAt.Add(time.Second)}}
	return f
}

func (f *preparationFixture) run(ctx context.Context) (domain.Materialization, error) {
	f.service.Content = f.content
	return f.service.Prepare(ctx, f.store.m.TenantID, f.store.m.ID)
}

func TestPrepareMaterializationFromPendingPVC(t *testing.T) {
	for _, mode := range []domain.MaterializationMode{domain.MaterializationReadOnly, domain.MaterializationReadWrite} {
		t.Run(string(mode), func(t *testing.T) {
			f := newPreparationFixture(t)
			f.store.m.Mode = mode
			m, err := f.run(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if m.State != domain.MaterializationReady || m.StateVersion != 4 || m.Handle != "k8s-pvc-v1:test-pvc-uid" || m != f.store.m {
				t.Fatalf("unexpected result: %+v", m)
			}
			content, err := os.ReadFile(filepath.Join(f.dir, "repo/README.md"))
			if err != nil || string(content) != "generation one\n" {
				t.Fatalf("restored content: %q, %v", content, err)
			}
			// Ready replay cannot restore over subsequent working changes.
			if err := os.WriteFile(filepath.Join(f.dir, "repo/README.md"), []byte("working edit"), 0644); err != nil {
				t.Fatal(err)
			}
			replay, err := f.run(context.Background())
			if err != nil || replay != m || f.restores != 1 {
				t.Fatalf("replay: %+v, %v, restores=%d", replay, err, f.restores)
			}
			content, _ = os.ReadFile(filepath.Join(f.dir, "repo/README.md"))
			if string(content) != "working edit" || f.api.deletes != 0 {
				t.Fatal("retry changed working content or deleted PVC")
			}
		})
	}
}

func TestPrepareResumesAfterBindingFailureAndPendingMount(t *testing.T) {
	f := newPreparationFixture(t)
	f.store.failBind = true
	if _, err := f.run(context.Background()); err == nil {
		t.Fatal("expected binding failure")
	}
	if f.api.pvc == nil || f.store.m.Handle != "" || f.restores != 0 {
		t.Fatal("unexpected partial progress")
	}
	f.store.failBind = false
	mount := f.content.WithRoot
	f.content.WithRoot = func(context.Context, domain.Materialization, func(*os.Root) error) (bool, error) { return false, nil }
	m, err := f.run(context.Background())
	if err != nil || m.State != domain.MaterializationPreparing || m.Handle == "" || f.restores != 0 {
		t.Fatalf("pending: %+v, %v", m, err)
	}
	f.content.WithRoot = mount
	m, err = f.run(context.Background())
	if err != nil || m.State != domain.MaterializationReady || f.restores != 1 || f.api.deletes != 0 {
		t.Fatalf("resume: %+v, %v", m, err)
	}
}

func TestPrepareFailuresNeverPublishReadiness(t *testing.T) {
	for _, failure := range []string{"restore", "layout", "fenced-during-restore", "fenced-before-mount", "canceled", "lost", "replaced"} {
		t.Run(failure, func(t *testing.T) {
			f := newPreparationFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			restore := f.content.Restore
			fence := func() {
				f.store.mu.Lock()
				defer f.store.mu.Unlock()
				f.store.m.State = domain.MaterializationFenced
				f.store.m.StateVersion++
			}
			if failure == "fenced-before-mount" {
				mount := f.content.WithRoot
				f.content.WithRoot = func(ctx context.Context, m domain.Materialization, use func(*os.Root) error) (bool, error) {
					fence()
					return mount(ctx, m, use)
				}
			}
			f.content.Restore = func(ctx context.Context, m domain.Materialization, root *os.Root) ([]domain.WorkspaceComponent, error) {
				if failure == "restore" {
					return nil, errors.New("restore failed")
				}
				components, err := restore(ctx, m, root)
				if err != nil {
					return nil, err
				}
				switch failure {
				case "layout":
					components[0].WorkspaceID = uuid.Must(uuid.NewV7())
				case "fenced-during-restore":
					fence()
				case "canceled":
					cancel()
				case "lost":
					f.api.mu.Lock()
					f.api.pvc.Status.Phase = corev1.ClaimLost
					f.api.mu.Unlock()
				case "replaced":
					f.api.mu.Lock()
					f.api.pvc.UID = "replacement"
					f.api.mu.Unlock()
				}
				return components, nil
			}
			_, err := f.run(ctx)
			if err == nil || f.store.m.State == domain.MaterializationReady || f.store.m.Handle == "" || f.api.deletes != 0 {
				t.Fatalf("unsafe result: %+v, %v", f.store.m, err)
			}
			if failure == "fenced-before-mount" && f.restores != 0 {
				t.Fatal("stale request restored content")
			}
			if failure == "restore" || failure == "layout" {
				f.content.Restore = restore
				m, err := f.run(context.Background())
				if err != nil || m.State != domain.MaterializationReady {
					t.Fatalf("retry failed: %+v, %v", m, err)
				}
			}
		})
	}
}

func TestPrepareRejectsTenantAndLifecycleBeforeProvisioning(t *testing.T) {
	f := newPreparationFixture(t)
	if _, err := f.service.Prepare(context.Background(), uuid.Must(uuid.NewV7()), f.store.m.ID); !errors.Is(err, ports.ErrMaterializationNotFound) {
		t.Fatal(err)
	}
	for _, state := range []domain.MaterializationState{domain.MaterializationActive, domain.MaterializationFailed, domain.MaterializationFenced, domain.MaterializationReleased} {
		f.store.m.State = state
		if _, err := f.run(context.Background()); !errors.Is(err, ports.ErrMaterializationStateConflict) {
			t.Fatalf("%s: %v", state, err)
		}
	}
	if f.api.pvc != nil || f.restores != 0 {
		t.Fatal("invalid request provisioned storage")
	}
}

func TestConcurrentPreparationDoesNotRestoreAfterReady(t *testing.T) {
	f := newPreparationFixture(t)
	mount := f.content.WithRoot
	// Persist the binding first, so both attempts observe the same PREPARING
	// version and have to serialize at the mounted volume, not just at Bind.
	f.content.WithRoot = func(context.Context, domain.Materialization, func(*os.Root) error) (bool, error) { return false, nil }
	m, err := f.run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{}, 2)
	proceed := make(chan struct{})
	f.content.WithRoot = func(ctx context.Context, m domain.Materialization, use func(*os.Root) error) (bool, error) {
		entered <- struct{}{}
		select {
		case <-proceed:
		case <-ctx.Done():
			return false, ctx.Err()
		}
		return mount(ctx, m, use)
	}
	f.service.Content = f.content
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := f.service.Prepare(ctx, m.TenantID, m.ID)
			results <- err
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("preparation did not reach mount")
		}
	}
	close(proceed)
	var successes, conflicts int
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			successes++
		} else if errors.Is(err, ports.ErrMaterializationStateConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 || f.restores != 1 || f.store.m.State != domain.MaterializationReady {
		t.Fatalf("successes=%d conflicts=%d restores=%d state=%s", successes, conflicts, f.restores, f.store.m.State)
	}
}
