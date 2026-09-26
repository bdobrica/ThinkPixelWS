package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/adapters/workingstorage/mounted"
	"github.com/bdobrica/ThinkPixelWS/internal/app/materialization"
	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
	"k8s.io/client-go/rest"
)

// Recreate service, repository object and HTTP clients from persisted metadata
// and operator config. Only the external API fixture and mounted files survive.
// This exercises restart boundaries, not a real PostgreSQL/process restart.
func restartedRecovery(t *testing.T, m domain.Materialization, api *storageFixture, content mounted.Preparer) (materialization.Recoverer, *preparationStore) {
	t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	store := &preparationStore{}
	if err := json.Unmarshal(data, &store.m); err != nil {
		t.Fatal(err)
	}
	client, err := newClient(Config{Namespace: "ws-test"}, func() (*rest.Config, error) {
		return &rest.Config{Host: api.url, ContentConfig: rest.ContentConfig{ContentType: "application/json"}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewProvider(context.Background(), client, testProviderConfig("test-target", "ssd", "1Gi"))
	if err != nil {
		t.Fatal(err)
	}
	return materialization.Recoverer{Repository: store, Storage: provider, Content: content, Clock: preparationClock{m.UpdatedAt.Add(time.Second)}}, store
}

func TestRecoverPreparationAfterRestart(t *testing.T) {
	for _, boundary := range []string{"requested", "allocated-before-bind", "bound-before-mount", "partial-restore", "restored-before-ready"} {
		t.Run(boundary, func(t *testing.T) {
			f := newPreparationFixture(t)
			original := f.store.m
			switch boundary {
			case "allocated-before-bind":
				f.store.failBind = true
				if _, err := f.run(context.Background()); err == nil {
					t.Fatal("expected bind failure")
				}
			case "bound-before-mount":
				f.service.Content = mounted.Preparer{WithRoot: func(context.Context, domain.Materialization, func(*os.Root) error) (bool, error) { return false, nil }, Restore: f.content.Restore}
				if _, err := f.service.Prepare(context.Background(), original.TenantID, original.ID); err != nil {
					t.Fatal(err)
				}
			case "partial-restore", "restored-before-ready":
				content := f.content
				restore := content.Restore
				content.Restore = func(ctx context.Context, m domain.Materialization, root *os.Root) ([]domain.WorkspaceComponent, error) {
					components, err := restore(ctx, m, root)
					if err != nil {
						return nil, err
					}
					if boundary == "partial-restore" {
						if err := root.WriteFile("repo/README.md", []byte("partial"), 0644); err != nil {
							return nil, err
						}
					}
					return components, errors.New("interrupted before readiness")
				}
				f.service.Content = content
				if _, err := f.service.Prepare(context.Background(), original.TenantID, original.ID); err == nil {
					t.Fatal("expected interruption")
				}
			}
			var pvcName, pvcUID string
			if f.api.pvc != nil {
				pvcName, pvcUID = f.api.pvc.Name, string(f.api.pvc.UID)
			}
			recovery, store := restartedRecovery(t, f.store.m, f.api, f.content)
			got, err := recovery.Recover(context.Background(), original.TenantID, original.ID)
			if err != nil || got.State != domain.MaterializationReady || got != store.m {
				t.Fatalf("recover: %+v, %v", got, err)
			}
			if got.ID != original.ID || got.WorkspaceID != original.WorkspaceID || got.BaseGeneration != original.BaseGeneration {
				t.Fatal("changed durable identity")
			}
			if pvcUID != "" && (string(f.api.pvc.UID) != pvcUID || f.api.pvc.Name != pvcName) {
				t.Fatal("replaced storage")
			}
			data, err := os.ReadFile(filepath.Join(f.dir, "repo/README.md"))
			if err != nil || string(data) != "generation one\n" || f.api.deletes != 0 {
				t.Fatalf("content=%q err=%v", data, err)
			}
		})
	}
}

func TestRecoverPreservesWorkingAndTerminalStates(t *testing.T) {
	for _, state := range []domain.MaterializationState{domain.MaterializationReady, domain.MaterializationActive, domain.MaterializationCheckpointing, domain.MaterializationFenced, domain.MaterializationFailed, domain.MaterializationReleased} {
		t.Run(string(state), func(t *testing.T) {
			f := newPreparationFixture(t)
			m, err := f.run(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			m.State = state
			path := filepath.Join(f.dir, "repo/README.md")
			if err := os.WriteFile(path, []byte("uncommitted working edit"), 0644); err != nil {
				t.Fatal(err)
			}
			recovery, store := restartedRecovery(t, m, f.api, mounted.Preparer{})
			before := store.m
			pvc := f.api.pvc.DeepCopy()
			calls := f.api.discoveryCalls
			got, err := recovery.Recover(context.Background(), m.TenantID, m.ID)
			if state == domain.MaterializationCheckpointing {
				if !errors.Is(err, ports.ErrMaterializationStateConflict) {
					t.Fatalf("checkpoint: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if got != before || store.m != before || calls != f.api.discoveryCalls || f.api.deletes != 0 {
				t.Fatal("recovery changed stable state")
			}
			beforeJSON, _ := json.Marshal(pvc)
			afterJSON, _ := json.Marshal(f.api.pvc)
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "uncommitted working edit" || string(beforeJSON) != string(afterJSON) {
				t.Fatal("recovery changed working storage")
			}
		})
	}
}

func TestRecoverReleaseAfterRestart(t *testing.T) {
	for _, boundary := range []string{"before-delete", "deleting", "deleted-before-persist"} {
		t.Run(boundary, func(t *testing.T) {
			release, store, api := releaseFixture(t)
			api.failDelete = boundary == "before-delete"
			api.delayDelete = boundary == "deleting"
			if boundary == "deleted-before-persist" {
				release.Repository = &releaseRepository{preparationStore: store, failState: domain.MaterializationReleased}
			}
			_, err := release.Release(context.Background(), store.m.TenantID, store.m.ID)
			if boundary != "deleting" && err == nil {
				t.Fatal("expected interruption")
			}
			if store.m.State != domain.MaterializationReleasing {
				t.Fatal("release intent not persisted")
			}
			api.failDelete = false
			recovery, restarted := restartedRecovery(t, store.m, api, mounted.Preparer{})
			got, err := recovery.Recover(context.Background(), store.m.TenantID, store.m.ID)
			if err != nil {
				t.Fatal(err)
			}
			if boundary == "deleting" {
				if got.State != domain.MaterializationReleasing {
					t.Fatal("completed pending deletion")
				}
				api.mu.Lock()
				api.pvc = nil
				api.mu.Unlock()
				recovery, restarted = restartedRecovery(t, restarted.m, api, mounted.Preparer{})
				got, err = recovery.Recover(context.Background(), store.m.TenantID, store.m.ID)
			}
			if err != nil || got.State != domain.MaterializationReleased || got.Handle != store.m.Handle || api.deletes != 1 {
				t.Fatalf("recover: %+v %v deletes=%d", got, err, api.deletes)
			}
		})
	}
}

func TestRecoverFailsClosed(t *testing.T) {
	f := newPreparationFixture(t)
	f.store.failBind = true
	if _, err := f.run(context.Background()); err == nil {
		t.Fatal("expected interruption")
	}
	recovery, store := restartedRecovery(t, f.store.m, f.api, f.content)
	if _, err := recovery.Recover(context.Background(), uuid.Must(uuid.NewV7()), store.m.ID); !errors.Is(err, ports.ErrMaterializationNotFound) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := recovery.Recover(ctx, store.m.TenantID, store.m.ID); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	f.api.discovery = false
	if _, err := recovery.Recover(context.Background(), store.m.TenantID, store.m.ID); !errors.Is(err, ports.ErrWorkingStorageUnavailable) {
		t.Fatal(err)
	}
	if store.m.Handle != "" || f.restores != 0 || f.api.deletes != 0 {
		t.Fatal("failed recovery changed storage")
	}
	f.api.discovery = true
	if _, err := recovery.Recover(context.Background(), store.m.TenantID, store.m.ID); err != nil {
		t.Fatal(err)
	}
}
