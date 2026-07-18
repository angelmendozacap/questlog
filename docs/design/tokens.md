# QuestLog — Design tokens (Phase 0 final)

**Chosen direction: F** — the pixel-modern hybrid layout (direction C) with subtle
silver frames, where only the user's own panel gets the full frame treatment.
Reference mockups: `docs/design/mockups/f-hybrid-plata.html` (detail) and
`f-diario-plata.html` (diary). Sprite implementation: `docs/design/mockups/sprite.js`.

## Color

### Base (fixed)

| Token | Value | Use |
| --- | --- | --- |
| `--bg` | `#0e1116` | Page background |
| `--panel` | `#171c24` | Panel background |
| `--panel-2` | `#1d232e` | Inset elements (empty pips/segments) |
| `--line` | `#262d38` | Legacy hairline (inputs, chips) |
| `--text` | `#e6e9ee` | Primary text |
| `--muted` | `#8a93a3` | Secondary text |
| `--lav` | `#b3a6ff` | Usernames, links to titles, secondary data accent |
| `--amber` | `#ffd166` | Likes, warnings, "en curso"/private badges, media-type badge |

### Frames (F treatment)

| Token | Value | Use |
| --- | --- | --- |
| `--frame-dim` | `#a9b6c91f` (~12%) | Single border on regular panels — no inner frame, no rivets |
| `--frame-faint` | `#a9b6c912` (~7%) | Minor dividers (feed rows, footer) and the signature panel's inner frame |

Rules: square corners everywhere (`border-radius: 0`), 1px borders only.
**Only the user's own panel ("Tu puntuación")** gets the full signature treatment:
accent-colored border at 55% + 3px accent corner rivets + faint inner frame (inset 3px).

### Accent theme (user-switchable)

Both accents ship; the user toggles them (two color swatches in the nav — active
outlined, inactive dimmed to 45%). Mechanism: `data-accent="green|blue"` on `<html>`,
swapping exactly three variables:

| Token | Green (default) | Blue (FF) |
| --- | --- | --- |
| `--accent` | `#9df164` | `#7ea2ff` |
| `--accent-deep` | `#6fbf3e` | `#4a6fd4` |
| `--accent-ink` | `#10230a` | `#0b1430` |

Accent drives: primary buttons, active nav item, XP bar fill, filled pips, logo cursor,
section markers (✦), stat numbers, signature-panel border and rivets.

## Typography

| Role | Face | Notes |
| --- | --- | --- |
| Display / headings | **Silkscreen** (400/700) | Sparingly: h1–h2, big numbers, logo, badges. Small sizes (9–15px) except hero |
| Body | **Inter** (400/500/600) | All running text, review bodies — long-form must stay readable |
| Data / labels | **JetBrains Mono** (400/500) | Usernames, dates, counts, kickers, filters |

Section headings: Silkscreen 14–15px prefixed by an accent `✦` (F) marker.

## Signature components

- **XP bar**: segmented (10 segments, 3px gap), filled = accent with `inset 0 -4px`
  shade of `--accent-deep`; partial segment via gradient stop. Used for aggregate
  score and annual goal.
- **Score pips**: 8–9px squares, filled = accent. Used for per-title user scores.
- **Score-reactive pixel avatar** (`sprite.js`): FFRK proportions, 16×20 grid,
  box-shadow rendering, dark outline `#2b2320`, fixed palette (independent of theme).
  2 frames per mood at 430ms, `steps()` easing. Moods: **cheer** (score ≥9, +CSS hop),
  **idle** (6–8, breathe), **sad** (≤5, sway + desaturate + tear frame).
  Shown **only beside the user's own score** — never on other users' content.
- **Posters**: gradient placeholder + dither overlay (`repeating-conic-gradient`,
  4–5px cell) + scanlines (4px period). Frame anchored to poster height (`align-self: start`).

## Motion

- Pixel-feel timing: `steps(1|2)` for blink/hop/breathe; no easing curves on sprites.
- Page-level animation minimal; `prefers-reduced-motion` disables all animation.

## Language

- Default locale **es**; UI copy written ES-first, EN via catalogs (next-intl).
- ES|EN switcher lives in the nav next to the accent swatches.
