// Package ui is the interactive view of every job on the machine.
package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sanifhimani/turnus/internal/job"
	"github.com/sanifhimani/turnus/internal/render"
	"github.com/sanifhimani/turnus/internal/style"
)

// Loader finds and checks jobs, including OS jobs when builtin is true.
type Loader func(builtin bool) []job.Job

// refreshEvery keeps states and countdowns current while the view is open.
const refreshEvery = 5 * time.Second

type (
	loadedMsg struct {
		jobs []job.Job
		all  bool
	}
	tickMsg        time.Time
	clearStatusMsg struct{ id int }
	execDoneMsg    struct{ err error }
)

type model struct {
	load  Loader
	style style.Style
	now   time.Time

	all  bool
	jobs []job.Job

	// rows are the visible jobs in display order: scheduled, then services.
	rows      []render.Row
	scheduled int
	cursor    int
	offset    int

	filter string
	typing bool

	detail       bool
	detailOffset int

	status   string
	statusID int

	width, height int
}

// New returns the interactive view, starting with jobs already loaded so
// the first frame is complete.
func New(load Loader, jobs []job.Job, all bool, s style.Style) tea.Model {
	m := &model{load: load, jobs: jobs, all: all, style: s, now: time.Now()}
	m.rebuild("")
	return m
}

func (m *model) Init() tea.Cmd { return tick() }

func tick() tea.Cmd {
	return tea.Tick(refreshEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *model) reload() tea.Cmd {
	load, all := m.load, m.all
	return func() tea.Msg { return loadedMsg{jobs: load(all), all: all} }
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.scroll()
	case tickMsg:
		return m, tea.Batch(m.reload(), tick())
	case loadedMsg:
		if msg.all != m.all {
			return m, nil
		}
		id := m.selectedID()
		m.jobs, m.now = msg.jobs, time.Now()
		m.rebuild(id)
	case execDoneMsg:
		if msg.err != nil {
			return m, tea.Batch(m.flash(msg.err.Error()), m.reload())
		}
		return m, m.reload()
	case clearStatusMsg:
		if msg.id == m.statusID {
			m.status = ""
		}
	case tea.KeyPressMsg:
		return m, m.key(msg.String())
	}
	return m, nil
}

func (m *model) key(k string) tea.Cmd {
	if k == "ctrl+c" {
		return tea.Quit
	}
	if m.typing {
		return m.typeFilter(k)
	}
	if m.detail {
		return m.detailKey(k)
	}

	switch k {
	case "q":
		return tea.Quit
	case "esc":
		if m.filter != "" {
			m.setFilter("")
		}
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "pgup", "ctrl+u":
		m.move(-m.listHeight() / 2)
	case "pgdown", "ctrl+d":
		m.move(m.listHeight() / 2)
	case "home", "g":
		m.move(-len(m.rows))
	case "end", "G":
		m.move(len(m.rows))
	case "/":
		m.typing = true
	case "enter":
		if m.selected() != nil {
			m.detail, m.detailOffset = true, 0
		}
	case "a":
		m.all = !m.all
		msg := "Hiding built-in jobs"
		if m.all {
			msg = "Showing built-in jobs"
		}
		return tea.Batch(m.reload(), m.flash(msg))
	case "r":
		return tea.Batch(m.reload(), m.flash("Refreshed"))
	default:
		return m.action(k)
	}
	return nil
}

func (m *model) detailKey(k string) tea.Cmd {
	switch k {
	case "esc", "enter", "q", "backspace", "left", "h":
		m.detail = false
	case "up", "k":
		m.detailOffset = max(m.detailOffset-1, 0)
	case "down", "j":
		m.detailOffset++
	default:
		return m.action(k)
	}
	return nil
}

