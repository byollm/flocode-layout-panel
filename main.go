package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Default palette: cool blues/teals/lavender (Tokyo-Night-inspired).
// No orange anywhere by default.
var (
	cHeader  = lipgloss.Color("#7aa2f7") // blue
	cPanel   = lipgloss.Color("#9ece6a") // green
	cFocus   = lipgloss.Color("#bb9af7") // lavender
	cDim     = lipgloss.Color("#565f89") // muted
	cText    = lipgloss.Color("#c0caf5") // soft fg
	cSuccess = lipgloss.Color("#73daca") // teal

	styHeader = lipgloss.NewStyle().Foreground(cHeader).Bold(true)
	styPanel  = lipgloss.NewStyle().Foreground(cPanel)
	styFocus  = lipgloss.NewStyle().Foreground(cFocus).Bold(true)
	styDim    = lipgloss.NewStyle().Foreground(cDim)
	styText   = lipgloss.NewStyle().Foreground(cText)
	styOK     = lipgloss.NewStyle().Foreground(cSuccess)
)

// panel is a toggleable UI region.
type panel struct {
	key   string // toggle key letter
	name  string
	w, h  int    // size in cells
	fill  string // placeholder content
	on    bool
	focus bool // selected in the toggle list
}

type mode int

const (
	modeLayout mode = iota
	modeToggles
)

type tickMsg struct{}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second/18, func(time.Time) tea.Msg { return tickMsg{} })
}

// Width bounds for the layout. maxWidth caps manual 'w' growth; minWidth is
// the reasonable floor for wide terminals — narrow terminals are allowed to
// go below it so panels actually fit on screen.
const (
	minWidth = 60
	maxWidth = 160
)

type model struct {
	panels  []*panel
	preset  int
	width   int
	minW    int // smallest terminal width seen (floor for clamping)
	focus   int // focused panel index in layout (left-to-right)
	mode    mode
	status  string
	saveDir string   // optional override for tests; empty = user cache dir
	names   []string // preset names
	presets [][]string

	frame int // animation frame counter

}

var presetDefs = []struct {
	name   string
	panels []string
}{
	{"classic chat+files", []string{"chat", "files"}},
	{"zen chat only", []string{"chat"}},
	{"chat+tree", []string{"chat", "tree"}},
	{"status|chat", []string{"status", "chat"}},
	{"todo right", []string{"chat", "todo"}},
	{"tools left", []string{"tools", "chat"}},
	{"diff below", []string{"chat", "diff"}},
	{"terminal split", []string{"chat", "term"}},
	{"help overlay", []string{"chat", "help"}},
	{"three pane", []string{"tree", "chat", "files"}},
	{"dashboard", []string{"status", "tools", "todo", "chat", "logs", "verify"}},
	{"quad view", []string{"chat", "diff", "term", "files"}},
	{"monitor", []string{"logs", "mesh", "tokens", "chat"}},
	{"git focus", []string{"branch", "diff", "files", "chat"}},
	{"history focus", []string{"history", "chat", "todo"}},
	{"mesh focus", []string{"mesh", "status", "chat"}},
	{"budget focus", []string{"tokens", "model", "chat"}},
	{"preview focus", []string{"preview", "chat"}},
	{"ide style", []string{"tree", "editor", "preview"}},
	{"everything", []string{"tree", "chat", "files", "preview", "status", "todo", "verify", "tokens"}},
}

