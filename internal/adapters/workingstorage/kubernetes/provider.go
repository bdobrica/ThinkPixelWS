package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

// ProviderConfig is operator-selected placement for a single storage target.
// StorageClass must be explicit; no cluster default or caller-supplied PVC spec
// is used. Profile selection and scheduling policy belong above this adapter.
type ProviderConfig struct {
	TargetID     string
	StorageClass string
	Capacity     string
}

type Provider struct {
	client   Client
	config   ProviderConfig
	capacity resource.Quantity
}

var _ ports.WorkingStorageProvider = (*Provider)(nil)

func NewProvider(ctx context.Context, client *Client, cfg ProviderConfig) (*Provider, error) {
	if client == nil || client.Core == nil || client.Discovery == nil || len(validation.IsDNS1123Label(client.Namespace)) != 0 {
		return nil, errors.New("invalid kubernetes provider client")
	}
	if strings.TrimSpace(cfg.TargetID) != cfg.TargetID || cfg.TargetID == "" || len(cfg.TargetID) > 256 || len(validation.IsDNS1123Subdomain(cfg.StorageClass)) != 0 {
		return nil, errors.New("invalid kubernetes storage target")
	}
	capacity, err := resource.ParseQuantity(cfg.Capacity)
	if err != nil || capacity.Sign() <= 0 {
		return nil, errors.New("kubernetes storage capacity must be positive")
	}
	p := &Provider{client: *client, config: cfg, capacity: capacity}
	if err := p.checkAPI(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

// checkAPI uses uncached, context-aware discovery at startup and before each
// operation. This checks core PVC support, not CSI snapshot/clone capabilities.
func (p *Provider) checkAPI(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	rc := p.client.Discovery.RESTClient()
	if rc == nil {
		return ports.ErrWorkingStorageUnavailable
	}
	var resources metav1.APIResourceList
	if err := rc.Get().AbsPath("/api/v1").Do(ctx).Into(&resources); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ports.ErrWorkingStorageUnavailable
	}
	for _, r := range resources.APIResources {
		if r.Name != "persistentvolumeclaims" || !r.Namespaced || r.Kind != "PersistentVolumeClaim" {
			continue
		}
		verbs := map[string]bool{}
		for _, v := range r.Verbs {
			verbs[v] = true
		}
		if verbs["create"] && verbs["get"] && verbs["delete"] {
			return nil
		}
	}
	return ports.ErrWorkingStorageUnavailable
}

func (p *Provider) validate(m domain.Materialization, allocate bool) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if m.Provider != "kubernetes" || m.Target.ID != p.config.TargetID || (m.Target.StorageClass != "" && m.Target.StorageClass != p.config.StorageClass) {
		return ports.ErrWorkingStorageConflict
	}
	if allocate {
		if m.State != domain.MaterializationPreparing || m.Handle != "" {
			return ports.ErrWorkingStorageConflict
		}
	} else if !strings.HasPrefix(string(m.Handle), "k8s-pvc-v1:") || len(m.Handle) <= len("k8s-pvc-v1:") {
		return ports.ErrWorkingStorageConflict
	}
	return nil
}

func pvcName(m domain.Materialization) string { return "ws-" + m.ID.String() }

func ownership(m domain.Materialization) map[string]string {
	return map[string]string{
		"app.kubernetes.io/managed-by":  "thinkpixelws",
		"thinkpixel.io/tenant":          m.TenantID.String(),
		"thinkpixel.io/workspace":       m.WorkspaceID.String(),
		"thinkpixel.io/materialization": m.ID.String(),
		"thinkpixel.io/generation":      strconv.FormatUint(m.BaseGeneration, 10),
		"thinkpixel.io/mode":            string(m.Mode),
	}
}

func (p *Provider) owned(pvc *corev1.PersistentVolumeClaim, m domain.Materialization) bool {
	if pvc.Name != pvcName(m) || pvc.Namespace != p.client.Namespace || pvc.UID == "" || len(pvc.OwnerReferences) != 0 {
		return false
	}
	for k, v := range ownership(m) {
		if pvc.Labels[k] != v {
			return false
		}
	}
	return pvc.Annotations["thinkpixel.io/target"] == p.config.TargetID
}