func (m *model) typeFilter(k string) tea.Cmd {
	switch k {
	case "esc":
		m.typing = false
		m.setFilter("")
	case "enter":
		m.typing = false
	case "backspace":
		if r := []rune(m.filter); len(r) > 0 {
			m.setFilter(string(r[:len(r)-1]))
		}
	case "up", "down":
		m.typing = false
		return m.key(k)
	case "space":
		m.setFilter(m.filter + " ")
	default:
		if len([]rune(k)) == 1 {
			m.setFilter(m.filter + k)
		}
	}
	return nil
}

func (m *model) setFilter(f string) {
	id := m.selectedID()
	m.filter = f
	m.rebuild(id)
}

// flash shows a message in the footer for a couple of seconds.
func (m *model) flash(text string) tea.Cmd {
	m.statusID++
	m.status = text
	id := m.statusID
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return clearStatusMsg{id: id} })
}

// rebuild recomputes the visible rows and keeps the cursor on the job with
// the given ID when it is still there.
func (m *model) rebuild(keepID string) {
	jobs := m.jobs
	if m.filter != "" {
		jobs = slices.DeleteFunc(slices.Clone(jobs), func(j job.Job) bool { return !matches(&j, m.filter) })
	}
	scheduled, services := render.Group(jobs, m.now)
	m.rows = slices.Concat(scheduled, services)
	m.scheduled = len(scheduled)

	m.cursor = min(m.cursor, max(len(m.rows)-1, 0))
	if keepID != "" {
		if i := slices.IndexFunc(m.rows, func(r render.Row) bool { return r.Job.ID == keepID }); i >= 0 {
			m.cursor = i
		}
	}
	m.scroll()
}

func matches(j *job.Job, filter string) bool {
	filter = strings.ToLower(filter)
	for _, field := range []string{j.Name, j.Command, j.Schedule, string(j.Source), j.File} {
		if strings.Contains(strings.ToLower(field), filter) {
			return true
		}
	}
	return false
}

func (m *model) move(n int) {
	if len(m.rows) == 0 {
		return
	}
	m.cursor = min(max(m.cursor+n, 0), len(m.rows)-1)
	m.scroll()
}

func (m *model) selected() *job.Job {
	if m.cursor < len(m.rows) {
		return m.rows[m.cursor].Job
	}
	return nil
}

func (m *model) selectedID() string {
	if j := m.selected(); j != nil {
		return j.ID
	}
	return ""
}

// line is one line of the list: a section title, a blank, or a row.
type line struct {
	text string
	row  int // index into rows, or -1
}

func (m *model) lines() []line {
	var out []line
	s := m.style
	section := func(title string, from, to int) {
		if from == to {
			return
		}
		if len(out) > 0 {
			out = append(out, line{row: -1})
		}
		out = append(out, line{text: "  " + s.Bold(title) + " " + s.Dim(fmt.Sprint(to-from)), row: -1})
		for i := from; i < to; i++ {
			out = append(out, line{row: i})
		}
	}
	section("Scheduled", 0, m.scheduled)
	section("Services", m.scheduled, len(m.rows))
	return out
}

// scroll keeps the selected row, and its section title when near the top,
// inside the list area.
func (m *model) scroll() {
	h := m.listHeight()
	if h <= 0 {
		return
	}
	lines := m.lines()
	at := slices.IndexFunc(lines, func(l line) bool { return l.row == m.cursor })
	if at < 0 {
		m.offset = 0
		return
	}
	// Show the section title when the cursor is on a section's first row.
	top := at
	for top > 0 && lines[top-1].row < 0 && at-top < 2 {
		top--
	}
	switch {
	case top < m.offset:
		m.offset = top
	case at >= m.offset+h:
		m.offset = at - h + 1
	}
	m.offset = min(m.offset, max(len(lines)-h, 0))
}

// paneHeight is the height of the detail pane under the list, which is
// left out on short windows.
func (m *model) paneHeight() int {
	if m.height < 16 {
		return 0
	}
	return min(10, m.height/3)
}

