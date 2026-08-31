// Package config loads and validates process configuration.
//
// Configuration precedence is: safe defaults, an optional JSON file, then
// THINKPIXELWS_* environment variables. SecretReferences are opaque locators;
// this package deliberately does not resolve secret values.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	envConfigFile       = "THINKPIXELWS_CONFIG_FILE"
	envListenAddress    = "THINKPIXELWS_HTTP_LISTEN_ADDRESS"
	envReadHeader       = "THINKPIXELWS_HTTP_READ_HEADER_TIMEOUT"
	envRequest          = "THINKPIXELWS_HTTP_REQUEST_TIMEOUT"
	envShutdown         = "THINKPIXELWS_HTTP_SHUTDOWN_TIMEOUT"
	envMaxHeaderBytes   = "THINKPIXELWS_HTTP_MAX_HEADER_BYTES"
	envLogLevel         = "THINKPIXELWS_LOG_LEVEL"
	envMetricsAddress   = "THINKPIXELWS_METRICS_LISTEN_ADDRESS"
	envSecretReferences = "THINKPIXELWS_SECRET_REFERENCES"
)

var (
	namePattern     = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)
	providerPattern = regexp.MustCompile(`^[a-z][a-z0-9.-]{0,62}$`)
)

// Config contains process-level configuration. It does not contain secret
// values; credentials are represented only by SecretReference values.
type Config struct {
	HTTP             HTTPConfig                 `json:"http"`
	Log              LogConfig                  `json:"log"`
	Metrics          MetricsConfig              `json:"metrics"`
	SecretReferences map[string]SecretReference `json:"secret_references,omitempty"`
}

type HTTPConfig struct {
	ListenAddress     string        `json:"listen_address"`
	ReadHeaderTimeout time.Duration `json:"-"`
	RequestTimeout    time.Duration `json:"-"`
	ShutdownTimeout   time.Duration `json:"-"`
	MaxHeaderBytes    int           `json:"max_header_bytes"`
}

type LogConfig struct {
	Level string `json:"level"`
}

type MetricsConfig struct {
	ListenAddress string `json:"listen_address"`
}

// SecretReference identifies a secret held by an external provider. Reference
// is a provider-defined identifier and MUST NOT be the secret value itself.
type SecretReference struct {
	Provider  string `json:"provider"`
	Reference string `json:"reference"`
}

// Defaults returns conservative, independently usable process defaults.
// Administrative interfaces bind to loopback unless explicitly configured.
func Defaults() Config {
	return Config{
		HTTP: HTTPConfig{
			ListenAddress:     "127.0.0.1:8080",
			ReadHeaderTimeout: 5 * time.Second,
			RequestTimeout:    30 * time.Second,
			ShutdownTimeout:   15 * time.Second,
			MaxHeaderBytes:    64 * 1024,
		},
		Log:              LogConfig{Level: "info"},
		Metrics:          MetricsConfig{ListenAddress: "127.0.0.1:9090"},
		SecretReferences: make(map[string]SecretReference),
	}
}

// Load reads an optional JSON file, applies environment overrides, and
// validates the result. An empty path means no configuration file.
func Load(path string) (Config, error) {
	return load(path, os.LookupEnv)
}

// LoadFromEnvironment uses THINKPIXELWS_CONFIG_FILE as the optional file path.
func LoadFromEnvironment() (Config, error) {
	path, _ := os.LookupEnv(envConfigFile)
	return load(path, os.LookupEnv)
}

type fileConfig struct {
	HTTP *struct {
		ListenAddress     *string `json:"listen_address"`
		ReadHeaderTimeout *string `json:"read_header_timeout"`
		RequestTimeout    *string `json:"request_timeout"`
		ShutdownTimeout   *string `json:"shutdown_timeout"`
		MaxHeaderBytes    *int    `json:"max_header_bytes"`
	} `json:"http"`
	Log *struct {
		Level *string `json:"level"`
	} `json:"log"`
	Metrics *struct {
		ListenAddress *string `json:"listen_address"`
	} `json:"metrics"`
	SecretReferences *map[string]SecretReference `json:"secret_references"`
}

type lookupEnv func(string) (string, bool)

func load(path string, lookup lookupEnv) (Config, error) {
	cfg := Defaults()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("read configuration file: %w", err)
		}
		var file fileConfig
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&file); err != nil {
			return Config{}, fmt.Errorf("decode configuration file: %w", err)
		}
		if err := ensureJSONEOF(decoder); err != nil {
			return Config{}, fmt.Errorf("decode configuration file: %w", err)
		}
		if err := applyFile(&cfg, file); err != nil {
			return Config{}, err
		}
	}
	if err := applyEnvironment(&cfg, lookup); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyFile(cfg *Config, file fileConfig) error {
	if file.HTTP != nil {
		if file.HTTP.ListenAddress != nil {
			cfg.HTTP.ListenAddress = *file.HTTP.ListenAddress
		}
		if file.HTTP.MaxHeaderBytes != nil {
			cfg.HTTP.MaxHeaderBytes = *file.HTTP.MaxHeaderBytes
		}
		if err := setDuration(file.HTTP.ReadHeaderTimeout, &cfg.HTTP.ReadHeaderTimeout); err != nil {
			return fmt.Errorf("parse http.read_header_timeout: %w", err)
		}
		if err := setDuration(file.HTTP.RequestTimeout, &cfg.HTTP.RequestTimeout); err != nil {
			return fmt.Errorf("parse http.request_timeout: %w", err)
		}
		if err := setDuration(file.HTTP.ShutdownTimeout, &cfg.HTTP.ShutdownTimeout); err != nil {
			return fmt.Errorf("parse http.shutdown_timeout: %w", err)
		}
	}
	if file.Log != nil && file.Log.Level != nil {
		cfg.Log.Level = *file.Log.Level
	}
	if file.Metrics != nil && file.Metrics.ListenAddress != nil {
		cfg.Metrics.ListenAddress = *file.Metrics.ListenAddress
	}
	if file.SecretReferences != nil {
		cfg.SecretReferences = *file.SecretReferences
	}
	return nil
}

