# Claude Code References

Source: `~/workspace/cc/extracted/src/` (decompiled from CC 2.1.88)

## Spinner

### Animation timing

| Path | Value |
|------|-------|
| Animation clock | 50ms tick (20fps) |
| Normal spinner frame advance | every 120ms (`Math.floor(time / 120)`) |
| Reduced-motion cycle | 2000ms total (1s visible, 1s dim) |

### Characters

Normal spinner (`getDefaultCharacters()` palindrome):
```
· ✢ ✳ ✶ ✻ ✽  →  ✽ ✻ ✶ ✳ ✢ ·
```
Full `SPINNER_FRAMES = [...DEFAULT_CHARACTERS, ...[...DEFAULT_CHARACTERS].reverse()]`

Reduced-motion glyph: `●` (slow flash, not a sequence)

### Color

Default `messageColor`: `'claude'` → `rgb(215,119,87)` (Claude orange)

Theme key → hex mapping (from `src/utils/theme.js`):
- `claude`: `rgb(215,119,87)` — Claude orange (normal spinner)
- `claudeShimmer`: `rgb(245,149,117)` — lighter orange (shimmer effect)
- `claudeBlue_FOR_SYSTEM_SPINNER`: `rgb(87,105,247)` — medium blue (system spinner)
- `error`: `rgb(171,43,63)` — red (stalled spinner interpolates toward this)

ANSI 16-color approximation used in aps: `"9"` (bright red, renders as orange in most terminals)

### Spinner modes (`SpinnerMode`)

| Mode | Trigger | Mode glyph (beside spinner) | Visual behavior |
|------|---------|----------------------------|-----------------|
| `responding` | Streaming response text | `↓` (dimmed) | Normal spinner + glimmer sweep (200ms/cycle) |
| `tool-use` | Tool call executing | `↓` (dimmed) | Normal spinner + flash effect (sin wave 1s period) |
| `tool-input` | Tool input being written | `↓` (dimmed) | Same as tool-use |
| `thinking` | Extended thinking active | `↓` (dimmed) | Normal spinner + thinking shimmer text |
| `requesting` | Sending request to API | `↑` (dimmed) | Fast glimmer sweep (50ms/cycle, left→right) |

Initial state on mount: `'responding'`

Stall detection: if response length stops growing for ~3s, `stalledIntensity` ramps 0→1, interpolating spinner color from `claude` orange toward `error` red.

### Source files

- `src/components/Spinner.tsx` — top-level component, sets `messageColor = 'claude'`
- `src/components/Spinner/SpinnerGlyph.tsx` — renders the glyph, handles stalled interpolation
- `src/components/Spinner/SpinnerAnimationRow.tsx` — owns the 50ms animation clock, computes `frame`
- `src/components/Spinner/utils.ts` — `getDefaultCharacters()`, color interpolation helpers
