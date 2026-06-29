// Package store persists heart-rate samples and timeline events as JSON Lines
// (NDJSON). Samples are partitioned into one file per local date under samples/;
// events share a single events.jsonl. Append-only NDJSON is crash-resilient and
// streamable, unlike a single growing JSON array.
package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/binsabbar/heartrate-monitor/internal/model"
)

// Store appends and reads samples/events under a data directory. It is safe for
// concurrent use; appends from the monitor pipeline and the TUI may interleave.
type Store struct {
	dir string
	mu  sync.Mutex
}

// New returns a Store rooted at dir, creating the directory layout if needed.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "samples"), 0o755); err != nil {
		return nil, fmt.Errorf("create store dirs: %w", err)
	}
	return &Store{dir: dir}, nil
}

func (s *Store) samplePath(t time.Time) string {
	return filepath.Join(s.dir, "samples", t.Format("2006-01-02")+".jsonl")
}

func (s *Store) eventsPath() string { return filepath.Join(s.dir, "events.jsonl") }

func appendLine(path string, v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal record: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", filepath.Base(path), err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}

// AppendSample writes one sample to the day file for its timestamp's local date.
func (s *Store) AppendSample(sm model.Sample) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return appendLine(s.samplePath(sm.Time), sm)
}

// AppendEvent writes one event to events.jsonl.
func (s *Store) AppendEvent(ev model.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return appendLine(s.eventsPath(), ev)
}

// readJSONL reads and decodes every line of a JSONL file into out via decode.
// A missing file yields an empty result, not an error.
func readJSONL[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open %s: %w", filepath.Base(path), err)
	}
	defer f.Close()

	var out []T
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for line := 1; sc.Scan(); line++ {
		b := sc.Bytes()
		if len(b) == 0 {
			continue
		}
		var v T
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, fmt.Errorf("parse %s line %d: %w", filepath.Base(path), line, err)
		}
		out = append(out, v)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", filepath.Base(path), err)
	}
	return out, nil
}

// ReadSamples returns all samples recorded on the local date of t, ordered as written.
func (s *Store) ReadSamples(t time.Time) ([]model.Sample, error) {
	return readJSONL[model.Sample](s.samplePath(t))
}

// ReadEvents returns all events whose local date matches that of t.
func (s *Store) ReadEvents(t time.Time) ([]model.Event, error) {
	all, err := readJSONL[model.Event](s.eventsPath())
	if err != nil {
		return nil, err
	}
	y, m, d := t.Date()
	var out []model.Event
	for _, ev := range all {
		ey, em, ed := ev.Time.Date()
		if ey == y && em == m && ed == d {
			out = append(out, ev)
		}
	}
	return out, nil
}