func setDuration(value *string, target *time.Duration) error {
	if value == nil {
		return nil
	}
	d, err := time.ParseDuration(*value)
	if err != nil {
		return err
	}
	*target = d
	return nil
}

func applyEnvironment(cfg *Config, lookup lookupEnv) error {
	if v, ok := lookup(envListenAddress); ok {
		cfg.HTTP.ListenAddress = v
	}
	if v, ok := lookup(envLogLevel); ok {
		cfg.Log.Level = v
	}
	if v, ok := lookup(envMetricsAddress); ok {
		cfg.Metrics.ListenAddress = v
	}
	for key, target := range map[string]*time.Duration{
		envReadHeader: &cfg.HTTP.ReadHeaderTimeout,
		envRequest:    &cfg.HTTP.RequestTimeout,
		envShutdown:   &cfg.HTTP.ShutdownTimeout,
	} {
		if v, ok := lookup(key); ok {
			d, err := time.ParseDuration(v)
			if err != nil {
				return fmt.Errorf("parse %s: %w", key, err)
			}
			*target = d
		}
	}
	if v, ok := lookup(envMaxHeaderBytes); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("parse %s: %w", envMaxHeaderBytes, err)
		}
		cfg.HTTP.MaxHeaderBytes = n
	}
	if v, ok := lookup(envSecretReferences); ok {
		var refs map[string]SecretReference
		decoder := json.NewDecoder(strings.NewReader(v))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&refs); err != nil {
			return fmt.Errorf("parse %s: %w", envSecretReferences, err)
		}
		if err := ensureJSONEOF(decoder); err != nil {
			return fmt.Errorf("parse %s: %w", envSecretReferences, err)
		}
		cfg.SecretReferences = refs
	}
	return nil
}

// Validate rejects unsafe or ambiguous configuration.
func (cfg Config) Validate() error {
	var problems []error
	if cfg.SecretReferences == nil {
		problems = append(problems, errors.New("secret_references must be an object, not null"))
	}
	if err := validateAddress("http.listen_address", cfg.HTTP.ListenAddress); err != nil {
		problems = append(problems, err)
	}
	if err := validateAddress("metrics.listen_address", cfg.Metrics.ListenAddress); err != nil {
		problems = append(problems, err)
	}
	if cfg.HTTP.ReadHeaderTimeout <= 0 {
		problems = append(problems, errors.New("http.read_header_timeout must be positive"))
	} else if cfg.HTTP.ReadHeaderTimeout > 30*time.Second {
		problems = append(problems, errors.New("http.read_header_timeout must not exceed 30s"))
	}
	if cfg.HTTP.RequestTimeout <= 0 {
		problems = append(problems, errors.New("http.request_timeout must be positive"))
	} else if cfg.HTTP.RequestTimeout > 5*time.Minute {
		problems = append(problems, errors.New("http.request_timeout must not exceed 5m"))
	}
	if cfg.HTTP.ShutdownTimeout <= 0 {
		problems = append(problems, errors.New("http.shutdown_timeout must be positive"))
	} else if cfg.HTTP.ShutdownTimeout > 5*time.Minute {
		problems = append(problems, errors.New("http.shutdown_timeout must not exceed 5m"))
	}
	if cfg.HTTP.MaxHeaderBytes < 1024 || cfg.HTTP.MaxHeaderBytes > 64*1024 {
		problems = append(problems, errors.New("http.max_header_bytes must be between 1024 and 65536"))
	}
	switch cfg.Log.Level {
	case "debug", "info", "warn", "error":
	default:
		problems = append(problems, errors.New("log.level must be debug, info, warn, or error"))
	}
	for name, ref := range cfg.SecretReferences {
		if !namePattern.MatchString(name) {
			problems = append(problems, fmt.Errorf("secret reference name %q is invalid", name))
		}
		if !providerPattern.MatchString(ref.Provider) {
			problems = append(problems, fmt.Errorf("secret reference %q has invalid provider", name))
		}
		if ref.Reference == "" || len(ref.Reference) > 2048 || strings.IndexFunc(ref.Reference, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
			problems = append(problems, fmt.Errorf("secret reference %q has invalid opaque reference", name))
		}
	}
	return errors.Join(problems...)
}

func validateAddress(field, address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port == "" {
		return fmt.Errorf("%s must be a host:port address", field)
	}
	if host == "" {
		return fmt.Errorf("%s must include an explicit host", field)
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNumber == 0 {
		return fmt.Errorf("%s must include a numeric port from 1 to 65535", field)
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return errors.New("multiple JSON values")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}
