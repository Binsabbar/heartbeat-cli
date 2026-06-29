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
	// EventTag opens a manual tag/interval (e.g. a meeting). It carries an ID so a
	// later EventTagEnd can close it; a tag that is never closed is a point marker.
	EventTag EventType = "tag"
	// EventTagEnd closes a previously opened tag, matched by ID.
	EventTagEnd EventType = "tag_end"
	// EventStressChange is emitted automatically when the stress zone transitions.
	EventStressChange EventType = "stress_change"
)

// Event is a timeline annotation: a manual tag (open or close) or a detected
// stress-zone change. Events are appended to events.jsonl.
type Event struct {
	Time time.Time `json:"ts"`
	Type EventType `json:"type"`
	// ID links an EventTag to its EventTagEnd. Empty for stress changes.
	ID string `json:"id,omitempty"`
	// Tag fields (EventTag):
	Kind   string `json:"kind,omitempty"`   // meeting | focus | break | interrupt | …
	Label  string `json:"label,omitempty"`  // the title
	Person string `json:"person,omitempty"` // optional attendee(s)
	Note   string `json:"note,omitempty"`   // optional free note (e.g. JIRA-123)
	// Stress-change fields (EventStressChange):
	From Zone `json:"from,omitempty"`
	To   Zone `json:"to,omitempty"`
}
