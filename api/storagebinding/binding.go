// Package storagebinding defines storage instructions for trusted runtime adapters.
// These descriptions contain no credentials and never grant permission to attach.
package storagebinding

import "encoding/json"

// Binding is the result of resolving a WS Materialization handle. Kind selects
// a versioned adapter contract; consumers must reject unsupported kinds.
// Handle pins provider identity and is opaque outside the selected adapter.
type Binding struct {
	Handle    string          `json:"handle"`
	Kind      string          `json:"kind"`
	Reference json.RawMessage `json:"reference"`
}

const PVCBindingKind = "kubernetes-pvc-v1"

// PVCBinding is consumed by AR's Kubernetes adapter, never untrusted execution.
// Check ClaimUID against the live PVC before attachment: Pod volume references
// use names and cannot pin UIDs. ReadOnly applies to the volume and every mount.
// Namespace must match the sandbox namespace. MountPath is /workspace.
type PVCBinding struct {
	Namespace string `json:"namespace"`
	ClaimName string `json:"claimName"`
	ClaimUID  string `json:"claimUid"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly"`
}
