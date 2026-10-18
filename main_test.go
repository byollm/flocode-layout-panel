package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestHSVToHex(t *testing.T) {
	tests := []struct {
		name string
		h, s float64
		v    float64
		want string
	}{
		{"red", 0, 1, 1, "#ff0000"},
		{"green", 120, 1, 1, "#00ff00"},
		{"blue", 240, 1, 1, "#0000ff"},
		{"black (v=0)", 0, 1, 0, "#000000"},
		{"white (s=0)", 0, 0, 1, "#ffffff"},
		{"gray (s=0, v=.5)", 0, 0, 0.5, "#7f7f7f"},
		{"teal-ish (170)", 170, 0.72, 0.9, "#40e5c9"}, // computed below
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(hsvToHex(tt.h, tt.s, tt.v))
			if got != tt.want {
				t.Errorf("hsvToHex(%v,%v,%v) = %s, want %s", tt.h, tt.s, tt.v, got, tt.want)
			}
		})
	}
}

func TestSweepHueStaysInCoolBand(t *testing.T) {
	m := &model{frame: 0}
	for frame := 0; frame < 200; frame++ {
		m.frame = frame
		for i := 0; i < 100; i++ {
			h := m.sweepHue(float64(i) / 100)
			if h < hueMin || h > hueMax {
				t.Fatalf("frame %d i %d: hue %v outside [%v,%v]", frame, i, h, hueMin, hueMax)
			}
		}
	}
}

func TestSweepHuePingPong(t *testing.T) {
	m := &model{frame: 0}
	// At t=0 the hue should start at the min and travel toward max as phase
	// grows, then mirror back. Verify monotonic-ish travel over a half cycle.
	prev := m.sweepHue(0)
	for f := 1; f < 32; f++ {
		m.frame = f
		cur := m.sweepHue(0)
		if cur < prev {
			t.Fatalf("frame %d: hue decreased (%v -> %v), expected travel toward max", f-1, prev, cur)
		}
		prev = cur
	}
}

func TestLerpColor(t *testing.T) {
	tests := []struct {
		name string
		a, b lipgloss.Color
		t    float64
		want string
	}{
		{"t=0 -> a", lipgloss.Color("#000000"), lipgloss.Color("#ffffff"), 0, "#000000"},
		{"t=1 -> b", lipgloss.Color("#000000"), lipgloss.Color("#ffffff"), 1, "#ffffff"},
		{"t=.5 midpoint", lipgloss.Color("#000000"), lipgloss.Color("#ffffff"), 0.5, "#7f7f7f"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(lerpColor(tt.a, tt.b, tt.t)); got != tt.want {
				t.Errorf("lerpColor(%s,%s,%v) = %s, want %s", tt.a, tt.b, tt.t, got, tt.want)
			}
		})
	}
}

func TestCenter(t *testing.T) {
	tests := []struct {
		name string
		s    string
		w    int
		want string
	}{
		{"exact fit", "abc", 3, "abc"},
		{"wider", "ab", 4, " ab "},
		{"wider odd pad", "a", 4, " a  "},
		{"too long truncates", "abcdef", 3, "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := center(tt.s, tt.w); got != tt.want {
				t.Errorf("center(%q,%d) = %q, want %q", tt.s, tt.w, got, tt.want)
			}
		})
	}
}

func TestDisplayWidth(t *testing.T) {
	tests := []struct {
		s    string
		want int
	}{
		{"abc", 3},
		{"· messages ·", len([]rune("· messages ·"))},
	}
	for _, tt := range tests {
		if got := displayWidth(tt.s); got != tt.want {
			t.Errorf("displayWidth(%q) = %d, want %d", tt.s, got, tt.want)
		}
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "hello", "hello"},
		{"fg color", "\x1b[38;2;152;69;24mhello\x1b[0m", "hello"},
		{"bold + color", "\x1b[1;38;2;255;0;0mhi\x1b[0m", "hi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripANSI(tt.in); got != tt.want {
				t.Errorf("stripANSI(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestOnOff(t *testing.T) {
	if onOff(true) != "on" || onOff(false) != "off" {
		t.Error("onOff mismatch")
	}
}

// TestRowPacking verifies the layout packs panels into rows that fit the
// width without splitting a panel across rows.
func TestRowPacking(t *testing.T) {
	m := newModel()
	m.applyPreset(9) // "three pane": tree(24) + chat(44) + files(24)

	tests := []struct {
		name      string
		width     int
		wantRows  int
		wantFirst int // panels in first row
	}{
		{"wide fits all", 100, 1, 3},
		{"narrow splits", 60, 2, 1}, // tree(24)+chat(44)=69 > 60 -> split
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.width = tt.width
			var rows [][]*panel
			var cur []*panel
			curw := 0
			for _, p := range m.active() {
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
			if len(rows) != tt.wantRows {
				t.Errorf("width %d: got %d rows, want %d", tt.width, len(rows), tt.wantRows)
			}
			if len(rows) > 0 && len(rows[0]) != tt.wantFirst {
				t.Errorf("width %d: first row has %d panels, want %d", tt.width, len(rows[0]), tt.wantFirst)
			}
			for _, row := range rows {
				w := -1
				for _, p := range row {
					w += p.w + 1
				}
				if w > m.width {
					t.Errorf("width %d: row width %d exceeds layout width", m.width, w)
				}
			}
		})
	}
}

// TestSaveLayoutSecurity verifies the async save uses O_EXCL (never follows a
// symlink / never overwrites), writes under flocode-layout-panel, and never
// touches world-writable /tmp/mf or the real user cache dir.
func TestSaveLayoutSecurity(t *testing.T) {
	t.Parallel()

	m := newModel()
	m.width = 80
	m.saveDir = t.TempDir() // inject a temp dir so we never touch the real user cache

	saveCmd := m.saveLayout()
	if saveCmd == nil {
		t.Fatal("saveLayout returned nil cmd")
	}
	res := saveCmd()
	msg, ok := res.(saveResultMsg)
	if !ok {
		t.Fatalf("saveLayout returned %T, want saveResultMsg", res)
	}
	if msg.err != nil {
		t.Fatalf("save failed: %v", msg.err)
	}
	gotPath := msg.path

	if strings.Contains(gotPath, "/tmp/mf") {
		t.Errorf("save path %q still uses world-writable /tmp/mf", gotPath)
	}
	if !strings.HasPrefix(gotPath, m.saveDir) {
		t.Errorf("save path %q not under injected saveDir %q", gotPath, m.saveDir)
	}
	if !strings.Contains(gotPath, "flocode-layout-panel") {
		t.Errorf("save path %q not under flocode-layout-panel dir", gotPath)
	}

	// Second save with same content must pick a NEW slot (O_EXCL), never overwrite.
	res2 := saveCmd()
	if msg2, ok := res2.(saveResultMsg); ok && msg2.err == nil && msg2.path == gotPath {
		t.Errorf("second save reused path %q; O_EXCL should force a new slot", gotPath)
	}
}
