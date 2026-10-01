package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/kaushalacts/vyom/internal/logging"
	"github.com/kaushalacts/vyom/internal/policy"
	"github.com/kaushalacts/vyom/pkg/models"
	"github.com/kaushalacts/vyom/internal/providererr"
)

// Config is Vyom's complete runtime configuration.
//
// Priority: CLI flags > environment variables > config file > defaults.
type Config struct {
	// Database
	Database DatabaseConfig `yaml:"database" json:"database"`

	// Logging
	Logging logging.Config `yaml:"logging" json:"logging"`

	// Server (REST API)
	Server ServerConfig `yaml:"server" json:"server"`

	// Providers
	Providers ProvidersConfig `yaml:"providers" json:"providers"`

	// Default scan window (used when CLI flags are not supplied)
	Scan ScanConfig `yaml:"scan" json:"scan"`

	// Default optimization policy
	Policy PolicyConfig `yaml:"policy" json:"policy"`

	// Pricing (optional, used for cost estimates)
	Pricing models.PricingConfig `yaml:"pricing" json:"pricing"`

	// Optimization
	Optimization OptimizationConfig `yaml:"optimization" json:"optimization"`

	// File path this config was loaded from (populated by Load)
	SourcePath string `yaml:"-" json:"-"`
}

// DatabaseConfig configures the persistence layer.
type DatabaseConfig struct {
	// Path is the SQLite database file. Empty means ":memory:".
	Path string `yaml:"path" json:"path" env:"VYOM_DB_PATH"`
}

// ServerConfig configures the REST API server.
type ServerConfig struct {
	// Host is the interface to bind to.
	Host string `yaml:"host" json:"host" env:"VYOM_SERVER_HOST"`
	// Port is the TCP port to listen on.
	Port int `yaml:"port" json:"port" env:"VYOM_SERVER_PORT"`
	// ReadTimeout bounds the request read.
	ReadTimeout time.Duration `yaml:"read_timeout" json:"read_timeout"`
	// WriteTimeout bounds the response write.
	WriteTimeout time.Duration `yaml:"write_timeout" json:"write_timeout"`
	// IdleTimeout bounds idle keep-alive connections.
	IdleTimeout time.Duration `yaml:"idle_timeout" json:"idle_timeout"`
}

// ProvidersConfig holds the configured CDN and storage providers.
type ProvidersConfig struct {
	CDN     ProviderRef `yaml:"cdn" json:"cdn"`
	Storage ProviderRef `yaml:"storage" json:"storage"`
}

// ProviderRef references a configured provider by its ID.
type ProviderRef struct {
	// ID is the provider configuration ID from the providers table.
	ID string `yaml:"id" json:"id"`
}

// ScanConfig configures the default scan window.
type ScanConfig struct {
	// DaysBack is the number of days before today to start the window.
	DaysBack int `yaml:"days_back" json:"days_back"`
	// Timezone is the IANA timezone for window boundaries (default UTC).
	Timezone string `yaml:"timezone" json:"timezone"`
}

// PolicyConfig names the default optimization policy.
type PolicyConfig struct {
	// Name is the policy name from the policies table.
	Name string `yaml:"name" json:"name" env:"VYOM_POLICY_NAME"`
}

// OptimizationConfig configures optimization behavior.
type OptimizationConfig struct {
	// DryRun defaults to true; --apply overrides.
	DryRun bool `yaml:"dry_run" json:"dry_run" env:"VYOM_DRY_RUN"`
	// Concurrency bounds parallel image encodes.
	Concurrency int `yaml:"concurrency" json:"concurrency" env:"VYOM_OPT_CONCURRENCY"`
	// TempDir is where intermediate files are written.
	TempDir string `yaml:"temp_dir" json:"temp_dir" env:"VYOM_TEMP_DIR"`
}

// Default returns a configuration with sensible defaults.
func Default() Config {
	return Config{
		Database: DatabaseConfig{Path: "vyom.db"},
		Logging: logging.Config{
			Level:     logging.LevelInfo,
			Format:    logging.FormatJSON,
			Service:   "vyom",
			Component: "cli",
		},
		Server: ServerConfig{
			Host:         "127.0.0.1",
			Port:         8080,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		Scan: ScanConfig{
			DaysBack: 30,
			Timezone: "UTC",
		},
		Policy: PolicyConfig{Name: policy.DefaultName},
		Optimization: OptimizationConfig{
			DryRun:      true,
			Concurrency: 2,
			TempDir:     "",
		},
	}
}

// Load reads config from file, then overlays environment variables.
//
// The file is optional: if it doesn't exist and no path is provided, Default()
// is returned. This allows `vyom` to run with zero configuration for demos.
func Load(path string) (Config, error) {
	cfg := Default()
	if path != "" {
		cfg.SourcePath = path
		if err := loadFile(path, &cfg); err != nil {
			return cfg, err
		}
	}
	applyEnv(&cfg)
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func loadFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // file is optional
		}
		return providererr.Wrap(err, "config", "Load", providererr.KindConfiguration)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return providererr.Wrap(err, "config", "Load:unmarshal", providererr.KindMalformedResponse)
	}
	return nil
}

