package kubernetes

import (
	"context"
	"errors"
	"testing"

	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	corev1 "k8s.io/api/core/v1"
)

func TestSelectedStorageProfile(t *testing.T) {
	for _, access := range []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce, corev1.ReadWriteOncePod} {
		t.Run(string(access), func(t *testing.T) {
			base, f, m := providerFixture(t)
			cfg := testProviderConfig("test-target", "unused", "1Gi")
			cfg.Profile = "durable"
			cfg.Profiles["durable"] = StorageProfile{StorageClass: "csi-ssd", Capacity: "8Gi", AccessMode: access}
			p, err := NewProvider(context.Background(), &base.client, cfg)
			if err != nil {
				t.Fatal(err)
			}
			// Mutating the caller's map cannot change an already constructed provider.
			cfg.Profiles["durable"] = StorageProfile{StorageClass: "other", Capacity: "99Gi"}
			m.Target.StorageClass = "csi-ssd"
			first, err := p.Allocate(context.Background(), m)
			if err != nil {
				t.Fatal(err)
			}
			f.mu.Lock()
			pvc := f.pvc.DeepCopy()
			f.mu.Unlock()
			if *pvc.Spec.StorageClassName != "csi-ssd" || pvc.Spec.Resources.Requests.Storage().String() != "8Gi" || len(pvc.Spec.AccessModes) != 1 || pvc.Spec.AccessModes[0] != access || *pvc.Spec.VolumeMode != corev1.PersistentVolumeFilesystem || pvc.Annotations["thinkpixel.io/storage-profile"] != "durable" {
				t.Fatalf("incorrect profile PVC: %+v", pvc.Spec)
			}
			again, err := p.Allocate(context.Background(), m)
			if err != nil || again != first {
				t.Fatalf("retry: %+v %v", again, err)
			}
			// A profile update must not resize or adopt existing storage on retry.
			changed := testProviderConfig("test-target", "csi-ssd", "16Gi")
			changed.Profile = "durable"
			changed.Profiles["durable"] = StorageProfile{StorageClass: "csi-ssd", Capacity: "16Gi", AccessMode: access}
			replacement, err := NewProvider(context.Background(), &base.client, changed)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = replacement.Allocate(context.Background(), m); !errors.Is(err, ports.ErrWorkingStorageConflict) {
				t.Fatalf("profile drift: %v", err)
			}
			m.Handle = first.Handle
			if _, err = replacement.Status(context.Background(), m); err != nil {
				t.Fatalf("status after profile change: %v", err)
			}
			if err = replacement.Release(context.Background(), m); err != nil {
				t.Fatalf("release after profile change: %v", err)
			}
		})
	}
}

func TestInvalidStorageProfileDoesNotContactCluster(t *testing.T) {
	base, f, _ := providerFixture(t)
	for _, kind := range []string{"missing", "unknown", "invalid name", "empty class", "zero capacity", "shared access", "read only access", "block"} {
		t.Run(kind, func(t *testing.T) {
			cfg := testProviderConfig("test-target", "ssd", "1Gi")
			profile := cfg.Profiles[cfg.Profile]
			switch kind {
			case "missing":
				cfg.Profile = ""
			case "unknown":
				cfg.Profile = "absent"
			case "invalid name":
				cfg.Profile = "invalid/name"
			case "empty class":
				profile.StorageClass = ""
			case "zero capacity":
				profile.Capacity = "0"
			case "shared access":
				profile.AccessMode = corev1.ReadWriteMany
			case "read only access":
				profile.AccessMode = corev1.ReadOnlyMany
			case "block":
				profile.AccessMode = "Block"
			}
			cfg.Profiles["standard"] = profile
			f.mu.Lock()
			calls := f.discoveryCalls
			f.mu.Unlock()
			if _, err := NewProvider(context.Background(), &base.client, cfg); err == nil {
				t.Fatal("accepted invalid profile")
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if calls != f.discoveryCalls || f.pvc != nil {
				t.Fatal("invalid profile contacted cluster")
			}
		})
	}
}

func TestProfileIdentityAndAccessModeConflicts(t *testing.T) {
	for _, field := range []string{"profile", "access mode", "target class"} {
		t.Run(field, func(t *testing.T) {
			p, f, m := providerFixture(t)
			if _, err := p.Allocate(context.Background(), m); err != nil {
				t.Fatal(err)
			}
			f.mu.Lock()
			switch field {
			case "profile":
				f.pvc.Annotations["thinkpixel.io/storage-profile"] = "other"
			case "access mode":
				f.pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
			case "target class":
				m.Target.StorageClass = "unapproved"
			}
			f.mu.Unlock()
			if _, err := p.Allocate(context.Background(), m); !errors.Is(err, ports.ErrWorkingStorageConflict) {
				t.Fatalf("expected conflict: %v", err)
			}
		})
	}
}
