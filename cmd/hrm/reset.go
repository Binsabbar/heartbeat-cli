package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newResetCmd(g *globals) *cobra.Command {
	var force, all bool
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Delete recorded heart-rate data (samples and events)",
		RunE: func(_ *cobra.Command, _ []string) error {
			dir, err := g.resolveDataDir()
			if err != nil {
				return err
			}

			target := "all recorded samples and events"
			if all {
				target = "ALL data, including config.json (device pin and HR baselines)"
			}
			if !force {
				fmt.Printf("This permanently deletes %s\nin %s\nContinue? [y/N]: ", target, dir)
				var ans string
				_, _ = fmt.Scanln(&ans)
				if !strings.EqualFold(strings.TrimSpace(ans), "y") {
					fmt.Println("Aborted.")
					return nil
				}
			}

			st, err := openStore(dir)
			if err != nil {
				return err
			}
			if err := st.ClearData(); err != nil {
				return err
			}
			if all {
				if err := os.Remove(filepath.Join(dir, "config.json")); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return fmt.Errorf("remove config: %w", err)
				}
			}
			fmt.Println("Done.")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip the confirmation prompt")
	cmd.Flags().BoolVar(&all, "all", false, "also delete config.json (device pin and baselines)")
	return cmd
}
