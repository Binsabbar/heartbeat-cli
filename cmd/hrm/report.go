package main

import (
	"fmt"
	"time"

	"github.com/binsabbar/heartrate-monitor/internal/report"
	"github.com/spf13/cobra"
)

func newReportCmd(g *globals) *cobra.Command {
	var date string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Summarise a day's heart rate, stress, and tags",
		RunE: func(_ *cobra.Command, _ []string) error {
			day := today()
			if date != "" {
				d, err := time.ParseInLocation("2006-01-02", date, time.Local)
				if err != nil {
					return fmt.Errorf("invalid --date (want YYYY-MM-DD): %w", err)
				}
				day = d
			}
			dir, err := g.resolveDataDir()
			if err != nil {
				return err
			}
			st, err := openStore(dir)
			if err != nil {
				return err
			}
			rep, err := report.Build(st, day)
			if err != nil {
				return err
			}
			fmt.Print(rep.String())
			return nil
		},
	}
	cmd.Flags().StringVar(&date, "date", "", "day to report (YYYY-MM-DD, default today)")
	return cmd
}
