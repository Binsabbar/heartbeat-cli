package main

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/binsabbar/heartrate-monitor/internal/ble"
	"github.com/binsabbar/heartrate-monitor/internal/config"
	"github.com/spf13/cobra"
)

func newCalibrateCmd(g *globals) *cobra.Command {
	var dur time.Duration
	cmd := &cobra.Command{
		Use:   "calibrate",
		Short: "Estimate your resting heart rate by sitting still, then save it",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := g.resolveDataDir()
			if err != nil {
				return err
			}
			cfg, err := config.Load(dir)
			if err != nil {
				return err
			}
			log := fileLogger(dir)

			ctx, cancel := context.WithTimeout(cmd.Context(), dur)
			defer cancel()

			match := ble.Match{ID: cfg.DeviceID, Name: cfg.DeviceNameMatch}

			readings := make(chan ble.Reading, 32)
			go func() {
				_ = ble.Monitor(ctx, match, readings, log)
				close(readings)
			}()

			fmt.Printf("Sit still and breathe normally for %s …\n", dur)
			var bpms []int
			for r := range readings {
				bpms = append(bpms, r.BPM)
				fmt.Printf("\r  %d samples, latest %d bpm   ", len(bpms), r.BPM)
			}
			fmt.Println()

			if len(bpms) < 10 {
				return fmt.Errorf("not enough readings (%d) to calibrate; check the connection", len(bpms))
			}
			rhr := percentile(bpms, 10) // robust low estimate, ignores the odd dip
			cfg.RestingHR = rhr
			if cfg.MaxHR <= rhr {
				cfg.MaxHR = config.Default().MaxHR
			}
			if err := config.Save(dir, cfg); err != nil {
				return err
			}
			fmt.Printf("Resting heart rate set to %d bpm (from %d readings).\n", rhr, len(bpms))
			return nil
		},
	}
	cmd.Flags().DurationVar(&dur, "duration", 2*time.Minute, "how long to sample")
	return cmd
}

// percentile returns the p-th percentile (0..100) of vals using nearest-rank.
func percentile(vals []int, p int) int {
	s := append([]int(nil), vals...)
	sort.Ints(s)
	rank := p * (len(s) - 1) / 100
	return s[rank]
}
