// Package model holds the shared domain types used across the heartrate-monitor
// packages: heart-rate samples, timeline events, and stress zones. It depends on
// nothing else in the project so every other package can import it freely.
package model

import "time"

// Zone is a discrete stress band derived from a stress score in [0,100].
type Zone string

const (
	// ZoneCalm is at or near resting heart rate.
	ZoneCalm Zone = "calm"
	// ZoneMild is a slight elevation above rest.
	ZoneMild Zone = "mild"
	// ZoneElevated is a clearly raised cardiovascular load.
	ZoneElevated Zone = "elevated"
	// ZoneHigh is a strong stress/exertion response.
	ZoneHigh Zone = "high"
)

// Sample is a single heart-rate reading together with the stress values derived
// from it. It is the unit persisted to the daily JSONL sample files.
type Sample struct {
	Time   time.Time `json:"ts"`
	BPM    int       `json:"bpm"`
	RR     []int     `json:"rr,omitempty"` // RR-intervals in milliseconds, only when broadcast.
	Stress float64   `json:"stress"`
	Zone   Zone      `json:"zone"`
}

// EventType enumerates the kinds of timeline annotations.
type EventType string

const (
	// EventTag is a manual moment tag (e.g. "standup", "JIRA-123").
	EventTag EventType = "tag"
	// EventSessionStart marks the beginning of a named monitoring session.
	EventSessionStart EventType = "session_start"
	// EventSessionEnd marks the end of a named monitoring session.
	EventSessionEnd EventType = "session_end"
	// EventStressChange is emitted automatically when the stress zone transitions.
	EventStressChange EventType = "stress_change"
)

// Event is a timeline annotation: a manual tag, a session boundary, or a
// detected stress-zone change. Events are appended to events.jsonl.
type Event struct {
	Time  time.Time `json:"ts"`
	Type  EventType `json:"type"`
	Label string    `json:"label,omitempty"`
	Note  string    `json:"note,omitempty"`
	From  Zone      `json:"from,omitempty"` // populated for stress_change events
	To    Zone      `json:"to,omitempty"`   // populated for stress_change events
}
