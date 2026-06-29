// Command hrm reads live heart rate from a Whoop strap over Bluetooth LE,
// computes a stress level, displays it in an interactive terminal UI, and stores
// the time-series locally for later correlation with meetings and Jira activity.
package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/binsabbar/heartrate-monitor/internal/config"
	"github.com/binsabbar/heartrate-monitor/internal/store"
	"github.com/spf13/cobra"
)

// globals holds values bound to root persistent flags.
type globals struct {
	dataDir string
	resting int
	maxHR   int
	device  string
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	g := &globals{}
	root := &cobra.Command{
		Use:           "hrm",
		Short:         "Live Whoop heart-rate & stress monitor",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	pf := root.PersistentFlags()
	pf.StringVar(&g.dataDir, "data-dir", "", "data directory (default $HRM_DATA_DIR or ~/.heartrate-monitor)")
	pf.IntVar(&g.resting, "resting-hr", 0, "override resting heart rate (bpm)")
	pf.IntVar(&g.maxHR, "max-hr", 0, "override maximum heart rate (bpm)")
	pf.StringVar(&g.device, "device", "", "BLE device id or name substring to connect to")

	root.AddCommand(newMonitorCmd(g), newDevicesCmd(g), newCalibrateCmd(g), newReportCmd(g))
	return root
}

// resolveDataDir returns the data dir from the flag or default and ensures it exists.
func (g *globals) resolveDataDir() (string, error) {
	dir := g.dataDir
	if dir == "" {
		d, err := config.DataDir()
		if err != nil {
			return "", err
		}
		dir = d
	}
	if err := config.EnsureDataDir(dir); err != nil {
		return "", err
	}
	return dir, nil
}

// loadConfig loads config from dir and applies any flag overrides, validating the result.
func (g *globals) loadConfig(dir string) (config.Config, error) {
	cfg, err := config.Load(dir)
	if err != nil {
		return cfg, err
	}
	if g.resting > 0 {
		cfg.RestingHR = g.resting
	}
	if g.maxHR > 0 {
		cfg.MaxHR = g.maxHR
	}
	if g.device != "" {
		cfg.DeviceID = g.device
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// openStore creates the JSONL store rooted at dir.
func openStore(dir string) (*store.Store, error) { return store.New(dir) }

// fileLogger returns a slog.Logger writing to hrm.log in dir, so the TUI's
// stdout is never polluted. Returns a discard logger on failure.
func fileLogger(dir string) *slog.Logger {
	f, err := os.OpenFile(filepath.Join(dir, "hrm.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// today returns the current local date truncated to midnight.
func today() time.Time {
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location())
}