func newModel() *model {
	m := &model{
		width: 96,
		mode:  modeLayout,
	}
	defs := []struct {
		key, name, fill string
		w, h            int
	}{
		{"c", "chat", "· messages ·", 44, 10},
		{"f", "files", "file.go", 24, 10},
		{"t", "tree", "▾ src/", 24, 10},
		{"s", "status", "● ok", 24, 6},
		{"o", "tools", "bash edit", 24, 6},
		{"d", "todo", "[x] task", 24, 6},
		{"i", "diff", "+ added", 44, 6},
		{"e", "term", "$ go test", 44, 6},
		{"p", "preview", "render", 24, 8},
		{"l", "logs", "INFO", 24, 8},
		{"m", "mesh", "3 peers", 24, 4},
		{"k", "tokens", "12.3k", 24, 4},
		{"M", "model", "glm-4.7", 24, 4},
		{"b", "branch", "fix/lock", 24, 4},
		{"v", "verify", "PASS", 24, 6},
		{"h", "history", "session 42", 24, 8},
		{"H", "help", "? keys", 44, 5},
		{"E", "editor", "func main() {}", 50, 12},
	}
	for _, d := range defs {
		m.panels = append(m.panels, &panel{key: d.key, name: d.name, fill: d.fill, w: d.w, h: d.h})
	}
	for _, p := range presetDefs {
		m.names = append(m.names, p.name)
		m.presets = append(m.presets, p.panels)
	}
	m.applyPreset(0)
	return m
}

func (m *model) applyPreset(i int) {
	for _, p := range m.panels {
		p.on = false
	}
	for _, name := range m.presets[i] {
		p, err := m.byName(name)
		if err != nil {
			// A preset names a panel that doesn't exist — surface it instead
			// of silently enabling the wrong panel.
			m.status = "preset error: " + err.Error()
			return
		}
		p.on = true
	}
	m.preset = i
	m.focus = 0
}

func (m *model) byName(n string) (*panel, error) {
	for _, p := range m.panels {
		if p.name == n {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no panel %q", n)
}

func (m *model) active() []*panel {
	var out []*panel
	for _, p := range m.panels {
		if p.on {
			out = append(out, p)
		}
	}
	return out
}

func (m *model) Init() tea.Cmd { return tickCmd() }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Fit the layout to the terminal. Track the smallest width seen so
		// we can recover when a narrow terminal widens again.
		if msg.Width > 0 {
			if m.minW == 0 || msg.Width < m.minW {
				m.minW = msg.Width
			}
			maxFit := msg.Width - 2
			switch {
			case maxFit < minWidth:
				// Narrow terminal: fit exactly — no artificial floor that
				// would overflow the screen.
				if m.width > maxFit {
					m.width = maxFit
				}
			default:
				// Wide enough for the standard floor: keep at least
				// minWidth once there's room, but never exceed what fits.
				if m.width > maxFit {
					m.width = maxFit
				}
				if m.width < minWidth {
					m.width = minWidth
				}
			}
		}
		return m, nil
	case tickMsg:
		m.frame++
		return m, tickCmd()
	case saveResultMsg:
		if msg.err != nil {
			m.status = "save failed: " + msg.err.Error()
		} else {
			m.status = "saved " + msg.path
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			if m.mode == modeLayout {
				m.focusNext()
			} else {
				m.mode = modeLayout
				m.status = ""
			}
		case "P":
			if m.mode == modeLayout {
				m.applyPreset((m.preset + len(presetDefs) - 1) % len(presetDefs))
				m.status = "preset: " + m.names[m.preset]
			}
		case "N":
			if m.mode == modeLayout {
				m.applyPreset((m.preset + 1) % len(presetDefs))
				m.status = "preset: " + m.names[m.preset]
			}
		case "w":
			if m.mode == modeLayout {
				m.width += 12
				if m.width > maxWidth {
					m.width = minWidth
				}
				// Never let manual growth overflow the known terminal size.
				if term := m.minW - 2; term > 0 && m.width > term {
					m.width = term
				}
				m.status = fmt.Sprintf("width %d", m.width)
			}
		case "S":
			if m.mode == modeLayout {
				m.status = "saving…"
				return m, m.saveLayout()
			}
		case "enter", " ":
			if m.mode == modeToggles {
				for _, p := range m.panels {
					if p.focus {
						p.on = !p.on
						m.status = fmt.Sprintf("%s %s", p.name, onOff(p.on))
					}
				}
				m.clampFocus()
			}
		case "down", "j":
			if m.mode == modeToggles {
				m.moveToggle(1)
			}
		case "up", "k":
			if m.mode == modeToggles {
				m.moveToggle(-1)
			}
		case "T":
			m.mode = modeToggles
			m.syncToggleCursor()
			m.status = "toggles: ↑↓ move, space toggle, tab back"
		default:
			if len(msg.String()) == 1 {
				for _, p := range m.panels {
					if p.key == msg.String() {
						p.on = !p.on
						m.status = fmt.Sprintf("%s %s", p.name, onOff(p.on))
					}
				}
				m.clampFocus()
				m.syncToggleCursor()
			}
		}
	}
	return m, nil
}

