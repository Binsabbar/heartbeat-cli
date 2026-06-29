package report_test

import (
	"testing"
	"time"

	"github.com/binsabbar/heartrate-monitor/internal/model"
	"github.com/binsabbar/heartrate-monitor/internal/report"
	"github.com/stretchr/testify/suite"
)

type fakeReader struct {
	samples []model.Sample
	events  []model.Event
}

func (f fakeReader) ReadSamples(time.Time) ([]model.Sample, error) { return f.samples, nil }
func (f fakeReader) ReadEvents(time.Time) ([]model.Event, error)   { return f.events, nil }

type ReportSuite struct {
	suite.Suite
	day time.Time
}

func TestReportSuite(t *testing.T) { suite.Run(t, new(ReportSuite)) }

func (s *ReportSuite) SetupTest() {
	s.day = time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
}

func (s *ReportSuite) TestBuildAggregates() {
	r := fakeReader{
		samples: []model.Sample{
			{Time: s.day, BPM: 60, Stress: 10, Zone: model.ZoneCalm},
			{Time: s.day.Add(10 * time.Second), BPM: 120, Stress: 60, Zone: model.ZoneElevated},
			{Time: s.day.Add(20 * time.Second), BPM: 90, Stress: 30, Zone: model.ZoneMild},
		},
		events: []model.Event{
			{Time: s.day.Add(5 * time.Second), Type: model.EventTag, ID: "a", Kind: "meeting", Label: "standup", Person: "Sarah"},
			{Time: s.day.Add(12 * time.Second), Type: model.EventStressChange, From: model.ZoneCalm, To: model.ZoneElevated},
		},
	}
	rep, err := report.Build(r, s.day)
	s.Require().NoError(err)

	s.Equal(3, rep.SampleCount)
	s.Equal(60, rep.MinBPM)
	s.Equal(120, rep.MaxBPM)
	s.InDelta(90.0, rep.AvgBPM, 0.01)
	s.InDelta(60.0, rep.MaxStress, 0.01)
	s.Require().Len(rep.Tags, 1)
	s.Equal("standup", rep.Tags[0].Title)
	s.Equal("Sarah", rep.Tags[0].Person)
	s.Len(rep.Changes, 1)
	// First two gaps are 10s each, attributed to calm then elevated.
	s.Equal(10*time.Second, rep.TimeInZone[model.ZoneCalm])
	s.Equal(10*time.Second, rep.TimeInZone[model.ZoneElevated])
}

func (s *ReportSuite) TestTagIntervalPairing() {
	r := fakeReader{
		samples: []model.Sample{{Time: s.day, BPM: 70, Zone: model.ZoneCalm}},
		events: []model.Event{
			{Time: s.day, Type: model.EventTag, ID: "m1", Kind: "meeting", Label: "Sprint planning"},
			{Time: s.day.Add(30 * time.Minute), Type: model.EventTagEnd, ID: "m1"},
			{Time: s.day.Add(time.Hour), Type: model.EventTag, ID: "m2", Kind: "focus", Label: "deep work"},
		},
	}
	rep, err := report.Build(r, s.day)
	s.Require().NoError(err)
	s.Require().Len(rep.Tags, 2)

	d, closed := rep.Tags[0].Duration()
	s.True(closed)
	s.Equal(30*time.Minute, d)

	_, openClosed := rep.Tags[1].Duration()
	s.False(openClosed, "unclosed tag stays open")
	s.Contains(rep.String(), "(open)")
}

func (s *ReportSuite) TestBuildEmpty() {
	rep, err := report.Build(fakeReader{}, s.day)
	s.Require().NoError(err)
	s.Equal(0, rep.SampleCount)
	s.Contains(rep.String(), "No samples")
}
