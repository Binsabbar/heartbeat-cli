// Package tui implements the live monitoring terminal UI with Bubble Tea: a
// large current-BPM readout, a heart-rate sparkline, a colour-coded stress gauge
// and zone, and a scrolling log of stress changes and tags.
//
// The user can open a tag/interval (meeting, focus, break, …) with a small form
// capturing a kind, title, optional person and note, then close it later to
// record its duration. A tag that is never closed stays a point marker.
package tui

import (
	"fmt"
	"strconv"
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

// EventSink persists user-generated events (tags).
type EventSink interface {
	AppendEvent(model.Event) error
}

const maxEventLog = 8

type (
	frameMsg        Frame
	streamClosedMsg struct{}
)

// mode is the current interaction state.
type mode int

const (
	modeNormal  mode = iota
	modeTagging      // filling in the tag form
	modeClosing      // picking which open tag to close
)

// form field indices.
const (
	fieldKind = iota
	fieldTitle
	fieldPerson
	fieldNote
	numFields
)

// openTag is a tag interval that has been opened but not yet closed.
type openTag struct {
	id, kind, title string
	start           time.Time
}

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

	mode     mode
	inputs   []textinput.Model
	focus    int
	openTags []openTag
}

// New builds the model. frames is the live stream; sink persists tag events.
func New(frames <-chan Frame, sink EventSink) Model {
	return Model{
		sink:    sink,
		frames:  frames,
		now:     time.Now,
		inputs:  newForm(),
		history: make([]int, 0, 256),
	}
}

func newForm() []textinput.Model {
	mk := func(placeholder string, limit int) textinput.Model {
		ti := textinput.New()
		ti.Placeholder = placeholder
		ti.CharLimit = limit
		return ti
	}
	in := make([]textinput.Model, numFields)
	in[fieldKind] = mk("meeting | focus | break | interrupt", 24)
	in[fieldTitle] = mk("title (e.g. Sprint planning)", 80)
	in[fieldPerson] = mk("person(s) — optional", 80)
	in[fieldNote] = mk("note — optional (e.g. JIRA-451)", 120)
	return in
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
	switch m.mode {
	case modeTagging:
		return m.handleTagging(msg)
	case modeClosing:
		return m.handleClosing(msg)
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "t":
		return m.startTagging()
	case "e":
		return m.startClosing()
	}
	return m, nil
}

// startTagging focuses the form on the kind field.
func (m Model) startTagging() (tea.Model, tea.Cmd) {
	m.mode = modeTagging
	m.focus = fieldKind
	for i := range m.inputs {
		m.inputs[i].SetValue("")
		m.inputs[i].Blur()
	}
	cmd := m.inputs[fieldKind].Focus()
	return m, cmd
}

func (m Model) handleTagging(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = modeNormal
		return m, nil
	case tea.KeyEnter, tea.KeyTab:
		if m.focus < numFields-1 {
			return m.focusField(m.focus + 1)
		}
		return m.submitTag()
	case tea.KeyShiftTab:
		if m.focus > 0 {
			return m.focusField(m.focus - 1)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
	return m, cmd
}

func (m Model) focusField(i int) (tea.Model, tea.Cmd) {
	m.inputs[m.focus].Blur()
	m.focus = i
	return m, m.inputs[i].Focus()
}

func (m Model) submitTag() (tea.Model, tea.Cmd) {
	title := strings.TrimSpace(m.inputs[fieldTitle].Value())
	if title == "" {
		// Title is required; jump back to it rather than saving an empty tag.
		return m.focusField(fieldTitle)
	}
	kind := strings.TrimSpace(m.inputs[fieldKind].Value())
	if kind == "" {
		kind = "tag"
	}
	now := m.now()
	id := strconv.FormatInt(now.UnixNano(), 36)
	ev := model.Event{
		Time:   now,
		Type:   model.EventTag,
		ID:     id,
		Kind:   kind,
		Label:  title,
		Person: strings.TrimSpace(m.inputs[fieldPerson].Value()),
		Note:   strings.TrimSpace(m.inputs[fieldNote].Value()),
	}
	m.addEvent(ev)
	m.openTags = append(m.openTags, openTag{id: id, kind: kind, title: title, start: now})
	m.pushLog(fmt.Sprintf("%s  opened [%s] %s", now.Format("15:04:05"), kind, title))
	m.mode = modeNormal
	return m, nil
}

// startClosing closes the only open tag, or enters the picker when several are open.
func (m Model) startClosing() (tea.Model, tea.Cmd) {
	switch len(m.openTags) {
	case 0:
		m.pushLog(m.now().Format("15:04:05") + "  (no open tags to close)")
		return m, nil
	case 1:
		return m.closeTag(0), nil
	default:
		m.mode = modeClosing
		return m, nil
	}
}

func (m Model) handleClosing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		m.mode = modeNormal
		return m, nil
	}
	if n, err := strconv.Atoi(msg.String()); err == nil && n >= 1 && n <= len(m.openTags) {
		mm := m.closeTag(n - 1)
		mm.mode = modeNormal
		return mm, nil
	}
	return m, nil
}

