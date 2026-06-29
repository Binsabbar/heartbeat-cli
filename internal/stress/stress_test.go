package stress_test

import (
	"testing"
	"time"

	"github.com/binsabbar/heartrate-monitor/internal/config"
	"github.com/binsabbar/heartrate-monitor/internal/model"
	"github.com/binsabbar/heartrate-monitor/internal/stress"
	"github.com/stretchr/testify/suite"
)

type StressSuite struct {
	suite.Suite
	base time.Time
}

func TestStressSuite(t *testing.T) { suite.Run(t, new(StressSuite)) }

func (s *StressSuite) SetupTest() {
	s.base = time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
}

// classifyTuning isolates the HRR term (no smoothing, no slope) so a steady BPM
// maps deterministically to score = 100 * pHRR.
func classifyTuning() config.StressTuning {
	return config.StressTuning{
		SmoothingAlpha: 1.0,
		HRRWeight:      1.0,
		SlopeWeight:    0,
		CalmMax:        25,
		MildMax:        50,
		ElevatedMax:    75,
		MinDwell:       15 * time.Second,
	}
}

// feedSteady pushes the same BPM n times at 1s spacing and returns the last Result.
func (s *StressSuite) feedSteady(e *stress.Engine, bpm, n int) stress.Result {
	var r stress.Result
	for i := range n {
		r = e.Update(s.base.Add(time.Duration(i)*time.Second), bpm)
	}
	return r
}

func (s *StressSuite) TestClassifyZones() {
	// resting 60, max 160 -> HRR span 100, so pHRR == (bpm-60)/100.
	cases := []struct {
		name string
		bpm  int
		want model.Zone
	}{
		{"resting is calm", 60, model.ZoneCalm},
		{"just below calm max", 84, model.ZoneCalm}, // score 24
		{"mild", 100, model.ZoneMild},               // score 40
		{"elevated", 125, model.ZoneElevated},       // score 65
		{"high", 150, model.ZoneHigh},               // score 90
		{"clamped at max is high", 200, model.ZoneHigh},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			e := stress.NewEngine(60, 160, classifyTuning())
			r := s.feedSteady(e, tc.bpm, 3)
			s.Equal(tc.want, r.Zone, "score=%.1f", r.Score)
		})
	}
}

func (s *StressSuite) TestScoreBoundsAndMonotonic() {
	e := stress.NewEngine(60, 160, classifyTuning())
	low := s.feedSteady(e, 60, 3)
	s.GreaterOrEqual(low.Score, 0.0)

	e2 := stress.NewEngine(60, 160, classifyTuning())
	high := s.feedSteady(e2, 250, 3)
	s.LessOrEqual(high.Score, 100.0)
	s.Greater(high.Score, low.Score)
}

func (s *StressSuite) TestStressChangeFiresAfterDwell() {
	e := stress.NewEngine(60, 160, classifyTuning())
	// Establish a calm baseline.
	s.feedSteady(e, 60, 3)

	// Jump to a high BPM; the change must only commit after MinDwell (15s).
	var changes []*model.Event
	start := s.base.Add(10 * time.Second)
	for i := 0; i <= 16; i++ {
		r := e.Update(start.Add(time.Duration(i)*time.Second), 150)
		if r.Change != nil {
			changes = append(changes, r.Change)
		}
	}

	s.Require().Len(changes, 1, "exactly one stress-change should commit")
	s.Equal(model.ZoneCalm, changes[0].From)
	s.Equal(model.ZoneHigh, changes[0].To)
	s.Equal(model.EventStressChange, changes[0].Type)
	// The committing sample is 15s after the first high reading at start+0s.
	s.Equal(start.Add(15*time.Second), changes[0].Time)
}

func (s *StressSuite) TestBriefSpikeDoesNotCommit() {
	e := stress.NewEngine(60, 160, classifyTuning())
	s.feedSteady(e, 60, 3)

	start := s.base.Add(10 * time.Second)
	var changes int
	// 10s of high (< MinDwell) then back to calm.
	for i := range 10 {
		if r := e.Update(start.Add(time.Duration(i)*time.Second), 150); r.Change != nil {
			changes++
		}
	}
	for i := 10; i < 20; i++ {
		if r := e.Update(start.Add(time.Duration(i)*time.Second), 60); r.Change != nil {
			changes++
		}
	}
	s.Zero(changes, "a spike shorter than MinDwell must not commit a change")
}

func (s *StressSuite) TestChangeEmittedOnce() {
	e := stress.NewEngine(60, 160, classifyTuning())
	s.feedSteady(e, 60, 3)

	start := s.base.Add(10 * time.Second)
	var changes int
	for i := 0; i <= 40; i++ {
		if r := e.Update(start.Add(time.Duration(i)*time.Second), 150); r.Change != nil {
			changes++
		}
	}
	s.Equal(1, changes, "sustained high should emit the transition only once")
}
