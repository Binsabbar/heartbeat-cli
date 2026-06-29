package insights_test

import (
	"testing"
	"time"

	"github.com/binsabbar/heartbeat-cli/internal/insights"
	"github.com/binsabbar/heartbeat-cli/internal/model"
	"github.com/stretchr/testify/suite"
)

type fakeSource struct {
	events  []model.Event
	samples []model.Sample
}

func (f fakeSource) ReadAllEvents() ([]model.Event, error) { return f.events, nil }

func (f fakeSource) ReadSamples(t time.Time) ([]model.Sample, error) {
	y, m, d := t.Date()
	var out []model.Sample
	for _, s := range f.samples {
		sy, sm, sd := s.Time.Date()
		if sy == y && sm == m && sd == d {
			out = append(out, s)
		}
	}
	return out, nil
}

type OffendersSuite struct {
	suite.Suite
	day time.Time
	src fakeSource
}

func TestOffendersSuite(t *testing.T) { suite.Run(t, new(OffendersSuite)) }

func (s *OffendersSuite) SetupTest() {
	s.day = time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	at := func(h, m int) time.Time { return s.day.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute) }
	s.src = fakeSource{
		samples: []model.Sample{
			{Time: at(8, 0), BPM: 60, Stress: 10},
			{Time: at(9, 10), BPM: 110, Stress: 70}, // in Sarah's meeting
			{Time: at(9, 20), BPM: 120, Stress: 80}, // in Sarah's meeting
			{Time: at(10, 10), BPM: 70, Stress: 20}, // in Omar's meeting
			{Time: at(11, 0), BPM: 60, Stress: 10},
		},
		events: []model.Event{
			{Time: at(9, 0), Type: model.EventTag, ID: "m1", Kind: "meeting", Label: "1:1", Person: "Sarah"},
			{Time: at(9, 30), Type: model.EventTagEnd, ID: "m1"},
			{Time: at(10, 0), Type: model.EventTag, ID: "m2", Kind: "meeting", Label: "standup", Person: "Omar"},
			{Time: at(10, 30), Type: model.EventTagEnd, ID: "m2"},
		},
	}
}

func (s *OffendersSuite) TestRankedByStress() {
	stats, err := insights.Offenders(s.src, insights.Options{Kind: "meeting", By: "stress"})
	s.Require().NoError(err)
	s.Require().Len(stats, 2)

	s.Equal("Sarah", stats[0].Person)
	s.InDelta(75.0, stats[0].AvgStress, 0.01)
	s.InDelta(80.0, stats[0].PeakStress, 0.01)
	s.Equal(1, stats[0].Meetings)
	s.Equal(30*time.Minute, stats[0].TotalDuration)
	// day avg stress = (10+70+80+20+10)/5 = 38; Sarah's meeting avg 75 -> delta 37.
	s.InDelta(37.0, stats[0].AvgDelta, 0.01)

	s.Equal("Omar", stats[1].Person)
	s.InDelta(20.0, stats[1].AvgStress, 0.01)
}

func (s *OffendersSuite) TestMinMeetingsFilter() {
	stats, err := insights.Offenders(s.src, insights.Options{Kind: "meeting", MinMeetings: 2})
	s.Require().NoError(err)
	s.Empty(stats)
}

func (s *OffendersSuite) TestOpenAndPersonlessSkipped() {
	src := s.src
	// An open meeting (no end) and a personless meeting must not produce stats.
	src.events = append(src.events,
		model.Event{Time: s.day.Add(12 * time.Hour), Type: model.EventTag, ID: "open", Kind: "meeting", Label: "x", Person: "Zoe"},
		model.Event{Time: s.day.Add(13 * time.Hour), Type: model.EventTag, ID: "np", Kind: "meeting", Label: "noone"},
		model.Event{Time: s.day.Add(14 * time.Hour), Type: model.EventTagEnd, ID: "np"},
	)
	stats, err := insights.Offenders(src, insights.Options{Kind: "meeting"})
	s.Require().NoError(err)
	for _, st := range stats {
		s.NotEqual("Zoe", st.Person) // open interval skipped
	}
}

func (s *OffendersSuite) TestMultiPersonAttributedToEach() {
	src := s.src
	at := func(h, m int) time.Time { return s.day.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute) }
	src.samples = append(src.samples, model.Sample{Time: at(13, 10), BPM: 100, Stress: 50})
	src.events = append(src.events,
		model.Event{Time: at(13, 0), Type: model.EventTag, ID: "g", Kind: "meeting", Label: "group", Person: "Sarah, Omar"},
		model.Event{Time: at(13, 30), Type: model.EventTagEnd, ID: "g"},
	)
	stats, err := insights.Offenders(src, insights.Options{Kind: "meeting"})
	s.Require().NoError(err)
	byName := map[string]insights.PersonStat{}
	for _, st := range stats {
		byName[st.Person] = st
	}
	s.Equal(2, byName["Sarah"].Meetings) // original + group
	s.Equal(2, byName["Omar"].Meetings)
}
