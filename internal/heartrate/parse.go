// Package heartrate decodes the standard BLE Heart Rate Measurement
// characteristic (0x2A37). It is deliberately free of any Bluetooth/cgo
// dependency so the parsing logic can be unit-tested (including under -race)
// without linking the platform Bluetooth stack.
package heartrate

import (
	"errors"
	"fmt"
)

// Reading is a single decoded Heart Rate Measurement.
type Reading struct {
	BPM int   // beats per minute
	RR  []int // RR-intervals in milliseconds (empty if not broadcast)
}

// ErrShortPayload indicates the characteristic value was too short to decode.
var ErrShortPayload = errors.New("heartrate: payload too short")

// Parse decodes a Heart Rate Measurement (0x2A37) value.
//
// Layout: byte 0 is flags:
//
//	bit0    HR value format: 0 => uint8 follows, 1 => uint16 (little-endian) follows
//	bit3    Energy Expended present (uint16) — skipped
//	bit4    RR-Interval(s) present (uint16 each, units of 1/1024 s)
func Parse(data []byte) (Reading, error) {
	if len(data) < 2 {
		return Reading{}, ErrShortPayload
	}
	flags := data[0]
	i := 1

	var bpm int
	if flags&0x01 != 0 { // 16-bit HR
		if len(data) < i+2 {
			return Reading{}, ErrShortPayload
		}
		bpm = int(data[i]) | int(data[i+1])<<8
		i += 2
	} else { // 8-bit HR
		bpm = int(data[i])
		i++
	}

	if flags&0x08 != 0 { // energy expended present (uint16) — skip
		i += 2
		if len(data) < i {
			return Reading{}, ErrShortPayload
		}
	}

	var rr []int
	if flags&0x10 != 0 { // RR-intervals present
		for ; i+2 <= len(data); i += 2 {
			raw := int(data[i]) | int(data[i+1])<<8 // units of 1/1024 s
			rr = append(rr, raw*1000/1024)
		}
	}

	if bpm <= 0 {
		return Reading{}, fmt.Errorf("heartrate: implausible value %d", bpm)
	}
	return Reading{BPM: bpm, RR: rr}, nil
}