// listHeight leaves room for the header and its blank line, the footer, and
// the pane with the rule above it.
func (m *model) listHeight() int {
	h := m.height - 3
	if p := m.paneHeight(); p > 0 {
		h -= p + 1
	}
	return h
}

func (m *model) View() tea.View {
	var v tea.View
	v.AltScreen = true
	v.WindowTitle = "turnus"
	if m.width == 0 {
		return v
	}

	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n\n")

	if m.detail {
		b.WriteString(m.detailView(m.height - 3))
	} else {
		b.WriteString(m.listView())
		if ph := m.paneHeight(); ph > 0 {
			b.WriteString(m.style.Dim(strings.Repeat("─", m.width)))
			b.WriteByte('\n')
			b.WriteString(m.pane(ph))
		}
	}
	b.WriteString(m.footer())
	v.SetContent(clip(b.String(), m.width))
	return v
}

// clip cuts every line to the window width, so a long value can never wrap
// and push the layout down.
func clip(s string, width int) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, width, "")
	}
	return strings.Join(lines, "\n")
}

func (m *model) header() string {
	s := m.style
	h := " " + s.Bold("turnus") + "  " + render.Summary(m.jobs, s)
	if m.all {
		h += s.Dim(" · with built-in")
	}
	if m.filter != "" && !m.typing {
		h += "  " + s.Cyan("/"+m.filter)
	}
	return h
}

func (m *model) listView() string {
	h := m.listHeight()
	lines := m.lines()
	widths := render.Fit(m.rows, m.width, 2)

	var b strings.Builder
	n := 0
	if len(m.rows) == 0 {
		msg := "No jobs found."
		if m.filter != "" {
			msg = fmt.Sprintf("No jobs match %q.", m.filter)
		}
		b.WriteString("  " + m.style.Dim(msg) + "\n")
		n++
	}
	for i := m.offset; i < len(lines) && n < h; i++ {
		l := lines[i]
		if l.row >= 0 {
			r := m.rows[l.row]
			if l.row == m.cursor {
				b.WriteString(m.style.Cyan("▌ "))
				b.WriteString(r.Line(widths, m.style, true))
			} else {
				b.WriteString("  ")
				b.WriteString(r.Line(widths, m.style, false))
			}
		} else {
			b.WriteString(l.text)
		}
		b.WriteByte('\n')
		n++
	}
	for ; n < h; n++ {
		b.WriteByte('\n')
	}
	return b.String()
}

func (m *model) pane(h int) string {
	j := m.selected()
	var lines []string
	if j != nil {
		lines = m.describe(j)
	}
	more := len(lines) > h
	if more {
		lines = lines[:h-1]
		lines = append(lines, m.style.Dim("  enter for more"))
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n") + "\n"
}

func (m *model) detailView(h int) string {
	j := m.selected()
	if j == nil {
		return strings.Repeat("\n", h)
	}
	lines := m.describe(j)
	m.detailOffset = min(m.detailOffset, max(len(lines)-h, 0))
	lines = lines[m.detailOffset:]
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n") + "\n"
}

func (m *model) footer() string {
	s := m.style
	switch {
	case m.typing:
		return " " + s.Cyan("/") + m.filter + s.Cyan("█") + s.Dim("   enter apply · esc clear")
	case m.status != "":
		return " " + s.Cyan(m.status)
	case m.detail:
		return " " + s.Dim(keys("esc back", "e edit", "l log", "c copy fix", "q back"))
	}
	hint := keys("↑↓ move", "/ filter", "enter details", "e edit", "l log", "c copy fix", "a all", "q quit")
	if m.filter != "" {
		hint = keys("↑↓ move", "esc clear filter", "enter details", "e edit", "l log", "c copy fix", "q quit")
	}
	return " " + s.Dim(render.Truncate(hint, m.width-1))
}

func keys(k ...string) string { return strings.Join(k, " · ") }
