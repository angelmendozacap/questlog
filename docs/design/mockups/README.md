# Design mockups

## Product views (system F) — the complete journey

All views share `f-base.css` (the seed of the future `@questlog/ui` tokens/components)
and follow the F system: silver frames, dual accent toggle, ES-first copy.

| # | View | File | Notes |
| --- | --- | --- | --- |
| 0 | Acceso | `v-login.html` | Keycloak-backed login/signup, pixel mascot |
| 1 | Inicio (feed) | `v-home.html` | Followed activity, annual goal, follow suggestions |
| 2 | Explorar | `v-explorar.html` | Local results + on-demand TMDB/IGDB import state |
| 3 | Detalle | `f-hybrid-plata.html` | Aggregate score, histogram, "Tu puntuación" + avatar |
| 4 | Reseña | `v-resena.html` | Full review, likes, comment thread, spoiler badge |
| 5 | Perfil público | `v-perfil.html` | Other users' profile: bio, stats, favorites, tabs |
| 6 | Lista | `v-lista.html` | Ranked public list, save/share, related lists |
| 7 | Mi diario | `f-diario-plata.html` | Private chronological log, year goal |
| 8 | Admin | `v-admin.html` | Separate app (:3001), fixed amber accent, moderation queue, user mgmt, catalog curation |

# Phase 0 — Design direction mockups (archive)

Visual directions for QuestLog, same content in all of them (media detail page:
hero, aggregate score, rating histogram, actions, reviews, trending row).
Open `index.html` for a side-by-side comparison.

**Conventions applied to every mockup** (reflecting PLAN decisions):

- All copy in **Spanish** (default locale), with an ES|EN switcher in the nav.
- The **score-reactive pixel avatar** (`sprite.js`) appears **only beside the user's own
  score** ("Tu puntuación"), never on other people's reviews. FFRK-style proportions:
  big outlined head, 16×20 grid, 2 frames per mood (cheer ≥9 / idle 6–8 / slump ≤5).

## A · Cozy Pixel JRPG (`a-pixel-jrpg.html`)

Sea of Stars / Final Fantasy. Warm, nostalgic, toy-like.

- **Palette:** night `#10122b`, panel blue `#232866→#161a44`, cream border `#e8e4d8`, gold `#f5c95c`, teal `#7de3d2`, HP green `#63d67a`, rose `#e26a5a`
- **Type:** Press Start 2P (headings, small sizes) + VT323 (body & big numbers)
- **Signature:** FF dialog-box panels with pixel-notched corners, blinking ▼/▶ cursors, starfield background, HP-bar rating histogram
- **Risk:** full pixel body text (VT323) — charming but must be validated for long reviews

## B · Kinetic JRPG (`b-kinetic-jrpg.html`)

Persona 5 / Metaphor. Loud, confident, editorial.

- **Palette:** ink `#0a0a0a`, paper `#f5f5f0`, vermilion `#e60023` (+ deep `#a80016`); halftone dot textures
- **Type:** Anton italic uppercase (display) + Barlow / Barlow Condensed
- **Signature:** giant jagged score stamp, clip-path panels with alternating tilt, staggered skew entry animation, rotated marquee ticker
- **Risk:** high visual noise — needs discipline so review *reading* stays comfortable

## C · Pixel Modern Hybrid (`c-hybrid.html`)

Modern dark product layout with pixel accents carrying the identity.

- **Palette:** bg `#0e1116`, panel `#171c24`, line `#262d38`, XP green `#9df164`, lavender `#b3a6ff`, amber `#ffd166`
- **Type:** Silkscreen (headings, sparingly) + Inter (body) + JetBrains Mono (data/labels)
- **Signature:** segmented XP-bar aggregate score, dithered + scanlined posters, pixel score pips, blinking cursor in logo
- **Risk:** could collapse into generic dark-dashboard if the pixel details are dropped — they are load-bearing

> **Accent toggle:** the user likes both green and blue, so C, E and F carry a live
> **verde ⇄ azul** toggle in the nav (`data-accent` on `<html>`, 3 CSS vars swap:
> `--accent #9df164→#7ea2ff`, `--accent-deep`, `--accent-ink`). The standalone C2 file
> was removed in favor of the toggle.

## D · HD-2D — Octopath Traveler II (`d-hd2d.html`)

Painterly dusk backdrops with thin gold filigree panels — now typeset with **C's font
stack** (Silkscreen headings + Inter body + JetBrains Mono data) per user request,
replacing the original serif pairing.

- **Palette:** umber `#171310`, panel `#211a13`, gold `#c9a86a`, parchment `#eadfc8`, ember `#e08d3c`, dusk blue `#2c3a52`
- **Type:** Silkscreen + Inter + JetBrains Mono (same as C)
- **Signature:** gold filigree panels with corner rivets, hero backdrop with light rays, floating ember bokeh; avatar in the "Tu puntuación" panel

## E · Mi diario (`e-diario.html`)

The user's **private** diary screen (Letterboxd-style log), built on the C layout:
chronological entries grouped by month (day, mini poster, title, platform/venue,
score pips, rewatch ↻ and review ✎ marks, "en curso" state), year-goal XP bar,
per-month bars, top genres, "Vista privada" badge, and the user avatar in the header.
Includes a live **green ⇄ blue accent toggle** to compare C vs C2 on a real screen.

## F · C/E + silver frames (`f-hybrid-plata.html`, `f-diario-plata.html`)

The C detail layout and the E diary with D's *frame structure* but recolored to match
C's cool palette (user feedback: no gold): thin steel-gray borders (`#a9b6c9` at ~30%),
faint inner frame, square corners, section markers ✦ — and the 3px corner rivets plus
the user-score panel border take the **accent color**, so they follow the green⇄blue
toggle. Generated as C/E + a single override stylesheet — one layer, easy to adopt or drop.

## Status

- User likes both green and blue → toggle instead of a fixed accent.
- Direction shortlist: **C** (flat borders) vs **F** (silver frames + accent rivets), accent to be chosen with the toggle.
- The score-reactive avatar (image reference: FF Record Keeper sprites) is a confirmed
  product feature, shown only beside the user's own score.

## Next steps

1. Pick: plain C borders vs F gold borders; green vs blue accent.
2. Extract final design tokens into `docs/design/tokens.md`.
3. Decide the pixel/readable font pairing for long-form review text.