func storageResult(pvc *corev1.PersistentVolumeClaim) ports.WorkingStorage {
	phase := ports.WorkingStoragePending
	if pvc.DeletionTimestamp != nil {
		phase = ports.WorkingStorageReleasing
	} else {
		switch pvc.Status.Phase {
		case corev1.ClaimBound:
			phase = ports.WorkingStorageBound
		case corev1.ClaimLost:
			phase = ports.WorkingStorageLost
		}
	}
	return ports.WorkingStorage{Handle: domain.MaterializationHandle("k8s-pvc-v1:" + string(pvc.UID)), Phase: phase}
}

func (p *Provider) Allocate(ctx context.Context, m domain.Materialization) (ports.WorkingStorage, error) {
	if err := p.validate(m, true); err != nil {
		return ports.WorkingStorage{}, err
	}
	if err := p.checkAPI(ctx); err != nil {
		return ports.WorkingStorage{}, err
	}
	mode := corev1.PersistentVolumeFilesystem
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: pvcName(m), Namespace: p.client.Namespace, Labels: ownership(m), Annotations: map[string]string{"thinkpixel.io/target": p.config.TargetID}},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &p.config.StorageClass,
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			VolumeMode:       &mode,
			Resources:        corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: p.capacity.DeepCopy()}},
		},
	}
	claims := p.client.Core.PersistentVolumeClaims(p.client.Namespace)
	actual, err := claims.Create(ctx, pvc, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		actual, err = claims.Get(ctx, pvc.Name, metav1.GetOptions{})
	}
	if err != nil {
		return ports.WorkingStorage{}, storageError(ctx, err)
	}
	// Never adopt a name collision or silently reuse storage with a different spec.
	if !p.owned(actual, m) || actual.DeletionTimestamp != nil || actual.Spec.StorageClassName == nil || *actual.Spec.StorageClassName != p.config.StorageClass ||
		actual.Spec.VolumeMode == nil || *actual.Spec.VolumeMode != mode || len(actual.Spec.AccessModes) != 1 || actual.Spec.AccessModes[0] != corev1.ReadWriteOnce ||
		actual.Spec.Resources.Requests.Storage().Cmp(p.capacity) != 0 || actual.Spec.DataSource != nil || actual.Spec.DataSourceRef != nil || actual.Spec.Selector != nil {
		return ports.WorkingStorage{}, ports.ErrWorkingStorageConflict
	}
	return storageResult(actual), nil
}

func (p *Provider) lookup(ctx context.Context, m domain.Materialization) (*corev1.PersistentVolumeClaim, error) {
	if err := p.validate(m, false); err != nil {
		return nil, err
	}
	if err := p.checkAPI(ctx); err != nil {
		return nil, err
	}
	pvc, err := p.client.Core.PersistentVolumeClaims(p.client.Namespace).Get(ctx, pvcName(m), metav1.GetOptions{})
	if err != nil {
		return nil, storageError(ctx, err)
	}
	if !p.owned(pvc, m) || storageResult(pvc).Handle != m.Handle {
		return nil, ports.ErrWorkingStorageConflict
	}
	return pvc, nil
}

func (p *Provider) Status(ctx context.Context, m domain.Materialization) (ports.WorkingStorage, error) {
	pvc, err := p.lookup(ctx, m)
	if err != nil {
		return ports.WorkingStorage{}, err
	}
	return storageResult(pvc), nil
}

func (p *Provider) Release(ctx context.Context, m domain.Materialization) error {
	pvc, err := p.lookup(ctx, m)
	if errors.Is(err, ports.ErrWorkingStorageNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if pvc.DeletionTimestamp != nil {
		return nil
	}
	// UID prevents deleting a replacement; resourceVersion also guards concurrent
	// changes to ownership metadata between the read and delete.
	err = p.client.Core.PersistentVolumeClaims(p.client.Namespace).Delete(ctx, pvc.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &pvc.UID, ResourceVersion: &pvc.ResourceVersion}})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return storageError(ctx, err)
}

func storageError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if apierrors.IsNotFound(err) {
		return ports.ErrWorkingStorageNotFound
	}
	if apierrors.IsConflict(err) {
		return ports.ErrWorkingStorageConflict
	}
	// Raw API/transport messages may include backend details or credentials.
	return fmt.Errorf("kubernetes request failed: %w", ports.ErrWorkingStorageUnavailable)
}
