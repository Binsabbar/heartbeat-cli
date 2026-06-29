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
			{Time: s.day.Add(5 * time.Second), Type: model.EventTag, Label: "standup"},
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
	s.Len(rep.Tags, 1)
	s.Len(rep.Changes, 1)
	// First two gaps are 10s each, attributed to calm then elevated.
	s.Equal(10*time.Second, rep.TimeInZone[model.ZoneCalm])
	s.Equal(10*time.Second, rep.TimeInZone[model.ZoneElevated])
}

func (s *ReportSuite) TestBuildEmpty() {
	rep, err := report.Build(fakeReader{}, s.day)
	s.Require().NoError(err)
	s.Equal(0, rep.SampleCount)
	s.Contains(rep.String(), "No samples")
}
