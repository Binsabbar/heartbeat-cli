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
	Tags        []model.Event
	Sessions    []model.Event
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

	for _, ev := range events {
		switch ev.Type {
		case model.EventStressChange:
			rep.Changes = append(rep.Changes, ev)
		case model.EventTag:
			rep.Tags = append(rep.Tags, ev)
		case model.EventSessionStart, model.EventSessionEnd:
			rep.Sessions = append(rep.Sessions, ev)
		}
	}
	return rep, nil
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

	tags := append([]model.Event(nil), r.Tags...)
	tags = append(tags, r.Sessions...)
	if len(tags) > 0 {
		sort.Slice(tags, func(i, j int) bool { return tags[i].Time.Before(tags[j].Time) })
		b.WriteString("\nTags & sessions:\n")
		for _, ev := range tags {
			label := ev.Label
			if ev.Note != "" {
				label += " — " + ev.Note
			}
			fmt.Fprintf(&b, "  %s  [%s] %s\n", ev.Time.Format("15:04:05"), ev.Type, label)
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
