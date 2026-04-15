# Bug Report: `Width` padding guarantee violated when combined with `MaxWidth` and CJK text

**Library:** `github.com/charmbracelet/lipgloss` — verified on v1.1.0 and v2.0.3
**Filed against:** `Style.Render()` in `style.go`

---

## Background

We're building a terminal TUI table with a fixed-width title column that may contain CJK characters (2 cells per rune). The natural way to express "exactly N columns" in lipgloss is:

```go
style := lipgloss.NewStyle().
    Width(40).     // pad to exactly 40 columns
    MaxWidth(40).  // truncate if longer than 40 columns
    Inline(true)   // single-line: no word-wrap
```

| Option | Contract |
|--------|----------|
| `Width(N)` | Output is exactly N columns (pad short lines with spaces) |
| `MaxWidth(N)` | Output is at most N columns (truncate long lines) |
| `Inline(true)` | Suppress word-wrap; strip embedded newlines |

With both `Width` and `MaxWidth` set to the same value, the expected result is always exactly N columns.

---

## Bug: output is N−1 columns for CJK text at a truncation boundary

When the input exceeds the column limit and the truncation boundary falls on a CJK character (which occupies 2 cells), `ansi.Truncate` drops that character to stay within the limit, producing a string of width **N−1 columns**. The `Width(N)` padding that should compensate runs *before* `MaxWidth` truncation in `Render()`, so it never sees the short result. The final output is N−1 columns, and all subsequent columns in the row are misaligned by one cell.

### Affected conditions

The bug requires all three of:

1. `Width(N)` — padding target is set
2. `MaxWidth(M)` where M ≤ N — a truncation limit is set
3. Input text wider than M columns, where the total width of characters before the truncation point equals `M − 1` and the next character is CJK (2 cols wide) — exactly 1 column of space remains, `ansi.Truncate` cannot fit the 2-col character and drops it entirely, producing `M − 1` columns

`Inline(true)` is the common trigger in practice (it disables word-wrap, making `MaxWidth` the sole truncation mechanism), but the root cause is the execution order in `Render()`, not `Inline` itself: `alignTextHorizontal` runs **before** `ansi.Truncate`, so padding can never compensate for a post-truncation width shortfall.

---

## Minimal reproduction

```go
package main

import (
    "fmt"
    "github.com/charmbracelet/lipgloss"
    "github.com/charmbracelet/x/ansi"
)

func main() {
    style := lipgloss.NewStyle().Width(10).MaxWidth(10).Inline(true)

    cases := []struct {
        s    string
        desc string
    }{
        {"AAAAAAAAAAA", "11 ASCII — truncated at ASCII boundary → 10 cols (OK)"},
        {"AAAAAAAAA中", "9 ASCII + CJK(2cols) = 11 — only 1 col remains at boundary, CJK needs 2, dropped → 9 cols (BUG)"},
        {"AAAAAAAA中B", "8 ASCII + CJK(2cols) + ASCII = 11 — CJK fits col 9-10, B dropped → 10 cols (OK)"},
        {"中中中中A中",  "4CJK+A+CJK = 11 — only 1 col remains at boundary, last CJK needs 2, dropped → 9 cols (BUG)"},
    }

    for _, c := range cases {
        out := style.Render(c.s)
        w := ansi.StringWidth(out)
        status := "OK "
        if w != 10 {
            status = "BUG"
        }
        fmt.Printf("[%s] width=%d input=%-14q %s\n", status, w, c.s, c.desc)
    }
}
```

Expected: all lines print `width=10`.
Actual (verified on v1.1.0 and v2.0.3):
```
[OK ] width=10 input="AAAAAAAAAAA"   11 ASCII — truncated at ASCII boundary → 10 cols (OK)
[BUG] width=9  input="AAAAAAAAA中"   9 ASCII + CJK(2cols) = 11 — only 1 col remains at boundary, CJK needs 2, dropped → 9 cols (BUG)
[OK ] width=10 input="AAAAAAAA中B"   8 ASCII + CJK(2cols) + ASCII = 11 — CJK fits col 9-10, B dropped → 10 cols (OK)
[BUG] width=9  input="中中中中A中"    4CJK+A+CJK = 11 — only 1 col remains at boundary, last CJK needs 2, dropped → 9 cols (BUG)
```

The bug triggers when **exactly 1 column of space remains at the truncation boundary and the next character is CJK (2 cols wide)**. `ansi.Truncate` cannot fit it and drops the whole character, leaving output 1 column short. This occurs whenever the total width of preceding characters equals `maxWidth − 1`. If the remaining space is ≥ 2 (or the next character is ASCII), truncation produces exactly `maxWidth` columns and no bug occurs.