// focusNext moves focus to the next active panel, wrapping.
func (m *model) focusNext() {
	act := m.active()
	if len(act) == 0 {
		return
	}
	m.focus = (m.focus + 1) % len(act)
}

// clampFocus keeps focus within the active panel list after panels are
// toggled off; otherwise the focus marker silently disappears.
func (m *model) clampFocus() {
	n := len(m.active())
	if n == 0 {
		return
	}
	if m.focus >= n {
		m.focus = n - 1
	}
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func (m *model) moveToggle(d int) {
	n := len(m.panels)
	for i, p := range m.panels {
		if p.focus {
			p.focus = false
			m.panels[(i+d+n)%n].focus = true
			return
		}
	}
	m.panels[0].focus = true
}

// syncToggleCursor aligns the toggles-mode cursor (p.focus) with the
// layout-mode focus (m.focus) so the two modes don't desync. It marks the
// active panel at m.focus as focused, falling back to the first active panel
// (or none) if the index is out of range.
func (m *model) syncToggleCursor() {
	for _, p := range m.panels {
		p.focus = false
	}
	act := m.active()
	if len(act) == 0 {
		return
	}
	if m.focus < 0 || m.focus >= len(act) {
		m.focus = 0
	}
	act[m.focus].focus = true
}

// saveLayout returns a tea.Cmd that writes the current layout to a file in
// the user's cache dir. It runs off the event loop so the TUI never blocks
// on disk I/O, and uses O_EXCL so it can never follow a symlink or overwrite
// an existing file (no TOCTOU). The save dir can be overridden for tests via
// m.saveDir so tests never write to the real user cache.
func (m *model) saveLayout() tea.Cmd {
	dir := m.saveDir
	if dir == "" {
		var err error
		dir, err = os.UserCacheDir()
		if err != nil {
			return func() tea.Msg { return saveResultMsg{err: err} }
		}
	}
	dir = filepath.Join(dir, "flocode-layout-panel")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return func() tea.Msg { return saveResultMsg{err: err} }
	}
	// Snapshot the layout now (on the event loop) so the goroutine doesn't
	// read mutable model state concurrently.
	lines := m.render()
	return func() tea.Msg {
		var b strings.Builder
		for _, line := range lines {
			b.WriteString(stripANSI(line))
			b.WriteString("\n")
		}
		// find next free layout number
		n := 1
		for n <= 99 {
			path := filepath.Join(dir, fmt.Sprintf("layout-%02d.txt", n))
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err == nil {
				_, werr := f.WriteString(b.String())
				cerr := f.Close()
				if werr != nil {
					return saveResultMsg{err: werr}
				}
				if cerr != nil {
					return saveResultMsg{err: cerr}
				}
				return saveResultMsg{path: path}
			}
			if !os.IsExist(err) {
				return saveResultMsg{err: err}
			}
			n++
		}
		return saveResultMsg{err: fmt.Errorf("no free layout slot (1-99)")}
	}
}

// saveResultMsg reports the outcome of an async layout save.
type saveResultMsg struct {
	path string
	err  error
}

// stripANSI removes ANSI escape sequences so saved layouts are plain text.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

func (m *model) View() string {
	var b strings.Builder
	if m.mode == modeToggles {
		b.WriteString(m.togglesView())
	} else {
		b.WriteString(strings.Join(m.render(), "\n"))
	}
	b.WriteString("\n")
	b.WriteString(m.footer())
	return b.String()
}

func (m *model) footer() string {
	f := fmt.Sprintf(" preset %d/%d %s · width %d · %s", m.preset+1, len(presetDefs), m.names[m.preset], m.width, m.status)
	if m.mode == modeLayout {
		f = styDim.Render(" tab focus · P/N preset · w width · T toggles · S save · q quit") + styText.Render(f)
	}
	return f
}

