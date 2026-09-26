package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/bdobrica/ThinkPixelWS/internal/ports"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// This opt-in test creates and deletes its own namespace, a provider PVC, a real
// agent-sandbox Sandbox and a read-only verification Pod. It needs an installed
// agents.x-k8s.io/v1beta1 controller and a dynamically provisioned StorageClass.
// It exercises storage lifetime, not AR/AG authorization or writer recovery.
func TestSandboxDeletionPreservesHotStorage(t *testing.T) {
	kubeconfig := os.Getenv("THINKPIXELWS_TEST_KUBECONFIG")
	if kubeconfig == "" {
		t.Skip("set THINKPIXELWS_TEST_KUBECONFIG to enable the live cluster test")
	}
	class := os.Getenv("THINKPIXELWS_TEST_STORAGE_CLASS")
	if class == "" {
		t.Fatal("THINKPIXELWS_TEST_STORAGE_CLASS must explicitly select test storage")
	}
	image := os.Getenv("THINKPIXELWS_TEST_STORAGE_IMAGE")
	if image == "" {
		t.Fatal("THINKPIXELWS_TEST_STORAGE_IMAGE must select a trusted image with sh, cat, sync and sleep")
	}
	namespace := "ws-rec004-" + uuid.NewString()
	client, err := New(Config{Kubeconfig: kubeconfig, Namespace: namespace})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	ns, err := client.Core.Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("isolated namespace: %s", namespace)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		err := client.Core.Namespaces().Delete(cleanup, namespace, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &ns.UID}})
		if err != nil {
			t.Errorf("cleanup namespace %s: %v", namespace, err)
			return
		}
		err = wait.PollUntilContextCancel(cleanup, time.Second, true, func(ctx context.Context) (bool, error) {
			_, err := client.Core.Namespaces().Get(ctx, namespace, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		})
		if err != nil {
			t.Errorf("cleanup namespace %s: %v", namespace, err)
		}
	})
	p, err := NewProvider(ctx, client, ProviderConfig{TargetID: "rec004-test", Profile: "durable", Profiles: map[string]StorageProfile{"durable": {StorageClass: class, Capacity: "64Mi"}}})
	if err != nil {
		t.Fatal(err)
	}
	m, err := (domain.NewMaterialization{TenantID: uuid.Must(uuid.NewV7()), WorkspaceID: uuid.Must(uuid.NewV7()), ID: uuid.Must(uuid.NewV7()), BaseGeneration: 1, Provider: "kubernetes", Target: domain.MaterializationTarget{ID: "rec004-test", Region: "test", StorageClass: class}, Mode: domain.MaterializationReadWrite}).Materialization(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	m, err = m.TransitionState(domain.MaterializationPreparing, m.StateVersion, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	allocated, err := p.Allocate(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	m, err = m.Bind(allocated.Handle, m.StateVersion, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// With WaitForFirstConsumer storage the first Pod must exist before binding.
	// This synthetic writer represents execution; no preparation/lease is claimed.
	marker := "uncommitted-work-" + m.ID.String()
	podSpec := corev1.PodSpec{
		AutomountServiceAccountToken: boolPtr(false),
		RestartPolicy:                corev1.RestartPolicyAlways,
		Containers: []corev1.Container{{Name: "writer", Image: image,
			Command:        []string{"sh", "-ec", fmt.Sprintf("printf '%%s\\n' '%s' > /workspace/continuation.txt; sync; exec sleep 3600", marker)},
			VolumeMounts:   []corev1.VolumeMount{{Name: "workspace", MountPath: "/workspace"}},
			ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"cat", "/workspace/continuation.txt"}}}, PeriodSeconds: 1},
		}},
		Volumes: []corev1.Volume{{Name: "workspace", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName(m)}}}},
	}
	sandbox := map[string]any{"apiVersion": "agents.x-k8s.io/v1beta1", "kind": "Sandbox", "metadata": map[string]any{"name": "writer", "namespace": namespace}, "spec": map[string]any{"podTemplate": map[string]any{"spec": podSpec}}}
	body, err := json.Marshal(sandbox)
	if err != nil {
		t.Fatal(err)
	}
	path := "/apis/agents.x-k8s.io/v1beta1/namespaces/" + namespace + "/sandboxes"
	raw, err := client.Discovery.RESTClient().Post().AbsPath(path).Body(body).DoRaw(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		Metadata metav1.ObjectMeta `json:"metadata"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatal(err)
	}
	var writer *corev1.Pod
	err = wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
		pods, err := client.Core.Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return false, err
		}
		for i := range pods.Items {
			pod := &pods.Items[i]
			for _, owner := range pod.OwnerReferences {
				if owner.UID != created.Metadata.UID {
					continue
				}
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
						writer = pod.DeepCopy()
						return true, nil
					}
				}
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("wait for sandbox writer: %v", err)
	}
	claims := client.Core.PersistentVolumeClaims(namespace)
	before, err := claims.Get(ctx, pvcName(m), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !p.owned(before, m) || before.Status.Phase != corev1.ClaimBound || before.Spec.VolumeName == "" {
		t.Fatal("sandbox changed independent PVC ownership or volume is not bound")
	}
	t.Logf("sandbox UID=%s pod UID=%s node=%s PVC UID=%s PV=%s", created.Metadata.UID, writer.UID, writer.Spec.NodeName, before.UID, before.Spec.VolumeName)
	policy := metav1.DeletePropagationForeground
	opts, err := json.Marshal(metav1.DeleteOptions{PropagationPolicy: &policy, Preconditions: &metav1.Preconditions{UID: &created.Metadata.UID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Discovery.RESTClient().Delete().AbsPath(path + "/writer").Body(opts).Do(ctx).Error(); err != nil {
		t.Fatal(err)
	}
	err = wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
		err := client.Discovery.RESTClient().Get().AbsPath(path + "/writer").Do(ctx).Error()
		if !apierrors.IsNotFound(err) {
			return false, err
		}
		pods, err := client.Core.Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return false, err
		}
		return len(pods.Items) == 0, nil
	})
	if err != nil {
		t.Fatalf("wait for sandbox and all execution Pods to disappear: %v", err)
	}
	after, err := claims.Get(ctx, pvcName(m), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if after.UID != before.UID || after.Spec.VolumeName != before.Spec.VolumeName || after.DeletionTimestamp != nil || !p.owned(after, m) {
		t.Fatal("hot storage identity/lifetime changed after sandbox deletion")
	}
	status, err := p.Status(ctx, m)
	if err != nil || status.Handle != allocated.Handle || status.Phase != ports.WorkingStorageBound {
		t.Fatalf("storage after deletion: %+v %v", status, err)
	}
	// Mount only after confirmed deletion, read-only. This verifies bytes survived;
	// it does not implement replacement writable execution (REC-005).
	verifier := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "verify"}, Spec: corev1.PodSpec{
		AutomountServiceAccountToken: boolPtr(false), RestartPolicy: corev1.RestartPolicyNever,
		Containers: []corev1.Container{{Name: "reader", Image: image, Command: []string{"cat", "/workspace/continuation.txt"}, VolumeMounts: []corev1.VolumeMount{{Name: "workspace", MountPath: "/workspace", ReadOnly: true}}}},
		Volumes:    []corev1.Volume{{Name: "workspace", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: after.Name, ReadOnly: true}}}},
	}}
	if _, err := client.Core.Pods(namespace).Create(ctx, verifier, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	err = wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
		pod, err := client.Core.Pods(namespace).Get(ctx, "verify", metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if pod.Status.Phase == corev1.PodFailed {
			return false, fmt.Errorf("read-only verification Pod failed")
		}
		return pod.Status.Phase == corev1.PodSucceeded, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := client.Core.Pods(namespace).GetLogs("verify", &corev1.PodLogOptions{Container: "reader"}).DoRaw(ctx)
	if err != nil || strings.TrimSpace(string(output)) != marker {
		t.Fatalf("persisted content mismatch: %v", err)
	}
	t.Log("sandbox and writer gone; original PVC/PV and uncommitted bytes preserved")
}

func boolPtr(v bool) *bool { return &v }
