package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
)

// Default palette: cool blues/teals/lavender (Tokyo-Night-inspired).
// No orange anywhere by default.
var (
	cHeader  = lipgloss.Color("#7aa2f7") // blue
	cAccent  = lipgloss.Color("#7dcfff") // light blue
	cPanel   = lipgloss.Color("#9ece6a") // green
	cFocus   = lipgloss.Color("#bb9af7") // lavender
	cDim     = lipgloss.Color("#565f89") // muted
	cText    = lipgloss.Color("#c0caf5") // soft fg
	cSuccess = lipgloss.Color("#73daca") // teal

	styHeader = lipgloss.NewStyle().Foreground(cHeader).Bold(true)
	styAccent = lipgloss.NewStyle().Foreground(cAccent)
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

type model struct {
	panels  []*panel
	preset  int
	width   int
	focus   int // focused panel index in layout (left-to-right)
	mode    mode
	status  string
	names   []string // preset names
	presets [][]string
	saved   int
	frame   int // animation frame counter
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
		m.byName(name).on = true
	}
	m.preset = i
	m.focus = 0
}

func (m *model) byName(n string) *panel {
	for _, p := range m.panels {
		if p.name == n {
			return p
		}
	}
	return m.panels[0]
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
	case tickMsg:
		m.frame++
		return m, tickCmd()
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			if m.mode == modeLayout {
				act := m.active()
				if len(act) > 0 {
					m.focus = (m.focus + 1) % len(act)
				}
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
				if m.width > 160 {
					m.width = 60
				}
				m.status = fmt.Sprintf("width %d", m.width)
			}
		case "S":
			if m.mode == modeLayout {
				m.status = m.save()
			}
		case "enter", " ":
			if m.mode == modeToggles {
				for i, p := range m.panels {
					if p.focus {
						p.on = !p.on
						m.status = fmt.Sprintf("%s %s", p.name, onOff(p.on))
						_ = i
					}
				}
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
			m.status = "toggles: ↑↓ move, space toggle, tab back"
		default:
			// single-letter panel toggles work in both modes
			if len(msg.String()) == 1 {
				for _, p := range m.panels {
					if p.key == msg.String() {
						p.on = !p.on
						m.status = fmt.Sprintf("%s %s", p.name, onOff(p.on))
					}
				}
			}
		}
	}
	return m, nil
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

func (m *model) save() string {
	dir := "/tmp/mf"
	os.MkdirAll(dir, 0755)
	// find next free layout number
	n := 1
	for {
		if _, err := os.Stat(filepath.Join(dir, fmt.Sprintf("layout-%02d.txt", n))); err != nil {
			break
		}
		n++
	}
	if n > 99 {
		return "no free layout slot"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "flocode mock layout — control panel export %d\n", n)
	b.WriteString(strings.Join(m.render(), "\n"))
	b.WriteString("\n")
	path := filepath.Join(dir, fmt.Sprintf("layout-%02d.txt", n))
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		return "save failed: " + err.Error()
	}
	m.saved++
	return "saved " + path
}

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
	top := "┌" + title + strings.Repeat("─", max(0, p.w-2-len(title))) + "┐"
	lines := []string{top}
	for i := 1; i < p.h-1; i++ {
		lines = append(lines, body.Render("│"+shimmerFill(p.fill, p.w-2, frame)+"│"))
	}
	lines = append(lines, styDim.Render("└"+strings.Repeat("─", p.w-2)+"┘"))
	return lines
}

func center(s string, w int) string {
	if len(s) >= w {
		return s[:w]
	}
	l := (w - len(s)) / 2
	return strings.Repeat(" ", l) + s + strings.Repeat(" ", w-l-len(s))
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
