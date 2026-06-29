package main

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/binsabbar/heartrate-monitor/internal/insights"
	"github.com/spf13/cobra"
)

func newOffendersCmd(g *globals) *cobra.Command {
	var (
		kind        string
		by          string
		minMeetings int
		days        int
		limit       int
	)
	cmd := &cobra.Command{
		Use:     "offenders",
		Aliases: []string{"triggers", "people"},
		Short:   "Rank the people whose meetings raise your heart rate and stress",
		RunE: func(_ *cobra.Command, _ []string) error {
			switch by {
			case "stress", "hr", "delta":
			default:
				return fmt.Errorf("invalid --by %q (want stress, hr, or delta)", by)
			}

			dir, err := g.resolveDataDir()
			if err != nil {
				return err
			}
			st, err := openStore(dir)
			if err != nil {
				return err
			}

			opts := insights.Options{Kind: kind, By: by, MinMeetings: minMeetings}
			if days > 0 {
				since := today().AddDate(0, 0, -(days - 1))
				opts.Since = &since
			}

			stats, err := insights.Offenders(st, opts)
			if err != nil {
				return err
			}
			if len(stats) == 0 {
				fmt.Println("No closed meetings with a named person found yet.")
				fmt.Println("Tag meetings during `hrm monitor` (press t, set kind=meeting and a person, press e to end).")
				return nil
			}

			scope := "all time"
			if days > 0 {
				scope = fmt.Sprintf("last %d days", days)
			}
			fmt.Printf("Top %s offenders by avg %s (%s)\n\n", kind, by, scope)

			w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "#\tPERSON\tMEETINGS\tTIME\tAVG HR\tAVG STRESS\tPEAK\tΔ VS DAY")
			for i, p := range stats {
				if limit > 0 && i >= limit {
					break
				}
				fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%.0f\t%.0f\t%.0f\t%+.0f\n",
					i+1, p.Person, p.Meetings, fmtDuration(p.TotalDuration),
					p.AvgHR, p.AvgStress, p.PeakStress, p.AvgDelta)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "meeting", "tag kind to analyse (e.g. meeting, focus); empty for all")
	cmd.Flags().StringVar(&by, "by", "stress", "ranking metric: stress, hr, or delta")
	cmd.Flags().IntVar(&minMeetings, "min-meetings", 1, "ignore people with fewer attributed meetings")
	cmd.Flags().IntVar(&days, "days", 0, "only consider the last N days (0 = all)")
	cmd.Flags().IntVar(&limit, "limit", 0, "show at most N people (0 = all)")
	return cmd
}

func fmtDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	h := d / time.Hour
	m := (d - h*time.Hour) / time.Minute
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
