package ag

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

const workloadURI = "spiffe://thinkpixel.test/workloads/ws"

func TestMutualAuthentication(t *testing.T) {
	f := newTLSFixture(t)
	var calls atomic.Int32
	s := f.server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates[0].URIs) != 1 || r.TLS.PeerCertificates[0].URIs[0].String() != workloadURI {
			t.Error("missing verified WS workload identity")
		}
		// Service identity alone does not authorize an action.
		w.WriteHeader(http.StatusForbidden)
	}))
	c := f.client(t, s.URL)
	r, _ := http.NewRequest(http.MethodPost, s.URL+"/fixture", nil)
	response, err := c.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden || calls.Load() != 1 {
		t.Fatal("did not preserve independent authorization denial")
	}

	for _, name := range []string{"Authorization", "Proxy-Authorization", "X-Tenant-ID", "X-Principal-ID", "X-Roles", "Forwarded", "X-Forwarded-Client-Cert", "Cookie"} {
		t.Run(name, func(t *testing.T) {
			r := r.Clone(context.Background())
			r.Header[name] = []string{"credential-must-not-leak"}
			if response, err := c.Do(r); response != nil || err != ErrRequest {
				t.Fatal("accepted forbidden header")
			}
		})
	}
	for _, target := range []string{"http" + s.URL[5:] + "/fixture", "https://untrusted.example/fixture", "https://secret@" + s.Listener.Addr().String() + "/fixture"} {
		r, _ := http.NewRequest(http.MethodPost, target, nil)
		if response, err := c.Do(r); response != nil || err != ErrRequest {
			t.Fatal("accepted untrusted origin")
		}
	}
	r.Host = "untrusted.example"
	if response, err := c.Do(r); response != nil || err != ErrRequest {
		t.Fatal("accepted Host override")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, _ = http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if response, err := c.Do(r); response != nil || err != ErrRequest {
		t.Fatal("accepted cancelled request")
	}
	if calls.Load() != 1 {
		t.Fatal("rejected requests reached AG")
	}
}

func TestRejectRedirect(t *testing.T) {
	f := newTLSFixture(t)
	var redirected atomic.Int32
	target := f.server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	s := f.server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/secret", http.StatusTemporaryRedirect)
	}))
	c := f.client(t, s.URL)
	r, _ := http.NewRequest(http.MethodGet, s.URL, nil)
	if response, err := c.Do(r); response != nil || err != ErrRequest {
		t.Fatal("accepted redirect")
	}
	if redirected.Load() != 0 {
		t.Fatal("followed redirect with service identity")
	}
}

func TestRejectUntrustedPeers(t *testing.T) {
	for _, scenario := range []string{"untrusted-server", "wrong-hostname", "untrusted-client", "expired-client", "wrong-client-usage"} {
		t.Run(scenario, func(t *testing.T) {
			f := newTLSFixture(t)
			var calls atomic.Int32
			s := f.server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
			cfg := f.config(s.URL)
			switch scenario {
			case "untrusted-server":
				cfg.CAFile = newTLSFixture(t).caFile
			case "wrong-hostname":
				u, _ := url.Parse(s.URL)
				cfg.Origin = "https://localhost:" + u.Port()
			case "untrusted-client":
				other := newTLSFixture(t)
				cfg.CertificateFile, cfg.KeyFile = other.certFile, other.keyFile
			case "expired-client", "wrong-client-usage":
				template := clientTemplate()
				if scenario == "expired-client" {
					template.NotAfter = time.Now().Add(-time.Minute)
				} else {
					template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
				}
				cert, key := issue(t, template, f.ca, f.key)
				cfg.CertificateFile, cfg.KeyFile = writePair(t, cert, key)
			}
			c, err := NewClient(cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(c.CloseIdleConnections)
			r, _ := http.NewRequest(http.MethodGet, cfg.Origin+"/fixture", nil)
			if response, err := c.Do(r); response != nil || err != ErrRequest {
				t.Fatal("accepted untrusted peer")
			}
			if calls.Load() != 0 {
				t.Fatal("unauthenticated request reached handler")
			}
		})
	}
}

func TestInvalidConfiguration(t *testing.T) {
	f := newTLSFixture(t)
	for _, mutate := range []func(*Config){
		func(c *Config) { c.Origin = "http://ag.example" },
		func(c *Config) { c.Origin += "/path" },
		func(c *Config) { c.Origin += "?secret=hidden" },
		func(c *Config) { c.CAFile = "missing-secret-file" },
		func(c *Config) { c.KeyFile = "missing-secret-file" },
		func(c *Config) { c.WorkloadURI = "spiffe://thinkpixel.test/other" },
	} {
		cfg := f.config("https://ag.example")
		mutate(&cfg)
		if c, err := NewClient(cfg); c != nil || err != ErrConfiguration {
			t.Fatal("accepted invalid configuration")
		}
	}
	template := clientTemplate()
	template.URIs = append(template.URIs, template.URIs[0])
	cert, key := issue(t, template, f.ca, f.key)
	cfg := f.config("https://ag.example")
	cfg.CertificateFile, cfg.KeyFile = writePair(t, cert, key)
	if c, err := NewClient(cfg); c != nil || err != ErrConfiguration {
		t.Fatal("accepted ambiguous workload identity")
	}
}

type tlsFixture struct {
	ca                        *x509.Certificate
	key                       *ecdsa.PrivateKey
	caFile, certFile, keyFile string
}

func newTLSFixture(t *testing.T) *tlsFixture {
	t.Helper()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, key := issue(t, ca, nil, nil)
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	cert, clientKey := issue(t, clientTemplate(), parsed, key)
	certFile, keyFile := writePair(t, cert, clientKey)
	return &tlsFixture{ca: parsed, key: key, caFile: caFile, certFile: certFile, keyFile: keyFile}
}

func clientTemplate() *x509.Certificate {
	u, _ := url.Parse(workloadURI)
	return &x509.Certificate{SerialNumber: big.NewInt(2), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, URIs: []*url.URL{u}}
}

func (f *tlsFixture) config(origin string) Config {
	return Config{Origin: origin, CAFile: f.caFile, CertificateFile: f.certFile, KeyFile: f.keyFile, WorkloadURI: workloadURI}
}

func (f *tlsFixture) client(t *testing.T, origin string) *Client {
	t.Helper()
	c, err := NewClient(f.config(origin))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.CloseIdleConnections)
	return c
}

func (f *tlsFixture) server(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	cert, key := issue(t, &x509.Certificate{SerialNumber: big.NewInt(3), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}, f.ca, f.key)
	roots := x509.NewCertPool()
	roots.AddCert(f.ca)
	s := httptest.NewUnstartedServer(handler)
	s.Config.ErrorLog = log.New(io.Discard, "", 0)
	s.TLS = &tls.Config{MinVersion: tls.VersionTLS12, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots, Certificates: []tls.Certificate{{Certificate: [][]byte{cert}, PrivateKey: key}}}
	s.StartTLS()
	t.Cleanup(s.Close)
	return s
}

func issue(t *testing.T, template, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) ([]byte, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if parent == nil {
		parent, parentKey = template, key
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, parentKey)
	if err != nil {
		t.Fatal(err)
	}
	return der, key
}

func writePair(t *testing.T, cert []byte, key *ecdsa.PrivateKey) (string, string) {
	t.Helper()
	dir := t.TempDir()
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}
