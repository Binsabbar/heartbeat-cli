// Package config resolves the application data directory and loads/saves the
// user configuration (heart-rate baselines, saved BLE device, stress tuning).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// EnvDataDir overrides the default data directory when set.
const EnvDataDir = "HRM_DATA_DIR"

// StressTuning holds the tunable parameters of the HR-relative + trend stress
// model. Durations marshal as nanoseconds in JSON.
type StressTuning struct {
	// SmoothingAlpha is the EWMA factor applied to raw BPM (0<alpha<=1); higher
	// reacts faster, lower is smoother.
	SmoothingAlpha float64 `json:"smoothingAlpha"`
	// HRRWeight and SlopeWeight weight the heart-rate-reserve and trend terms.
	// They should sum to roughly 1.
	HRRWeight   float64 `json:"hrrWeight"`
	SlopeWeight float64 `json:"slopeWeight"`
	// SlopeFullBPMPerSec is the smoothed-BPM rise rate (bpm/sec) that maps the
	// trend term to its full contribution.
	SlopeFullBPMPerSec float64 `json:"slopeFullBpmPerSec"`
	// Zone upper bounds on the 0..100 stress score. Anything above ElevatedMax
	// is High.
	CalmMax     float64 `json:"calmMax"`
	MildMax     float64 `json:"mildMax"`
	ElevatedMax float64 `json:"elevatedMax"`
	// MinDwell is how long a new candidate zone must persist before a
	// stress-change event is committed (debounce against flapping).
	MinDwell time.Duration `json:"minDwell"`
}

// Config is the persisted user configuration.
type Config struct {
	RestingHR  int    `json:"restingHR"`
	MaxHR      int    `json:"maxHR"`
	DeviceID   string `json:"deviceID,omitempty"`
	DeviceName string `json:"deviceName,omitempty"`
	// DeviceNameMatch optionally restricts auto-connect to peripherals whose
	// advertised name contains this substring (case-insensitive). Empty means
	// connect to the first device advertising the standard Heart Rate service.
	DeviceNameMatch string       `json:"deviceNameMatch,omitempty"`
	Stress          StressTuning `json:"stress"`
}

// Default returns a Config populated with sensible defaults.
func Default() Config {
	return Config{
		RestingHR: 60,
		MaxHR:     190,
		Stress: StressTuning{
			SmoothingAlpha:     0.3,
			HRRWeight:          0.8,
			SlopeWeight:        0.2,
			SlopeFullBPMPerSec: 3.0,
			CalmMax:            25,
			MildMax:            50,
			ElevatedMax:        75,
			MinDwell:           15 * time.Second,
		},
	}
}

// Validate fails fast on nonsensical configuration.
func (c Config) Validate() error {
	if c.RestingHR <= 0 || c.RestingHR > 120 {
		return fmt.Errorf("restingHR out of range: %d", c.RestingHR)
	}
	if c.MaxHR <= c.RestingHR {
		return fmt.Errorf("maxHR (%d) must exceed restingHR (%d)", c.MaxHR, c.RestingHR)
	}
	return nil
}

// DataDir returns the resolved data directory, honouring HRM_DATA_DIR and
// falling back to ~/.heartrate-monitor.
func DataDir() (string, error) {
	if d := os.Getenv(EnvDataDir); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".heartrate-monitor"), nil
}

// EnsureDataDir creates the data directory (and its samples/ subdir) if needed.
func EnsureDataDir(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, "samples"), 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	return nil
}

func configPath(dir string) string { return filepath.Join(dir, "config.json") }

// Load reads config.json from dir, returning defaults (merged) when the file
// does not yet exist.
func Load(dir string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(configPath(dir))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// Save writes config.json atomically (write to temp, then rename).
func Save(dir string, cfg Config) error {
	if err := EnsureDataDir(dir); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	tmp := configPath(dir) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, configPath(dir)); err != nil {
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}
