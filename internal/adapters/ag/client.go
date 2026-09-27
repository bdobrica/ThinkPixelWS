// Package ag provides the authenticated transport for trusted AG service calls.
// It does not define a grant exchange or grant execution authority.
package ag

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var (
	ErrConfiguration = errors.New("invalid AG service authentication configuration")
	ErrRequest       = errors.New("AG service request failed")
)

// Config is deployment-owned. Files are trusted local secret locators, never
// Workspace content. WorkloadURI must match the exact URI SAN bound by AG to
// the WS service principal and tenant. AG assigns roles and authorizes actions.
type Config struct {
	Origin          string
	CAFile          string
	CertificateFile string
	KeyFile         string
	WorkloadURI     string
}

// Client keeps its transport private so callers cannot disable TLS verification,
// change trust roots, follow redirects, or send the credential to another origin.
// Credentials are loaded once; replace the client after certificate rotation.
type Client struct {
	origin    string
	client    *http.Client
	transport *http.Transport
}

func NewClient(cfg Config) (*Client, error) {
	u, err := url.Parse(cfg.Origin)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return nil, ErrConfiguration
	}
	identity, err := url.Parse(cfg.WorkloadURI)
	if err != nil || identity.Scheme == "" || identity.Host == "" || strings.TrimSpace(cfg.WorkloadURI) != cfg.WorkloadURI {
		return nil, ErrConfiguration
	}
	roots := x509.NewCertPool()
	pem, err := os.ReadFile(cfg.CAFile)
	if err != nil || !roots.AppendCertsFromPEM(pem) {
		return nil, ErrConfiguration
	}
	cert, err := tls.LoadX509KeyPair(cfg.CertificateFile, cfg.KeyFile)
	if err != nil || len(cert.Certificate) == 0 {
		return nil, ErrConfiguration
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil || len(leaf.URIs) != 1 || leaf.URIs[0].String() != cfg.WorkloadURI {
		return nil, ErrConfiguration
	}
	transport := &http.Transport{
		// No environment proxy: credentials travel directly to the pinned AG origin.
		DialContext:            (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, Certificates: []tls.Certificate{cert}},
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  5 * time.Second,
		IdleConnTimeout:        30 * time.Second,
		MaxIdleConns:           2,
		MaxIdleConnsPerHost:    2,
		MaxResponseHeaderBytes: 64 << 10,
	}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return ErrRequest },
	}
	return &Client{origin: u.Host, client: client, transport: transport}, nil
}

// Do sends only requests to the configured HTTPS origin. The caller supplies a
// published AG route, owns/limits/closes the response body, and must interpret
// non-success responses as denial/unavailability, never as execution authority.
// Request context cancellation and a five-second total deadline are enforced.
// Transport errors are sanitized; credentials and upstream details are omitted.
func (c *Client) Do(r *http.Request) (*http.Response, error) {
	if r == nil || r.URL == nil || r.URL.Scheme != "https" || r.URL.Host != c.origin || r.URL.User != nil || r.URL.Opaque != "" || r.URL.Fragment != "" || (r.Host != "" && r.Host != c.origin) {
		return nil, ErrRequest
	}
	for _, headers := range []http.Header{r.Header, r.Trailer} {
		for name := range headers {
			key := strings.ToLower(name)
			if key == "authorization" || key == "proxy-authorization" || key == "cookie" || key == "forwarded" || strings.HasPrefix(key, "x-forwarded-") || key == "x-tenant-id" || key == "x-principal-id" || key == "x-roles" {
				return nil, ErrRequest
			}
		}
	}
	response, err := c.client.Do(r)
	if err != nil {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		return nil, ErrRequest
	}
	return response, nil
}

func (c *Client) CloseIdleConnections() { c.transport.CloseIdleConnections() }
