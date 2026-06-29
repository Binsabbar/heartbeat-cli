// Package tui implements the live monitoring terminal UI with Bubble Tea: a
// large current-BPM readout, a heart-rate sparkline, a colour-coded stress gauge
// and zone, and a scrolling log of stress changes and tags. The user can tag the
// current moment or start/stop a named session, which are persisted as events.
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/binsabbar/heartrate-monitor/internal/model"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Frame is one step of the monitoring stream delivered to the UI.
type Frame struct {
	Sample model.Sample
	Change *model.Event // non-nil when a stress-zone transition just committed
}

// EventSink persists user-generated events (tags, session boundaries).
type EventSink interface {
	AppendEvent(model.Event) error
}

const maxEventLog = 8

type (
	frameMsg        Frame
	streamClosedMsg struct{}
)

// Model is the Bubble Tea model for `hrm monitor`.
type Model struct {
	sink   EventSink
	frames <-chan Frame
	now    func() time.Time

	width, height int
	latest        model.Sample
	haveSample    bool
	history       []int
	connected     bool
	log           []string

	tagging       bool
	input         textinput.Model
	sessionActive bool
}

// New builds the model. frames is the live stream; sink persists tag/session events.
func New(frames <-chan Frame, sink EventSink) Model {
	ti := textinput.New()
	ti.Placeholder = "label (e.g. standup, JIRA-123)"
	ti.CharLimit = 80
	return Model{
		sink:    sink,
		frames:  frames,
		now:     time.Now,
		input:   ti,
		history: make([]int, 0, 256),
	}
}

func waitFrame(ch <-chan Frame) tea.Cmd {
	return func() tea.Msg {
		f, ok := <-ch
		if !ok {
			return streamClosedMsg{}
		}
		return frameMsg(f)
	}
}

// Init starts listening for frames.
func (m Model) Init() tea.Cmd { return waitFrame(m.frames) }

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case streamClosedMsg:
		m.connected = false
		return m, nil

	case frameMsg:
		m.connected = true
		m.haveSample = true
		m.latest = msg.Sample
		m.history = append(m.history, msg.Sample.BPM)
		if len(m.history) > 240 {
			m.history = m.history[len(m.history)-240:]
		}
		if msg.Change != nil {
			m.pushLog(fmt.Sprintf("%s  stress %s → %s",
				msg.Change.Time.Format("15:04:05"), msg.Change.From, msg.Change.To))
		}
		return m, waitFrame(m.frames)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.tagging {
		switch msg.Type {
		case tea.KeyEnter:
			label := strings.TrimSpace(m.input.Value())
			if label != "" {
				m.addEvent(model.Event{Time: m.now(), Type: model.EventTag, Label: label})
				m.pushLog(fmt.Sprintf("%s  tag: %s", m.now().Format("15:04:05"), label))
			}
			m.tagging = false
			m.input.Blur()
			m.input.SetValue("")
			return m, nil
		case tea.KeyEsc:
			m.tagging = false
			m.input.Blur()
			m.input.SetValue("")
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "t":
		m.tagging = true
		m.input.Focus()
		return m, textinput.Blink
	case "s":
		now := m.now()
		if m.sessionActive {
			m.addEvent(model.Event{Time: now, Type: model.EventSessionEnd})
			m.pushLog(now.Format("15:04:05") + "  session ended")
		} else {
			m.addEvent(model.Event{Time: now, Type: model.EventSessionStart})
			m.pushLog(now.Format("15:04:05") + "  session started")
		}
		m.sessionActive = !m.sessionActive
		return m, nil
	}
	return m, nil
}

func (m *Model) addEvent(ev model.Event) {
	if m.sink == nil {
		return
	}
	_ = m.sink.AppendEvent(ev) // best-effort; UI keeps running on write error
}

func (m *Model) pushLog(line string) {
	m.log = append(m.log, line)
	if len(m.log) > maxEventLog {
		m.log = m.log[len(m.log)-maxEventLog:]
	}
}

// ---- rendering ----

var (
	zoneColors = map[model.Zone]lipgloss.Color{
		model.ZoneCalm:     lipgloss.Color("42"),
		model.ZoneMild:     lipgloss.Color("220"),
		model.ZoneElevated: lipgloss.Color("208"),
		model.ZoneHigh:     lipgloss.Color("196"),
	}
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	boxStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
)

var sparkRunes = []rune("▁▂▃▄▅▆▇█")

// View renders the dashboard.
func (m Model) View() string {
	var b strings.Builder

	status := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("● disconnected")
	if m.connected {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("● live")
	}
	b.WriteString(titleStyle.Render("♥ Heart Rate & Stress Monitor") + "  " + status + "\n\n")

	if !m.haveSample {
		b.WriteString(dimStyle.Render("Waiting for heart-rate broadcast…\n"))
		b.WriteString("\n" + m.footer())
		return b.String()
	}

	zc := zoneColors[m.latest.Zone]
	bpm := lipgloss.NewStyle().Bold(true).Foreground(zc).
		Render(fmt.Sprintf("%3d BPM", m.latest.BPM))
	zone := lipgloss.NewStyle().Bold(true).Foreground(zc).
		Render(strings.ToUpper(string(m.latest.Zone)))

	left := boxStyle.Render(bpm + "\n" + dimStyle.Render("zone ") + zone)
	right := boxStyle.Render(m.gauge(m.latest.Stress, zc))
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right) + "\n\n")

	width := m.width
	if width <= 0 {
		width = 60
	}
	b.WriteString(dimStyle.Render("heart rate") + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(zc).Render(sparkline(m.history, width)) + "\n\n")

	if len(m.log) > 0 {
		b.WriteString(dimStyle.Render("events") + "\n")
		for _, l := range m.log {
			b.WriteString("  " + l + "\n")
		}
		b.WriteString("\n")
	}

	if m.tagging {
		b.WriteString("tag: " + m.input.View() + "\n")
	}
	b.WriteString(m.footer())
	return b.String()
}

func (m Model) footer() string {
	session := "start"
	if m.sessionActive {
		session = "end"
	}
	return dimStyle.Render(fmt.Sprintf("t tag · s %s session · q quit", session))
}

// gauge renders a 0..100 stress bar.
func (m Model) gauge(stress float64, c lipgloss.Color) string {
	const cells = 24
	filled := min(int(stress/100*cells+0.5), cells)
	bar := lipgloss.NewStyle().Foreground(c).Render(strings.Repeat("█", filled)) +
		dimStyle.Render(strings.Repeat("░", cells-filled))
	return fmt.Sprintf("stress %3.0f/100\n%s", stress, bar)
}

// sparkline maps recent BPM values to block runes scaled to their own min/max.
func sparkline(vals []int, width int) string {
	if len(vals) == 0 {
		return ""
	}
	if width > len(vals) {
		width = len(vals)
	}
	vals = vals[len(vals)-width:]
	lo, hi := vals[0], vals[0]
	for _, v := range vals {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	span := hi - lo
	var b strings.Builder
	for _, v := range vals {
		idx := 0
		if span > 0 {
			idx = (v - lo) * (len(sparkRunes) - 1) / span
		}
		b.WriteRune(sparkRunes[idx])
	}
	return b.String()
}
