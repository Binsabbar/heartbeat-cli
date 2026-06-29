package heartrate_test

import (
	"testing"

	"github.com/binsabbar/heartbeat-cli/internal/heartrate"
	"github.com/stretchr/testify/suite"
)

type ParseSuite struct {
	suite.Suite
}

func TestParseSuite(t *testing.T) { suite.Run(t, new(ParseSuite)) }

func (s *ParseSuite) TestParseHeartRate() {
	cases := []struct {
		name    string
		data    []byte
		wantBPM int
		wantRR  []int
		wantErr bool
	}{
		{
			name:    "8-bit hr no rr",
			data:    []byte{0x00, 72},
			wantBPM: 72,
		},
		{
			name:    "16-bit hr",
			data:    []byte{0x01, 0x2C, 0x01}, // flags bit0 set, 0x012C = 300
			wantBPM: 300,
		},
		{
			name:    "8-bit hr with one rr interval",
			data:    []byte{0x10, 80, 0x00, 0x04}, // rr raw 1024 -> 1000ms
			wantBPM: 80,
			wantRR:  []int{1000},
		},
		{
			name:    "energy expended skipped before rr",
			data:    []byte{0x18, 90, 0xFF, 0xFF, 0x00, 0x04}, // bit3 energy + bit4 rr
			wantBPM: 90,
			wantRR:  []int{1000},
		},
		{
			name:    "two rr intervals",
			data:    []byte{0x10, 100, 0x00, 0x04, 0x00, 0x02}, // 1024->1000, 512->500
			wantBPM: 100,
			wantRR:  []int{1000, 500},
		},
		{
			name:    "empty payload errors",
			data:    []byte{},
			wantErr: true,
		},
		{
			name:    "flags only errors",
			data:    []byte{0x00},
			wantErr: true,
		},
		{
			name:    "16-bit flag but truncated errors",
			data:    []byte{0x01, 0x2C},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			got, err := heartrate.Parse(tc.data)
			if tc.wantErr {
				s.Error(err)
				return
			}
			s.Require().NoError(err)
			s.Equal(tc.wantBPM, got.BPM)
			s.Equal(tc.wantRR, got.RR)
		})
	}
}
