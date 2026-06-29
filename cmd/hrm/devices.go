package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/binsabbar/heartrate-monitor/internal/ble"
	"github.com/binsabbar/heartrate-monitor/internal/config"
	"github.com/spf13/cobra"
)

func newDevicesCmd(g *globals) *cobra.Command {
	var (
		dur  time.Duration
		save string
	)
	cmd := &cobra.Command{
		Use:   "devices",
		Short: "Scan for nearby heart-rate devices (and optionally save one)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), dur+2*time.Second)
			defer cancel()

			fmt.Printf("Scanning for %s …\n", dur)
			devices, err := ble.Scan(ctx, dur)
			if err != nil {
				return err
			}
			if len(devices) == 0 {
				fmt.Println("No heart-rate devices found. Is 'Broadcast Heart Rate' enabled on the strap,")
				fmt.Println("and is the terminal granted Bluetooth permission?")
				return nil
			}
			sort.Slice(devices, func(i, j int) bool { return devices[i].RSSI > devices[j].RSSI })
			fmt.Printf("\n%-38s %-20s %s\n", "ID", "NAME", "RSSI")
			for _, d := range devices {
				fmt.Printf("%-38s %-20s %ddBm\n", d.ID, d.Name, d.RSSI)
			}

			if save == "" {
				return nil
			}
			for _, d := range devices {
				if strings.EqualFold(d.ID, save) ||
					strings.Contains(strings.ToUpper(d.Name), strings.ToUpper(save)) {
					return saveDevice(g, d)
				}
			}
			return fmt.Errorf("no scanned device matched %q", save)
		},
	}
	cmd.Flags().DurationVar(&dur, "duration", 6*time.Second, "scan duration")
	cmd.Flags().StringVar(&save, "save", "", "save the device matching this id or name to config")
	return cmd
}

func saveDevice(g *globals, d ble.Device) error {
	dir, err := g.resolveDataDir()
	if err != nil {
		return err
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return err
	}
	cfg.DeviceID = d.ID
	cfg.DeviceName = d.Name
	if err := config.Save(dir, cfg); err != nil {
		return err
	}
	fmt.Printf("Saved device %s (%s) to config.\n", d.Name, d.ID)
	return nil
}
