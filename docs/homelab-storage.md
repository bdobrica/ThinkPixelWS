# Homelab storage

## Observed configuration (2026-09-26)

Read-only inspection through `ssh k3spi` and direct worker SSH found:

| Node | Address | Role | RAM |
| --- | --- | --- | --- |
| k3spi-00 | 10.10.10.10 | control plane | 4 GB |
| k3spi-01 | 10.10.10.11 | worker | 8 GB |
| k3spi-02 | 10.10.10.12 | worker | 8 GB |
| k3spi-03 | 10.10.10.13 | worker | 8 GB |

RAM and nominal device sizes are operator-provided. All nodes report arm64,
Debian 13, and K3s `v1.36.4+k3s1`. Each has an ext4 SSD mounted at `/mnt/ssd`
(nominal 256 GB) and an ext4 USB flash drive at `/mnt/usb` (nominal 32 GB).
The SSD filesystems reported roughly 217 GiB available each; this is a point-in-time
observation, not reserved capacity.

No `CSIDriver` resources were registered. StorageClasses were:

- `local-path` (default): `rancher.io/local-path`, `Delete`,
  `WaitForFirstConsumer`, no expansion. Its provisioner configuration uses
  `/mnt/usb/k3s-storage` on each node.
- `ar-bounded-scratch-v1`: `kubernetes.io/no-provisioner`, `Retain`,
  `WaitForFirstConsumer`, no expansion. Existing AR scratch PVs must not be
  repurposed as WS durable storage.

Existing local-path claims include AG, AR, and monitoring data. Kata worker
DaemonSets and the agent-sandbox controller are installed. This inspection did
not run a sandbox or test storage persistence.

## Current choice

The operator confirmed on 2026-09-26 that simple local storage is sufficient for
now. Use the existing `local-path` StorageClass for the homelab integration;
installing or testing CSI is not a prerequisite for the next WS steps. The
working-storage adapter uses the Kubernetes PVC API, preserving the option to
select a CSI-backed StorageClass later without changing the provider-neutral
port. CSI-specific capabilities will need separate validation when used.

This choice accepts node-local durability: replacement execution must run where
the volume resides, and recovery after loss of that node or disk is not promised.
Requested PVC capacity must not be presented as an enforced filesystem quota.
The service integration described below still needs implementation.

## Democratic-CSI assessment

No driver was installed and no cluster resources, mounts, or disks were changed.
No ZFS executables or mounted NFS/SMB/ZFS filesystems were found on these nodes;
the control plane also lacked `exportfs`. This does not rule out an external NAS
that has not been configured on the cluster.

Democratic-CSI supports several storage backends; installing the driver alone
does not create a replicated storage system. Its
[`local-hostpath` backend](https://github.com/democratic-csi/democratic-csi#local-hostpath)
can use directories on the existing SSD filesystems, but binds volumes to their
original node and does not enforce requested volume size. It could exercise CSI
provisioning and same-node Pod replacement, but would not demonstrate node-loss
recovery or bounded writable storage for untrusted execution.

The decision for this inspection is to defer installation: adding this backend
now offers limited benefit over the existing node-local provisioner. A future
installation should choose the actual storage backend first. A separately
provisioned backend with capacity enforcement would be more useful for the WS
execution path. Reformatting the mounted SSDs or migrating existing data is not
part of the current change.

When that backend is available, record pinned driver/chart versions, installation
values, prerequisite host changes, StorageClass configuration, rollback steps,
and actual PVC write/replacement/readback evidence here. Qualify Kata attachment
separately. Do not treat a `Bound` PVC as proof of capacity enforcement, WS writer
fencing, snapshots, or recovery after loss of its storage node.

## WS service integration

The Kubernetes client and working-storage provider currently exist as Go
adapters. Named profiles select a StorageClass, capacity request, and RWO/RWOP
access mode; see [configuration](configuration.md#kubernetes-working-storage-client).

“Process wiring” means adding service configuration loading, constructing the
client/provider at startup, and invoking it from authorized Materialization
operations while persisting handles and lifecycle transitions. That connection
is not implemented yet. Installing a CSI driver does not implement it, and the
absence of a CSI driver does not prevent work on it. See [TODO](../TODO.md) for
implementation sequencing.

## Repeating the inspection

Use the operator's existing SSH configuration and cluster credentials; do not
copy kubeconfigs or private keys into this repository.

```sh
ssh k3spi 'kubectl get nodes -o wide; kubectl get storageclasses; kubectl get csidrivers'
ssh k3spi 'kubectl -n kube-system get configmap local-path-config -o yaml'
ssh k3spi 'kubectl get pv -o wide; findmnt /mnt/ssd; findmnt /mnt/usb; df -h /mnt/ssd /mnt/usb'
```

For each worker address above, inspect `findmnt /mnt/ssd`, `findmnt /mnt/usb`,
`df -h /mnt/ssd`, `command -v zfs`, `command -v zpool`, and
`findmnt -t nfs,nfs4,cifs,zfs` using authorized direct SSH access.
A missing executable or matching mount can make these inspection commands exit
nonzero; it is not an instruction to install software or modify a filesystem.

## Sandbox deletion survival test

`TestSandboxDeletionPreservesHotStorage` is an opt-in live test. It allocates a
64Mi PVC through the WS provider, creates an `agents.x-k8s.io/v1beta1` Sandbox
using the existing claim, writes an uncommitted marker, then deletes the Sandbox
with foreground propagation. It waits for the Sandbox and all its Pods to vanish,
checks the original PVC UID, PV name and provider handle, and reads the marker
through a new read-only Pod. The test creates a unique `ws-rec004-*` namespace;
cleanup deletes only that namespace and waits for its disappearance. A terminated
test process may leave that named namespace for operator cleanup.

Passed on 2026-09-26 in 80.76 seconds, including namespace cleanup, with
`local-path` on `k3spi-02`. The original PVC UID
`6ca62117-560d-4cec-b3ab-2ba652c72b6e` and PV binding survived foreground
Sandbox deletion, and the read-only verifier returned the original marker.

Run against the homelab without copying credentials off the control plane:

```sh
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go test -c \
  -o /tmp/ws-rec004.test ./internal/adapters/workingstorage/kubernetes
scp /tmp/ws-rec004.test k3spi:/tmp/ws-rec004.test
ssh k3spi 'THINKPIXELWS_TEST_KUBECONFIG=/etc/rancher/k3s/k3s.yaml \
  THINKPIXELWS_TEST_STORAGE_CLASS=local-path \
  THINKPIXELWS_TEST_STORAGE_IMAGE=docker.io/library/busybox@sha256:9db7b59979c38555a39def84a31fb98b5296952f9e3afd4f6f11f05b07adfab0 \
  /tmp/ws-rec004.test -test.run=^TestSandboxDeletionPreservesHotStorage$ \
  -test.v -test.timeout=8m'
```

For another cluster, set these three environment variables and run the named Go
test locally. The credential must permit namespace creation/deletion, PVC and Pod
operations, Pod logs, API discovery and Sandbox operations. The selected image
must provide `sh`, `cat`, `sync` and `sleep`. Normal unit tests skip this test when
`THINKPIXELWS_TEST_KUBECONFIG` is unset.

This uses the real Sandbox controller and default container runtime, not Kata or
the AR service. It does not qualify AG grants, PostgreSQL persistence, preparation
from a generation, lease renewal, writable replacement execution, or node/disk
loss. AR must preserve the claim and its namespace during execution cleanup; see
[storage lifetime](contracts/providers.md#storage-lifetime-after-sandbox-deletion).
