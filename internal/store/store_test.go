package store_test

import (
	"testing"
	"time"

	"github.com/binsabbar/heartbeat-cli/internal/model"
	"github.com/binsabbar/heartbeat-cli/internal/store"
	"github.com/stretchr/testify/suite"
)

type StoreSuite struct {
	suite.Suite
	dir string
	st  *store.Store
}

func TestStoreSuite(t *testing.T) { suite.Run(t, new(StoreSuite)) }

func (s *StoreSuite) SetupTest() {
	s.dir = s.T().TempDir()
	st, err := store.New(s.dir)
	s.Require().NoError(err)
	s.st = st
}

func (s *StoreSuite) TestSampleRoundTrip() {
	day := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	want := []model.Sample{
		{Time: day, BPM: 62, Stress: 5.5, Zone: model.ZoneCalm},
		{Time: day.Add(time.Second), BPM: 130, RR: []int{812, 799}, Stress: 70, Zone: model.ZoneElevated},
	}
	for _, sm := range want {
		s.Require().NoError(s.st.AppendSample(sm))
	}

	got, err := s.st.ReadSamples(day)
	s.Require().NoError(err)
	s.Require().Len(got, 2)
	s.Equal(want[0].BPM, got[0].BPM)
	s.Equal(want[1].RR, got[1].RR)
	s.Equal(model.ZoneElevated, got[1].Zone)
	s.True(want[1].Time.Equal(got[1].Time))
}

func (s *StoreSuite) TestSamplesPartitionedByDate() {
	d1 := time.Date(2026, 6, 29, 23, 30, 0, 0, time.UTC)
	d2 := time.Date(2026, 6, 30, 0, 30, 0, 0, time.UTC)
	s.Require().NoError(s.st.AppendSample(model.Sample{Time: d1, BPM: 60}))
	s.Require().NoError(s.st.AppendSample(model.Sample{Time: d2, BPM: 90}))

	got1, err := s.st.ReadSamples(d1)
	s.Require().NoError(err)
	s.Len(got1, 1)
	s.Equal(60, got1[0].BPM)

	got2, err := s.st.ReadSamples(d2)
	s.Require().NoError(err)
	s.Len(got2, 1)
	s.Equal(90, got2[0].BPM)
}

func (s *StoreSuite) TestReadMissingDayIsEmpty() {
	got, err := s.st.ReadSamples(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
	s.Require().NoError(err)
	s.Empty(got)
}

func (s *StoreSuite) TestClearData() {
	day := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	s.Require().NoError(s.st.AppendSample(model.Sample{Time: day, BPM: 70}))
	s.Require().NoError(s.st.AppendEvent(model.Event{Time: day, Type: model.EventTag, Label: "x"}))

	s.Require().NoError(s.st.ClearData())

	samples, err := s.st.ReadSamples(day)
	s.Require().NoError(err)
	s.Empty(samples)
	events, err := s.st.ReadEvents(day)
	s.Require().NoError(err)
	s.Empty(events)

	// The store stays usable: appends after a wipe still work.
	s.Require().NoError(s.st.AppendSample(model.Sample{Time: day, BPM: 80}))
	again, err := s.st.ReadSamples(day)
	s.Require().NoError(err)
	s.Len(again, 1)
}

func (s *StoreSuite) TestEventsFilteredByDate() {
	day := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	other := time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC)
	s.Require().NoError(s.st.AppendEvent(model.Event{Time: day, Type: model.EventTag, Label: "standup"}))
	s.Require().NoError(s.st.AppendEvent(model.Event{Time: other, Type: model.EventTag, Label: "yesterday"}))

	got, err := s.st.ReadEvents(day)
	s.Require().NoError(err)
	s.Require().Len(got, 1)
	s.Equal("standup", got[0].Label)
}