func (m *model) togglesView() string {
	var b strings.Builder
	b.WriteString(styHeader.Render("panel toggles") + styDim.Render(" — space toggles, ↑↓ moves, tab returns") + "\n\n")
	for _, p := range m.panels {
		cur := " "
		if p.focus {
			cur = ">"
		}
		mark := " "
		if p.on {
			mark = "x"
		}
		line := fmt.Sprintf("%s [%s] %-8s (%s)", cur, mark, p.name, p.key)
		if p.focus {
			b.WriteString(styFocus.Render(line))
		} else if p.on {
			b.WriteString(styOK.Render(line))
		} else {
			b.WriteString(styDim.Render(line))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// The header gradient sweeps the cool band of the hue wheel — teal →
// blue → lavender (HSV 170°–290°) — so it never enters the warm/orange
// region. The sweep ping-pongs (mirrors at the wrap) so the flow never
// jumps, and the phase advances 1/64 of the span per frame (~3.5s per
// full sweep at 18fps).
const (
	hueMin = 170.0
	hueMax = 290.0
)

// sweepHue maps a normalized position t (0..1 across the band) plus the
// frame phase to a hue, mirrored so the sweep flows back and forth.
func (m *model) sweepHue(t float64) float64 {
	span := hueMax - hueMin
	phase := float64(m.frame) / 64.0
	u := math.Mod(t+phase, 2.0)
	if u > 1.0 {
		u = 2.0 - u
	}
	return hueMin + u*span
}

// gradientHeader renders the title as a smooth traveling hue gradient with
// a subtle brightness wave riding on top, so two frequencies of motion
// flow across the text at once.
func (m *model) gradientHeader(title string) string {
	centered := center(title, m.width)
	runes := []rune(centered)
	n := len(runes)
	var b strings.Builder
	for i, r := range runes {
		t := float64(i) / float64(max(1, n-1))
		hue := m.sweepHue(t)
		wave := 0.10 * math.Sin(2*math.Pi*(t*2.0+float64(m.frame)/32.0))
		v := 0.90 + wave
		c := hsvToHex(hue, 0.72, v)
		b.WriteString(lipgloss.NewStyle().Foreground(c).Bold(true).Render(string(r)))
	}
	return b.String()
}

// gradientRule renders a horizontal rule as a dimmer gradient sharing the
// title's sweep, so the whole header band flows as one piece.
func (m *model) gradientRule(mark string) string {
	var b strings.Builder
	for i := 0; i < m.width; i++ {
		t := float64(i) / float64(max(1, m.width-1))
		c := hsvToHex(m.sweepHue(t), 0.45, 0.60)
		b.WriteString(lipgloss.NewStyle().Foreground(c).Render(mark))
	}
	return b.String()
}

// hsvToHex converts HSV (h in degrees, s/v in 0..1) to a hex color.
func hsvToHex(h, s, v float64) lipgloss.Color {
	hp := math.Mod(h, 360.0) / 60.0
	i := int(math.Floor(hp))
	f := hp - float64(i)
	p := v * (1 - s)
	q := v * (1 - s*f)
	t := v * (1 - s*(1-f))
	var r, g, b float64
	switch i % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	case 5:
		r, g, b = v, p, q
	}
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", uint8(r*255), uint8(g*255), uint8(b*255)))
}

// pulse interpolates between two colors over a sine-like cycle.
func pulse(frame int, period int) float64 {
	ph := float64(frame%period) / float64(period) * 2 * math.Pi
	return (1 + math.Sin(ph)) / 2 // 0..1
}

// focusStyle returns the body style for a focused panel, breathing between
// lavender and white.
func focusStyle(frame int) lipgloss.Style {
	t := pulse(frame, 36)
	c := lerpColor(cFocus, lipgloss.Color("#ffffff"), t)
	return lipgloss.NewStyle().Foreground(c).Bold(true)
}

// lerpColor linearly interpolates two hex colors.
func lerpColor(a, b lipgloss.Color, t float64) lipgloss.Color {
	pa, pb := hexToRGB(string(a)), hexToRGB(string(b))
	r := uint8(float64(pa[0]) + t*(float64(pb[0])-float64(pa[0])))
	g := uint8(float64(pa[1]) + t*(float64(pb[1])-float64(pa[1])))
	bch := uint8(float64(pa[2]) + t*(float64(pb[2])-float64(pa[2])))
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r, g, bch))
}

