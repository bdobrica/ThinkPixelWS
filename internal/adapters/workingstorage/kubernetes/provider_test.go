package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

type storageFixture struct {
	mu                      sync.Mutex
	pvc                     *corev1.PersistentVolumeClaim
	discovery               bool
	discoveryCalls, deletes int
	failDelete              bool
}

func providerFixture(t *testing.T) (*Provider, *storageFixture, domain.Materialization) {
	t.Helper()
	f := &storageFixture{discovery: true}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		respond := func(code int, reason metav1.StatusReason) {
			w.WriteHeader(code)
			json.NewEncoder(w).Encode(metav1.Status{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"}, Status: "Failure", Reason: reason, Code: int32(code), Message: "private backend detail"})
		}
		if r.URL.Path == "/api/v1" {
			f.discoveryCalls++
			if !f.discovery {
				respond(404, metav1.StatusReasonNotFound)
				return
			}
			json.NewEncoder(w).Encode(metav1.APIResourceList{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "APIResourceList"}, GroupVersion: "v1", APIResources: []metav1.APIResource{{Name: "persistentvolumeclaims", Namespaced: true, Kind: "PersistentVolumeClaim", Verbs: metav1.Verbs{"create", "get", "delete"}}}})
			return
		}
		prefix := "/api/v1/namespaces/ws-test/persistentvolumeclaims"
		if !strings.HasPrefix(r.URL.Path, prefix) {
			t.Errorf("unexpected path %s", r.URL.Path)
			respond(404, metav1.StatusReasonNotFound)
			return
		}
		switch r.Method {
		case "POST":
			if f.pvc != nil {
				respond(409, metav1.StatusReasonAlreadyExists)
				return
			}
			var pvc corev1.PersistentVolumeClaim
			if err := json.NewDecoder(r.Body).Decode(&pvc); err != nil {
				t.Error(err)
				respond(400, metav1.StatusReasonBadRequest)
				return
			}
			pvc.UID = "test-pvc-uid"
			pvc.ResourceVersion = "1"
			pvc.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "PersistentVolumeClaim"}
			f.pvc = &pvc
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(f.pvc)
		case "GET":
			if f.pvc == nil {
				respond(404, metav1.StatusReasonNotFound)
				return
			}
			if r.URL.Path != prefix+"/"+f.pvc.Name {
				t.Error("incorrect PVC name")
			}
			json.NewEncoder(w).Encode(f.pvc)
		case "DELETE":
			if f.pvc == nil {
				respond(404, metav1.StatusReasonNotFound)
				return
			}
			var opts metav1.DeleteOptions
			if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
				t.Error(err)
			}
			if opts.Preconditions == nil || opts.Preconditions.UID == nil || *opts.Preconditions.UID != f.pvc.UID || opts.Preconditions.ResourceVersion == nil || *opts.Preconditions.ResourceVersion != f.pvc.ResourceVersion {
				t.Error("missing delete preconditions")
			}
			if f.failDelete {
				respond(409, metav1.StatusReasonConflict)
				return
			}
			f.deletes++
			f.pvc = nil
			json.NewEncoder(w).Encode(metav1.Status{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"}, Status: "Success", Code: 200})
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))
	t.Cleanup(server.Close)
	client, err := newClient(Config{Namespace: "ws-test"}, func() (*rest.Config, error) {
		return &rest.Config{Host: server.URL, ContentConfig: rest.ContentConfig{ContentType: "application/json"}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewProvider(context.Background(), client, ProviderConfig{TargetID: "test-target", StorageClass: "ssd", Capacity: "1Gi"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := (domain.NewMaterialization{TenantID: uuid.Must(uuid.NewV7()), WorkspaceID: uuid.Must(uuid.NewV7()), ID: uuid.Must(uuid.NewV7()), BaseGeneration: 1, Provider: "kubernetes", Target: domain.MaterializationTarget{ID: "test-target", Region: "test-region", StorageClass: "ssd"}, Mode: domain.MaterializationReadWrite}).Materialization(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	m.State = domain.MaterializationPreparing
	return p, f, m
}

func TestProviderLifecycle(t *testing.T) {
	p, f, m := providerFixture(t)
	ctx := context.Background()
	first, err := p.Allocate(ctx, m)
	if err != nil || first.Phase != ports.WorkingStoragePending || first.Handle == "" {
		t.Fatalf("allocate: %+v %v", first, err)
	}
	again, err := p.Allocate(ctx, m)
	if err != nil || again != first {
		t.Fatalf("retry: %+v %v", again, err)
	}
	f.mu.Lock()
	if len(f.pvc.OwnerReferences) != 0 || f.pvc.Spec.VolumeName != "" || f.pvc.Spec.Resources.Requests.Storage().String() != "1Gi" {
		t.Error("unexpected storage spec")
	}
	f.pvc.Status.Phase = corev1.ClaimBound
	f.mu.Unlock()
	m.Handle = first.Handle
	got, err := p.Status(ctx, m)
	if err != nil || got.Phase != ports.WorkingStorageBound {
		t.Fatalf("status: %+v %v", got, err)
	}
	// A fresh provider instance can recover the persisted handle after restart.
	restarted, err := NewProvider(ctx, &p.client, p.config)
	if err != nil {
		t.Fatal(err)
	}
	if got, err = restarted.Status(ctx, m); err != nil || got.Handle != first.Handle {
		t.Fatalf("restart: %+v %v", got, err)
	}
	f.mu.Lock()
	f.failDelete = true
	f.mu.Unlock()
	if err = p.Release(ctx, m); !errors.Is(err, ports.ErrWorkingStorageConflict) {
		t.Fatalf("delete conflict: %v", err)
	}
	f.mu.Lock()
	f.failDelete = false
	f.mu.Unlock()
	if err = p.Release(ctx, m); err != nil {
		t.Fatal(err)
	}
	if err = p.Release(ctx, m); err != nil {
		t.Fatalf("release retry: %v", err)
	}
	if _, err = p.Status(ctx, m); !errors.Is(err, ports.ErrWorkingStorageNotFound) {
		t.Fatalf("missing: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deletes != 1 || f.discoveryCalls < 9 {
		t.Fatalf("deletes=%d discovery=%d", f.deletes, f.discoveryCalls)
	}
}

func TestProviderRejectsCollisions(t *testing.T) {
	for _, change := range []string{"tenant", "workspace", "generation", "mode", "target", "class", "capacity", "uid", "owner", "deleting"} {
		t.Run(change, func(t *testing.T) {
			p, f, m := providerFixture(t)
			ctx := context.Background()
			result, err := p.Allocate(ctx, m)
			if err != nil {
				t.Fatal(err)
			}
			f.mu.Lock()
			switch change {
			case "tenant":
				f.pvc.Labels["thinkpixel.io/tenant"] = uuid.Must(uuid.NewV7()).String()
			case "workspace":
				f.pvc.Labels["thinkpixel.io/workspace"] = uuid.Must(uuid.NewV7()).String()
			case "generation":
				f.pvc.Labels["thinkpixel.io/generation"] = "2"
			case "mode":
				f.pvc.Labels["thinkpixel.io/mode"] = "read-only"
			case "target":
				f.pvc.Annotations["thinkpixel.io/target"] = "another-target"
			case "class":
				*f.pvc.Spec.StorageClassName = "other"
			case "capacity":
				f.pvc.Spec.Resources.Requests = nil
			case "uid":
				f.pvc.UID = "replacement-uid"
			case "owner":
				f.pvc.OwnerReferences = []metav1.OwnerReference{{Name: "sandbox"}}
			case "deleting":
				now := metav1.Now()
				f.pvc.DeletionTimestamp = &now
			}
			f.mu.Unlock()
			if change != "uid" {
				if _, err = p.Allocate(ctx, m); !errors.Is(err, ports.ErrWorkingStorageConflict) {
					t.Fatalf("collision: %v", err)
				}
			}
			m.Handle = result.Handle
			if change == "uid" || change == "tenant" || change == "workspace" || change == "generation" || change == "mode" || change == "target" || change == "owner" {
				if _, err = p.Status(ctx, m); !errors.Is(err, ports.ErrWorkingStorageConflict) {
					t.Fatalf("status: %v", err)
				}
				if err = p.Release(ctx, m); !errors.Is(err, ports.ErrWorkingStorageConflict) {
					t.Fatalf("release: %v", err)
				}
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.deletes != 0 {
				t.Fatal("deleted collision")
			}
		})
	}
}

func TestProviderUnavailableAndValidation(t *testing.T) {
	p, f, m := providerFixture(t)
	ctx := context.Background()
	for _, cfg := range []ProviderConfig{{TargetID: "t", StorageClass: "", Capacity: "1Gi"}, {TargetID: "t", StorageClass: "ssd", Capacity: "0"}, {TargetID: "t", StorageClass: "ssd", Capacity: "invalid"}, {TargetID: "", StorageClass: "ssd", Capacity: "1Gi"}} {
		if _, err := NewProvider(ctx, &p.client, cfg); err == nil {
			t.Fatal("accepted invalid config")
		}
	}
	foreign := m
	foreign.Target.ID = "another-target"
	if _, err := p.Allocate(ctx, foreign); !errors.Is(err, ports.ErrWorkingStorageConflict) {
		t.Fatal(err)
	}
	requested := m
	requested.State = domain.MaterializationRequested
	if _, err := p.Allocate(ctx, requested); !errors.Is(err, ports.ErrWorkingStorageConflict) {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := p.Allocate(canceled, m); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	result, err := p.Allocate(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	m.Handle = result.Handle
	f.mu.Lock()
	f.discovery = false
	f.mu.Unlock()
	if _, err = NewProvider(ctx, &p.client, p.config); !errors.Is(err, ports.ErrWorkingStorageUnavailable) {
		t.Fatal(err)
	}
	// A discovery 404 must not be mistaken for an already deleted PVC.
	if err = p.Release(ctx, m); !errors.Is(err, ports.ErrWorkingStorageUnavailable) || strings.Contains(err.Error(), "private") {
		t.Fatalf("discovery: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deletes != 0 {
		t.Fatal("deleted while unavailable")
	}
}

func TestProviderConcurrentAllocate(t *testing.T) {
	p, _, m := providerFixture(t)
	const count = 8
	var wg sync.WaitGroup
	results := make(chan ports.WorkingStorage, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := p.Allocate(context.Background(), m)
			if err != nil {
				t.Error(err)
				return
			}
			results <- got
		}()
	}
	wg.Wait()
	close(results)
	var handle domain.MaterializationHandle
	n := 0
	for got := range results {
		n++
		if handle != "" && got.Handle != handle {
			t.Error("different handles for same identity")
		}
		handle = got.Handle
	}
	if n != count {
		t.Fatalf("only %d allocations succeeded", n)
	}
}

func TestProviderStoragePhases(t *testing.T) {
	p, f, m := providerFixture(t)
	got, err := p.Allocate(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	m.Handle = got.Handle
	f.mu.Lock()
	f.pvc.Status.Phase = corev1.ClaimLost
	f.mu.Unlock()
	if got, err = p.Status(context.Background(), m); err != nil || got.Phase != ports.WorkingStorageLost {
		t.Fatalf("lost: %+v %v", got, err)
	}
	f.mu.Lock()
	now := metav1.Now()
	f.pvc.DeletionTimestamp = &now
	f.mu.Unlock()
	if got, err = p.Status(context.Background(), m); err != nil || got.Phase != ports.WorkingStorageReleasing {
		t.Fatalf("releasing: %+v %v", got, err)
	}
	if err = p.Release(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deletes != 0 {
		t.Fatal("repeated deletion of terminating volume")
	}
}