---

## Root cause: execution order in `Render()`

Relevant excerpt from `style.go` (lipgloss v1.1.0, lines ~365–458):

```
1. cellbuf.Wrap(str, width)          // word-wrap to Width columns  [skipped if Inline]
2. alignTextHorizontal(str, width)   // pad short lines to Width     ← runs here
3. applyBorder / applyMargins
4. ansi.Truncate(str, maxWidth)      // truncate to MaxWidth         ← runs here
5. (nothing re-pads after step 4)
```

In normal (non-Inline) mode, step 1 guarantees every line is ≤ `width` columns before step 2 pads them, so step 4 is a no-op when `maxWidth == width`. The system works.

With `Inline(true)`, step 1 is skipped. Step 2 receives the full-length input string, finds `shortAmount = 0` (input is already wider than `width`), and does not pad. Step 4 truncates — potentially to `maxWidth − 1` at a CJK boundary. No subsequent step re-applies padding. The `Width(N)` contract is silently broken.

This is not unique to `Inline`: any configuration where `width > maxWidth` in non-Inline mode triggers the same path (Wrap folds to `width` columns, step 2 pads to `width`, step 4 then cuts to `maxWidth`, leaving a potentially N-1-column result).

---

## Proposed fix

After the `MaxWidth` truncation block in `Render()`, re-apply `alignTextHorizontal` with a target of `min(width, maxWidth)` when `Width` is also set.

```go
// Truncate according to MaxWidth
if maxWidth > 0 {
    lines := strings.Split(str, "\n")
    for i := range lines {
        lines[i] = ansi.Truncate(lines[i], maxWidth, "")
    }
    str = strings.Join(lines, "\n")

    // Re-apply Width padding after truncation.
    // ansi.Truncate may produce a string shorter than maxWidth (e.g. when a
    // wide CJK character straddles the boundary). If Width is also set, the
    // caller expects the output to be exactly min(width, maxWidth) columns;
    // the earlier alignTextHorizontal call ran before truncation and cannot
    // have compensated for this shortfall.
    if width > 0 {
        effectiveWidth := maxWidth
        if width < maxWidth {
            effectiveWidth = width
        }
        // colorWhitespace, styleWhitespace, teWhitespace: existing locals in Render();
        // mirrors the conditional at lines ~436-440.
        var st *termenv.Style
        if colorWhitespace || styleWhitespace {
            st = &teWhitespace
        }
        str = alignTextHorizontal(str, horizontalAlign, effectiveWidth, st)
    }
}
```

### Why this fix is correct

- `MaxWidth` alone (no `Width`): `width == 0`, condition is false, behaviour unchanged.
- `Width` alone (no `MaxWidth`): block is never entered, behaviour unchanged.
- Non-Inline `Width + MaxWidth`, `width == maxWidth`: Wrap already ensures lines ≤ width, truncation is a no-op, re-alignment is a no-op. No regression.
- Non-Inline `Width > maxWidth`: truncation now shrinks lines below `width`; the re-pad brings them to `maxWidth` (the effective ceiling). This is the correct behaviour and fixes the same latent bug in this configuration too.
- Inline `Width + MaxWidth`: the primary reported case. Re-padding after truncation restores the `Width` contract.

### Alternative considered: pre-truncate before padding

Moving `MaxWidth` before `alignTextHorizontal` would also work for the simple case, but `MaxWidth` runs after `applyBorder`/`applyMargins` which contribute to the measured width. Truncating before borders would cut the content too aggressively. The post-truncation re-pad is the safer, more local change.

---

## Workaround (until fix is released)

Pre-truncate the string before passing it to lipgloss, so `MaxWidth` never encounters a CJK boundary:

```go
// TruncateWidth truncates s to at most maxCols display columns, CJK-aware.
func TruncateWidth(s string, maxCols int) string {
    runes := []rune(s)
    for len(runes) > 0 && lipgloss.Width(string(runes)) > maxCols {
        runes = runes[:len(runes)-1]
    }
    return string(runes)
}

style := lipgloss.NewStyle().Width(40).Inline(true) // no MaxWidth needed
output := style.Render(TruncateWidth(input, 40))
```

A pure-lipgloss alternative: two passes, truncate then pad:

```go
truncStyle := lipgloss.NewStyle().MaxWidth(40).Inline(true)
padStyle   := lipgloss.NewStyle().Foreground(...).Width(40).Inline(true)
output := padStyle.Render(truncStyle.Render(input))
```
