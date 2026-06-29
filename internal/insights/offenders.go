// Package insights derives cross-day analytics from recorded data. Its first
// analysis ranks the people whose meetings coincide with the highest heart rate
// and stress — your "top offenders" — to surface what triggers you at work.
package insights

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/binsabbar/heartbeat-cli/internal/model"
	"github.com/binsabbar/heartbeat-cli/internal/report"
)

// Source supplies the recorded data the analysis reads.
type Source interface {
	ReadAllEvents() ([]model.Event, error)
	ReadSamples(t time.Time) ([]model.Sample, error)
}

// Options configures the offenders analysis.
type Options struct {
	Kind        string     // tag kind to include (e.g. "meeting"); empty = any
	By          string     // ranking key: "stress" (default), "hr", or "delta"
	MinMeetings int        // drop people with fewer attributed meetings
	Since       *time.Time // only meetings starting at/after this instant
}

// PersonStat aggregates one person's attributed meetings.
type PersonStat struct {
	Person        string
	Meetings      int
	TotalDuration time.Duration
	AvgHR         float64
	AvgStress     float64
	PeakStress    float64
	// AvgDelta is the mean per-meeting stress elevation vs that day's overall
	// average — how much above your daily norm this person's meetings sit.
	AvgDelta float64
}

type acc struct {
	meetings         int
	totalDur         time.Duration
	sumHR, sumStress float64
	nSamples         int
	peakStress       float64
	sumDelta         float64
}

// Offenders ranks people by how stressful their meetings are. Open meetings,
// meetings with no recorded samples, and tags without a person are skipped.
func Offenders(src Source, opts Options) ([]PersonStat, error) {
	events, err := src.ReadAllEvents()
	if err != nil {
		return nil, fmt.Errorf("read events: %w", err)
	}

	dayCache := map[string][]model.Sample{}
	loadDay := func(t time.Time) ([]model.Sample, error) {
		key := t.Format("2006-01-02")
		if s, ok := dayCache[key]; ok {
			return s, nil
		}
		s, err := src.ReadSamples(t)
		if err != nil {
			return nil, err
		}
		dayCache[key] = s
		return s, nil
	}

	people := map[string]*acc{}
	for _, tag := range report.PairTags(events) {
		if opts.Kind != "" && !strings.EqualFold(tag.Kind, opts.Kind) {
			continue
		}
		if tag.End == nil {
			continue // open interval: no bounded window
		}
		if opts.Since != nil && tag.Start.Before(*opts.Since) {
			continue
		}
		names := splitPersons(tag.Person)
		if len(names) == 0 {
			continue
		}

		window, err := samplesIn(loadDay, tag.Start, *tag.End)
		if err != nil {
			return nil, err
		}
		if len(window) == 0 {
			continue
		}

		var sumHR, sumStress, peak float64
		for _, s := range window {
			sumHR += float64(s.BPM)
			sumStress += s.Stress
			if s.Stress > peak {
				peak = s.Stress
			}
		}
		mAvgStress := sumStress / float64(len(window))

		dayAvg, err := dayAvgStress(loadDay, tag.Start)
		if err != nil {
			return nil, err
		}
		delta := mAvgStress - dayAvg

		for _, name := range names {
			a := people[name]
			if a == nil {
				a = &acc{}
				people[name] = a
			}
			a.meetings++
			a.totalDur += tag.End.Sub(tag.Start)
			a.sumHR += sumHR
			a.sumStress += sumStress
			a.nSamples += len(window)
			a.sumDelta += delta
			if peak > a.peakStress {
				a.peakStress = peak
			}
		}
	}

	stats := make([]PersonStat, 0, len(people))
	for name, a := range people {
		if a.meetings < opts.MinMeetings {
			continue
		}
		stats = append(stats, PersonStat{
			Person:        name,
			Meetings:      a.meetings,
			TotalDuration: a.totalDur,
			AvgHR:         a.sumHR / float64(a.nSamples),
			AvgStress:     a.sumStress / float64(a.nSamples),
			PeakStress:    a.peakStress,
			AvgDelta:      a.sumDelta / float64(a.meetings),
		})
	}
	sortStats(stats, opts.By)
	return stats, nil
}

func sortStats(stats []PersonStat, by string) {
	key := func(p PersonStat) float64 {
		switch by {
		case "hr":
			return p.AvgHR
		case "delta":
			return p.AvgDelta
		default:
			return p.AvgStress
		}
	}
	sort.SliceStable(stats, func(i, j int) bool {
		if ki, kj := key(stats[i]), key(stats[j]); ki != kj {
			return ki > kj // highest first
		}
		return stats[i].Meetings > stats[j].Meetings
	})
}

// samplesIn returns the samples falling within [start,end], spanning day files
// if the interval crosses midnight.
func samplesIn(loadDay func(time.Time) ([]model.Sample, error), start, end time.Time) ([]model.Sample, error) {
	var out []model.Sample
	for d := dayOf(start); !d.After(dayOf(end)); d = d.AddDate(0, 0, 1) {
		day, err := loadDay(d)
		if err != nil {
			return nil, err
		}
		for _, s := range day {
			if !s.Time.Before(start) && !s.Time.After(end) {
				out = append(out, s)
			}
		}
	}
	return out, nil
}

// dayAvgStress is the mean stress across every sample recorded on start's date.
func dayAvgStress(loadDay func(time.Time) ([]model.Sample, error), start time.Time) (float64, error) {
	day, err := loadDay(start)
	if err != nil {
		return 0, err
	}
	if len(day) == 0 {
		return 0, nil
	}
	var sum float64
	for _, s := range day {
		sum += s.Stress
	}
	return sum / float64(len(day)), nil
}

func dayOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// splitPersons normalises a person field into a deduped list of trimmed names.
func splitPersons(field string) []string {
	raw := strings.FieldsFunc(field, func(r rune) bool { return r == ',' || r == ';' })
	seen := map[string]bool{}
	var out []string
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p == "" || seen[strings.ToLower(p)] {
			continue
		}
		seen[strings.ToLower(p)] = true
		out = append(out, p)
	}
	return out
}
