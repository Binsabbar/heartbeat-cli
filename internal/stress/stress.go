// Package stress implements the HR-relative + trend stress model: it turns a
// stream of heart-rate samples into a 0..100 stress score, classifies it into a
// zone, and emits debounced stress-change events on zone transitions.
//
// The model is intentionally simple and personalised:
//
//	pHRR  = clamp((ewma - restingHR) / (maxHR - restingHR), 0, 1)   // heart-rate reserve
//	slope = max(0, d(ewma)/dt) normalised by SlopeFullBPMPerSec      // acute rise
//	score = clamp(100 * (HRRWeight*pHRR + SlopeWeight*slope), 0, 100)
//
// ewma is an exponentially weighted moving average of raw BPM to suppress sensor
// jitter. A new zone must persist for MinDwell before a change event fires, which
// debounces flapping at zone boundaries.
package stress

import (
	"time"

	"github.com/binsabbar/heartbeat-cli/internal/config"
	"github.com/binsabbar/heartbeat-cli/internal/model"
)

// Engine is a stateful stress calculator. It is not safe for concurrent use; the
// monitor pipeline drives it from a single goroutine.
type Engine struct {
	tuning    config.StressTuning
	restingHR float64
	maxHR     float64

	ewma     float64
	haveEWMA bool

	lastTime time.Time
	lastEWMA float64
	haveLast bool

	curZone   model.Zone
	haveZone  bool
	candidate model.Zone
	candSince time.Time
}

// NewEngine constructs an Engine from the resting/max heart rate and tuning.
func NewEngine(restingHR, maxHR int, tuning config.StressTuning) *Engine {
	return &Engine{
		tuning:    tuning,
		restingHR: float64(restingHR),
		maxHR:     float64(maxHR),
	}
}

// Result is the outcome of processing one reading.
type Result struct {
	Score float64
	Zone  model.Zone
	// Change is non-nil when this reading committed a zone transition.
	Change *model.Event
}

// Update processes a single heart-rate reading taken at time t and returns the
// derived score, zone, and any stress-change event.
func (e *Engine) Update(t time.Time, bpm int) Result {
	raw := float64(bpm)
	if !e.haveEWMA {
		e.ewma = raw
		e.haveEWMA = true
	} else {
		a := e.tuning.SmoothingAlpha
		e.ewma = a*raw + (1-a)*e.ewma
	}

	pHRR := clamp((e.ewma-e.restingHR)/(e.maxHR-e.restingHR), 0, 1)

	var slopeTerm float64
	if e.haveLast {
		if dt := t.Sub(e.lastTime).Seconds(); dt > 0 {
			slope := (e.ewma - e.lastEWMA) / dt // bpm per second
			if slope > 0 && e.tuning.SlopeFullBPMPerSec > 0 {
				slopeTerm = clamp(slope/e.tuning.SlopeFullBPMPerSec, 0, 1)
			}
		}
	}
	e.lastTime, e.lastEWMA, e.haveLast = t, e.ewma, true

	score := clamp(100*(e.tuning.HRRWeight*pHRR+e.tuning.SlopeWeight*slopeTerm), 0, 100)
	zone := e.classify(score)

	res := Result{Score: score, Zone: zone}

	// First-ever reading establishes the baseline zone without an event.
	if !e.haveZone {
		e.curZone, e.haveZone = zone, true
		e.candidate, e.candSince = zone, t
		return res
	}

	// Debounce: a differing zone must persist for MinDwell before it commits.
	switch {
	case zone == e.curZone:
		e.candidate, e.candSince = zone, t
	case zone != e.candidate:
		e.candidate, e.candSince = zone, t
	default:
		if t.Sub(e.candSince) >= e.tuning.MinDwell {
			from := e.curZone
			e.curZone = zone
			res.Zone = zone
			res.Change = &model.Event{
				Time: t,
				Type: model.EventStressChange,
				From: from,
				To:   zone,
			}
		}
	}

	// The committed zone is authoritative for the returned Result while a
	// candidate is still settling, so persisted samples reflect the stable zone.
	res.Zone = e.curZone
	if res.Change != nil {
		res.Zone = res.Change.To
	}
	return res
}

func (e *Engine) classify(score float64) model.Zone {
	switch {
	case score <= e.tuning.CalmMax:
		return model.ZoneCalm
	case score <= e.tuning.MildMax:
		return model.ZoneMild
	case score <= e.tuning.ElevatedMax:
		return model.ZoneElevated
	default:
		return model.ZoneHigh
	}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
