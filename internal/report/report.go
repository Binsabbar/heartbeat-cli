// Package report summarises a day's recorded samples and events: average/peak
// heart rate, time spent in each stress zone, detected stress changes, and the
// manual tags/sessions aligned on the timeline. This is the foundation for later
// correlating stress with meetings and Jira activity.
package report

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/binsabbar/heartrate-monitor/internal/model"
)

// SampleReader is the subset of store.Store the report needs.
type SampleReader interface {
	ReadSamples(t time.Time) ([]model.Sample, error)
	ReadEvents(t time.Time) ([]model.Event, error)
}

// Tagged is a manual tag resolved into an interval (open tags have no End).
type Tagged struct {
	ID     string
	Kind   string
	Title  string
	Person string
	Note   string
	Start  time.Time
	End    *time.Time
}

// Duration returns the interval length and whether it is closed.
func (t Tagged) Duration() (time.Duration, bool) {
	if t.End == nil {
		return 0, false
	}
	return t.End.Sub(t.Start), true
}

// Report is the computed summary for a single day.
type Report struct {
	Date        time.Time
	SampleCount int
	AvgBPM      float64
	MinBPM      int
	MaxBPM      int
	AvgStress   float64
	MaxStress   float64
	TimeInZone  map[model.Zone]time.Duration
	Changes     []model.Event
	Tags        []Tagged
}

// Build loads and summarises the given day.
func Build(r SampleReader, day time.Time) (Report, error) {
	samples, err := r.ReadSamples(day)
	if err != nil {
		return Report{}, fmt.Errorf("read samples: %w", err)
	}
	events, err := r.ReadEvents(day)
	if err != nil {
		return Report{}, fmt.Errorf("read events: %w", err)
	}

	rep := Report{Date: day, TimeInZone: map[model.Zone]time.Duration{}}
	rep.SampleCount = len(samples)

	if len(samples) > 0 {
		rep.MinBPM = samples[0].BPM
		var sumBPM, sumStress float64
		for i, s := range samples {
			sumBPM += float64(s.BPM)
			sumStress += s.Stress
			if s.BPM < rep.MinBPM {
				rep.MinBPM = s.BPM
			}
			if s.BPM > rep.MaxBPM {
				rep.MaxBPM = s.BPM
			}
			if s.Stress > rep.MaxStress {
				rep.MaxStress = s.Stress
			}
			// Attribute the gap until the next sample to this sample's zone.
			if i+1 < len(samples) {
				if dt := samples[i+1].Time.Sub(s.Time); dt > 0 && dt < 5*time.Minute {
					rep.TimeInZone[s.Zone] += dt
				}
			}
		}
		rep.AvgBPM = sumBPM / float64(len(samples))
		rep.AvgStress = sumStress / float64(len(samples))
	}

	rep.Tags = PairTags(events)
	for _, ev := range events {
		if ev.Type == model.EventStressChange {
			rep.Changes = append(rep.Changes, ev)
		}
	}
	return rep, nil
}

// PairTags resolves open/close tag events into intervals, matched by ID. Tags
// without a matching end remain open (End == nil).
func PairTags(events []model.Event) []Tagged {
	var tags []Tagged
	idx := map[string]int{}
	for _, ev := range events {
		switch ev.Type {
		case model.EventTag:
			idx[ev.ID] = len(tags)
			tags = append(tags, Tagged{
				ID: ev.ID, Kind: ev.Kind, Title: ev.Label,
				Person: ev.Person, Note: ev.Note, Start: ev.Time,
			})
		case model.EventTagEnd:
			if i, ok := idx[ev.ID]; ok {
				end := ev.Time
				tags[i].End = &end
			}
		}
	}
	return tags
}

// String renders the report as a plain-text terminal summary.
func (r Report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Heart Rate & Stress — %s\n", r.Date.Format("Mon 2006-01-02"))
	fmt.Fprintf(&b, "%s\n", strings.Repeat("─", 44))
	if r.SampleCount == 0 {
		b.WriteString("No samples recorded for this day.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "Samples:    %d\n", r.SampleCount)
	fmt.Fprintf(&b, "Heart rate: avg %.0f  min %d  max %d bpm\n", r.AvgBPM, r.MinBPM, r.MaxBPM)
	fmt.Fprintf(&b, "Stress:     avg %.0f  peak %.0f / 100\n\n", r.AvgStress, r.MaxStress)

	b.WriteString("Time in zone:\n")
	for _, z := range []model.Zone{model.ZoneCalm, model.ZoneMild, model.ZoneElevated, model.ZoneHigh} {
		fmt.Fprintf(&b, "  %-9s %s\n", z, fmtDur(r.TimeInZone[z]))
	}

	if len(r.Changes) > 0 {
		b.WriteString("\nStress changes:\n")
		for _, ev := range r.Changes {
			fmt.Fprintf(&b, "  %s  %s → %s\n", ev.Time.Format("15:04:05"), ev.From, ev.To)
		}
	}

	if len(r.Tags) > 0 {
		tags := append([]Tagged(nil), r.Tags...)
		sort.Slice(tags, func(i, j int) bool { return tags[i].Start.Before(tags[j].Start) })
		b.WriteString("\nTags & meetings:\n")
		for _, t := range tags {
			when := t.Start.Format("15:04:05")
			if d, ok := t.Duration(); ok {
				when += fmt.Sprintf("–%s (%s)", t.End.Format("15:04:05"), fmtDur(d))
			} else {
				when += " (open)"
			}
			kind := t.Kind
			if kind == "" {
				kind = "tag"
			}
			line := fmt.Sprintf("  %s  [%s] %s", when, kind, t.Title)
			if t.Person != "" {
				line += " · " + t.Person
			}
			if t.Note != "" {
				line += " · " + t.Note
			}
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

func fmtDur(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	if h > 0 {
		return fmt.Sprintf("%dh %02dm %02ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
