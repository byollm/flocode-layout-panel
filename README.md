# flocode-layout-panel

Interactive Bubble Tea control panel for composing mock FloCode TUI layouts —
with a captivating animated header.

## Run

    go run .

## Keys

- `tab` — cycle panel focus
- `P` / `N` — previous / next preset (20 presets)
- `w` — cycle layout width
- `T` — panel toggle list (space toggles, arrows move)
- single letters — toggle individual panels (shown in the toggle list)
- `S` — export the current layout to a per-user cache dir
  (`~/Library/Caches/flocode-layout-panel/layout-NN.txt` on macOS)
- `q` — quit

## Animation

- **Traveling gradient header** — continuous HSV hue sweep (170°–290°,
  teal→blue→lavender, no orange), mirrored ping-pong travel (~3.5s/cycle)
- **Breathing focus panel** — ~2s sine pulse on the focused panel border
- **Shimmer sweep** — periodic highlight sweep across panels
- Runs at 18fps via `tea.Tick`

## Palette

Default color scheme: cool blues/teals/lavender (Tokyo-Night-inspired).
No orange.

## Save

`S` writes a plain-text layout export (no ANSI escapes) to
`~/Library/Caches/flocode-layout-panel/layout-NN.txt` using `O_EXCL`, so it
never follows a symlink or overwrites an existing file.
