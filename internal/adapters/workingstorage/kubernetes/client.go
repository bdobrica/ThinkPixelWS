// Package kubernetes contains the Kubernetes working-storage adapter.
// Cluster credentials belong to the WS service, never to Workspace contents.
package kubernetes

import (
	"errors"
	"time"

	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/discovery"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// Config is trusted operator configuration. Kubeconfig is a secret file locator;
// kubeconfigs can execute credential plugins and must never come from a Workspace.
// An empty Kubeconfig selects in-cluster credentials, without local-file fallback.
type Config struct {
	Kubeconfig string
	Context    string
	Namespace  string
	Timeout    time.Duration
}

// Client holds credentials privately in client-go transports. Namespace is the
// configured resource scope, not a substitute for Kubernetes RBAC or AG authority.
type Client struct {
	Core      corev1.CoreV1Interface
	Discovery discovery.DiscoveryInterface
	Namespace string
}

// New loads credentials and constructs clients without contacting the API server.
// Provider startup must separately discover required capabilities before use.
func New(cfg Config) (*Client, error) {
	return newClient(cfg, rest.InClusterConfig)
}

func newClient(cfg Config, inCluster func() (*rest.Config, error)) (*Client, error) {
	if len(validation.IsDNS1123Label(cfg.Namespace)) != 0 {
		return nil, errors.New("kubernetes namespace must be an explicit DNS label")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Timeout < 0 || cfg.Timeout > 5*time.Minute {
		return nil, errors.New("kubernetes timeout must be positive and at most 5m")
	}
	if cfg.Kubeconfig == "" && cfg.Context != "" {
		return nil, errors.New("kubernetes context requires an explicit kubeconfig")
	}
	var rc *rest.Config
	var err error
	if cfg.Kubeconfig == "" {
		rc, err = inCluster()
	} else {
		rules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: cfg.Kubeconfig}
		overrides := &clientcmd.ConfigOverrides{CurrentContext: cfg.Context}
		var raw *clientcmdapi.Config
		raw, err = rules.Load()
		if err == nil {
			rc, err = clientcmd.NewNonInteractiveClientConfig(*raw, cfg.Context, overrides, rules).ClientConfig()
		}
	}
	// Do not propagate loader errors that can contain credential-bearing input.
	if err != nil {
		return nil, errors.New("load kubernetes credentials: check service account or explicit kubeconfig configuration")
	}
	rc = rest.CopyConfig(rc)
	rc.Timeout = cfg.Timeout
	rc.UserAgent = "thinkpixelws"
	core, err := corev1.NewForConfig(rc)
	if err != nil {
		return nil, errors.New("construct kubernetes core client: invalid transport configuration")
	}
	discover, err := discovery.NewDiscoveryClientForConfig(rc)
	if err != nil {
		return nil, errors.New("construct kubernetes discovery client: invalid transport configuration")
	}
	return &Client{Core: core, Discovery: discover, Namespace: cfg.Namespace}, nil
}