func hexToRGB(h string) [3]int {
	var r, g, b int
	fmt.Sscanf(h, "#%02x%02x%02x", &r, &g, &b)
	return [3]int{r, g, b}
}

// shimmerFill produces a moving highlight band across the panel body text.
func shimmerFill(fill string, w, frame int) string {
	s := center(fill, w)
	runes := []rune(s)
	pos := frame % (w + 8)
	var b strings.Builder
	for i, r := range runes {
		d := pos - i
		if d >= 0 && d < 5 {
			// bright band trailing off
			c := lerpColor(cText, lipgloss.Color("#ffffff"), 1-float64(d)/5)
			b.WriteString(lipgloss.NewStyle().Foreground(c).Render(string(r)))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (m *model) render() []string {
	act := m.active()
	var lines []string
	title := fmt.Sprintf("flocode — %s", m.names[m.preset])
	lines = append(lines, m.gradientRule("▔"), m.gradientHeader(title), m.gradientRule("▔"))

	// group active panels into rows that fit the width
	var rows [][]*panel
	var cur []*panel
	curw := 0
	for _, p := range act {
		if len(cur) > 0 && curw+p.w+1 > m.width {
			rows = append(rows, cur)
			cur = nil
			curw = 0
		}
		cur = append(cur, p)
		curw += p.w + 1
	}
	if len(cur) > 0 {
		rows = append(rows, cur)
	}

	fi := 0
	for _, row := range rows {
		cols := make([][]string, len(row))
		maxh := 0
		for i, p := range row {
			focused := fi == m.focus
			fi++
			cols[i] = drawPanel(p, focused, m.frame)
			if len(cols[i]) > maxh {
				maxh = len(cols[i])
			}
		}
		for r := 0; r < maxh; r++ {
			parts := make([]string, len(cols))
			for i := range cols {
				if r < len(cols[i]) {
					parts[i] = cols[i][r]
				} else {
					parts[i] = strings.Repeat(" ", row[i].w)
				}
			}
			lines = append(lines, strings.Join(parts, " "))
		}
	}
	lines = append(lines, styDim.Render("┌"+center("input", m.width-2)+"┐"))
	lines = append(lines, styText.Render("│"+center("> _", m.width-2)+"│"))
	lines = append(lines, styDim.Render("└"+strings.Repeat("─", m.width-2)+"┘"))
	return lines
}

func drawPanel(p *panel, focused bool, frame int) []string {
	body := styPanel
	if focused {
		body = focusStyle(frame)
	}
	mark := " "
	if focused {
		mark = "*"
	}
	title := mark + p.name + mark
	top := "┌" + title + strings.Repeat("─", max(0, p.w-2-displayWidth(title))) + "┐"
	lines := []string{top}
	for i := 1; i < p.h-1; i++ {
		lines = append(lines, body.Render("│"+shimmerFill(p.fill, p.w-2, frame)+"│"))
	}
	lines = append(lines, styDim.Render("└"+strings.Repeat("─", p.w-2)+"┘"))
	return lines
}

// displayWidth returns the number of display cells a string occupies
// (runes; adequate for the box-drawing chars used here).
func displayWidth(s string) int { return len([]rune(s)) }

// center pads s to exactly w display cells, rune-safe.
func center(s string, w int) string {
	r := []rune(s)
	if len(r) >= w {
		if len(r) == w {
			return s
		}
		return string(r[:w])
	}
	l := (w - len(r)) / 2
	return strings.Repeat(" ", l) + s + strings.Repeat(" ", w-l-len(r))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func main() {
	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
