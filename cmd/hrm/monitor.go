package main

import (
	"context"
	"fmt"
	"log/slog"
	"os/signal"
	"syscall"
	"time"

	"github.com/binsabbar/heartrate-monitor/internal/ble"
	"github.com/binsabbar/heartrate-monitor/internal/config"
	"github.com/binsabbar/heartrate-monitor/internal/model"
	"github.com/binsabbar/heartrate-monitor/internal/store"
	"github.com/binsabbar/heartrate-monitor/internal/stress"
	"github.com/binsabbar/heartrate-monitor/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func newMonitorCmd(g *globals) *cobra.Command {
	var printOnly bool
	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "Connect to the strap and show the live heart-rate & stress dashboard",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := g.resolveDataDir()
			if err != nil {
				return err
			}
			cfg, err := g.loadConfig(dir)
			if err != nil {
				return err
			}
			st, err := openStore(dir)
			if err != nil {
				return err
			}
			log := fileLogger(dir)

			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			frames := startPipeline(ctx, cfg, st, log)

			if printOnly {
				return runPrint(ctx, frames)
			}
			prog := tea.NewProgram(tui.New(frames, st), tea.WithContext(ctx), tea.WithAltScreen())
			_, err = prog.Run()
			return err
		},
	}
	cmd.Flags().BoolVar(&printOnly, "print", false, "stream readings to stdout instead of the TUI (debug)")
	return cmd
}

// startPipeline wires BLE → stress → store and returns a channel of UI frames.
// It owns two goroutines whose lifetime is bound to ctx.
func startPipeline(ctx context.Context, cfg config.Config, st *store.Store, log *slog.Logger) <-chan tui.Frame {
	readings := make(chan ble.Reading, 32)
	frames := make(chan tui.Frame, 32)

	match := ble.Match{ID: cfg.DeviceID, Name: cfg.DeviceName}
	if match.ID == "" && match.Name == "" {
		match.Name = "WHOOP"
	}

	go func() {
		if err := ble.Monitor(ctx, match, readings, log); err != nil {
			log.Error("ble monitor stopped", "err", err)
		}
		close(readings)
	}()

	go func() {
		defer close(frames)
		engine := stress.NewEngine(cfg.RestingHR, cfg.MaxHR, cfg.Stress)
		for {
			select {
			case <-ctx.Done():
				return
			case r, ok := <-readings:
				if !ok {
					return
				}
				now := time.Now()
				res := engine.Update(now, r.BPM)
				sample := model.Sample{
					Time:   now,
					BPM:    r.BPM,
					RR:     r.RR,
					Stress: res.Score,
					Zone:   res.Zone,
				}
				if err := st.AppendSample(sample); err != nil {
					log.Warn("append sample", "err", err)
				}
				if res.Change != nil {
					if err := st.AppendEvent(*res.Change); err != nil {
						log.Warn("append stress change", "err", err)
					}
				}
				select {
				case frames <- tui.Frame{Sample: sample, Change: res.Change}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return frames
}

// runPrint consumes frames and prints them to stdout (used by --print).
func runPrint(ctx context.Context, frames <-chan tui.Frame) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case f, ok := <-frames:
			if !ok {
				return nil
			}
			line := fmt.Sprintf("%s  %3d bpm  stress %5.1f  %-8s",
				f.Sample.Time.Format("15:04:05"), f.Sample.BPM, f.Sample.Stress, f.Sample.Zone)
			if f.Change != nil {
				line += fmt.Sprintf("  ← %s→%s", f.Change.From, f.Change.To)
			}
			fmt.Println(line)
		}
	}
}