// env tag mapping: struct field -> environment variable name.
// This is a manual mapping because we want explicit control over which
// fields are environment-configurable and what their env names are.
func applyEnv(cfg *Config) {
	// Database
	if v := os.Getenv("VYOM_DB_PATH"); v != "" {
		cfg.Database.Path = v
	}

	// Logging
	if v := os.Getenv("VYOM_LOG_LEVEL"); v != "" {
		if l, err := logging.ParseLevel(v); err == nil {
			cfg.Logging.Level = l
		}
	}
	if v := os.Getenv("VYOM_LOG_FORMAT"); v != "" {
		if f, err := logging.ParseFormat(v); err == nil {
			cfg.Logging.Format = f
		}
	}

	// Server
	if v := os.Getenv("VYOM_SERVER_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("VYOM_SERVER_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Server.Port)
	}

	// Providers (referenced by ID)
	if v := os.Getenv("VYOM_CDN_PROVIDER_ID"); v != "" {
		cfg.Providers.CDN.ID = v
	}
	if v := os.Getenv("VYOM_STORAGE_PROVIDER_ID"); v != "" {
		cfg.Providers.Storage.ID = v
	}

	// Scan
	if v := os.Getenv("VYOM_SCAN_DAYS_BACK"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Scan.DaysBack)
	}
	if v := os.Getenv("VYOM_SCAN_TIMEZONE"); v != "" {
		cfg.Scan.Timezone = v
	}

	// Policy
	if v := os.Getenv("VYOM_POLICY_NAME"); v != "" {
		cfg.Policy.Name = v
	}

	// Optimization
	if v := os.Getenv("VYOM_DRY_RUN"); v != "" {
		cfg.Optimization.DryRun = strings.ToLower(v) != "false" && v != "0"
	}
	if v := os.Getenv("VYOM_OPT_CONCURRENCY"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Optimization.Concurrency)
	}
	if v := os.Getenv("VYOM_TEMP_DIR"); v != "" {
		cfg.Optimization.TempDir = v
	}
}

// Validate checks the configuration for internal consistency.
func (c Config) Validate() error {
	var errs models.ValidationErrors

	if c.Database.Path == "" {
		errs.Addf("database.path", "must not be empty")
	}

	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		errs.Addf("server.port", "must be 1-65535, got %d", c.Server.Port)
	}
	if c.Server.ReadTimeout <= 0 {
		errs.Addf("server.read_timeout", "must be positive")
	}
	if c.Server.WriteTimeout <= 0 {
		errs.Addf("server.write_timeout", "must be positive")
	}
	if c.Server.IdleTimeout <= 0 {
		errs.Addf("server.idle_timeout", "must be positive")
	}

	if _, err := time.LoadLocation(c.Scan.Timezone); err != nil {
		errs.Addf("scan.timezone", "invalid IANA timezone %q: %v", c.Scan.Timezone, err)
	}
	if c.Scan.DaysBack <= 0 {
		errs.Addf("scan.days_back", "must be positive")
	}

	if strings.TrimSpace(c.Policy.Name) == "" {
		errs.Addf("policy.name", "must not be empty")
	}

	if c.Optimization.Concurrency <= 0 {
		errs.Addf("optimization.concurrency", "must be positive")
	}
	if c.Optimization.TempDir != "" {
		if _, err := os.Stat(c.Optimization.TempDir); err != nil {
			errs.Addf("optimization.temp_dir", "directory does not exist: %v", err)
		}
	}

	return errs.Err()
}

// Window returns the scan time window derived from the config.
func (c Config) Window() (models.TimeWindow, error) {
	loc, err := time.LoadLocation(c.Scan.Timezone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -c.Scan.DaysBack)
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	return models.NewTimeWindow(start, end), nil
}

// ExampleConfigPath returns the path to the example configuration file.
func ExampleConfigPath() string {
	return filepath.Join("configs", "vyom.example.yaml")
}

// MustLoad is like Load but panics on error (for tests and early init).
func MustLoad(path string) Config {
	cfg, err := Load(path)
	if err != nil {
		panic(fmt.Sprintf("config: %v", err))
	}
	return cfg
}