// closeTag writes the end event for openTags[i] and removes it from the open set.
func (m Model) closeTag(i int) Model {
	t := m.openTags[i]
	now := m.now()
	m.addEvent(model.Event{Time: now, Type: model.EventTagEnd, ID: t.id})
	m.pushLog(fmt.Sprintf("%s  closed [%s] %s (%s)",
		now.Format("15:04:05"), t.kind, t.title, fmtShort(now.Sub(t.start))))
	m.openTags = append(m.openTags[:i], m.openTags[i+1:]...)
	return m
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

var formLabels = [numFields]string{
	fieldKind:   "kind  ",
	fieldTitle:  "title ",
	fieldPerson: "person",
	fieldNote:   "note  ",
}

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

	if len(m.openTags) > 0 {
		b.WriteString(dimStyle.Render("open") + "\n")
		for _, t := range m.openTags {
			fmt.Fprintf(&b, "  • [%s] %s  %s\n", t.kind, t.title,
				dimStyle.Render(fmtShort(m.now().Sub(t.start))))
		}
		b.WriteString("\n")
	}

	if len(m.log) > 0 {
		b.WriteString(dimStyle.Render("events") + "\n")
		for _, l := range m.log {
			b.WriteString("  " + l + "\n")
		}
		b.WriteString("\n")
	}

	switch m.mode {
	case modeTagging:
		b.WriteString(m.viewForm())
	case modeClosing:
		b.WriteString(m.viewClosing())
	}
	b.WriteString(m.footer())
	return b.String()
}

func (m Model) viewForm() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("New tag") + dimStyle.Render("  (Tab/Enter next · Esc cancel)") + "\n")
	for i := range m.inputs {
		b.WriteString("  " + dimStyle.Render(formLabels[i]) + " " + m.inputs[i].View() + "\n")
	}
	return b.String()
}

func (m Model) viewClosing() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Close which tag?") + dimStyle.Render("  (press number · Esc cancel)") + "\n")
	for i, t := range m.openTags {
		fmt.Fprintf(&b, "  %d. [%s] %s\n", i+1, t.kind, t.title)
	}
	return b.String()
}

func (m Model) footer() string {
	open := ""
	if n := len(m.openTags); n > 0 {
		open = fmt.Sprintf(" (%d open)", n)
	}
	return dimStyle.Render(fmt.Sprintf("t tag · e end%s · q quit", open))
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

// fmtShort renders a duration compactly, e.g. "32m", "1h05m", "45s".
func fmtShort(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	mn := d / time.Minute
	d -= mn * time.Minute
	s := d / time.Second
	switch {
	case h > 0:
		return fmt.Sprintf("%dh%02dm", h, mn)
	case mn > 0:
		return fmt.Sprintf("%dm", mn)
	default:
		return fmt.Sprintf("%ds", s)
	}
}
