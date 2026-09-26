package kubernetes

import (
	"context"
	"encoding/json"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	corev1 "k8s.io/api/core/v1"
)

const PVCBindingKind = "kubernetes-pvc-v1"

// PVCBinding is consumed by AR's Kubernetes adapter, not untrusted execution.
// UID must be rechecked before use: Kubernetes Pod PVC references use names and
// cannot pin a UID. ReadOnly applies to both the PVC volume source and every mount.
type PVCBinding struct {
	Namespace string `json:"namespace"`
	ClaimName string `json:"claimName"`
	ClaimUID  string `json:"claimUid"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly"`
}

var _ ports.WorkingStorageBindingProvider = (*Provider)(nil)

func (p *Provider) Binding(ctx context.Context, m domain.Materialization) (ports.WorkingStorageBinding, error) {
	if err := ctx.Err(); err != nil {
		return ports.WorkingStorageBinding{}, err
	}
	if m.State != domain.MaterializationReady && m.State != domain.MaterializationActive {
		return ports.WorkingStorageBinding{}, ports.ErrMaterializationStateConflict
	}
	pvc, err := p.lookup(ctx, m)
	if err != nil {
		return ports.WorkingStorageBinding{}, err
	}
	if storageResult(pvc).Phase != ports.WorkingStorageBound {
		return ports.WorkingStorageBinding{}, ports.ErrWorkingStorageConflict
	}
	if pvc.Spec.VolumeMode == nil || *pvc.Spec.VolumeMode != corev1.PersistentVolumeFilesystem {
		return ports.WorkingStorageBinding{}, ports.ErrWorkingStorageConflict
	}
	reference, err := json.Marshal(PVCBinding{Namespace: pvc.Namespace, ClaimName: pvc.Name, ClaimUID: string(pvc.UID), MountPath: "/workspace", ReadOnly: m.Mode == domain.MaterializationReadOnly})
	if err != nil {
		return ports.WorkingStorageBinding{}, err
	}
	return ports.WorkingStorageBinding{Handle: m.Handle, Kind: PVCBindingKind, Reference: reference}, nil
}
