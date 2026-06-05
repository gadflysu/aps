# Plan: Cache listTitleWidth to eliminate per-frame O(N×V) recomputation

Issue: #49

## Goal

Remove the per-frame O(N×V) `lipgloss.Width` loop inside `listTitleWidth()` by
caching the two natural-maximum widths that drive the title-column bonus
calculation.  No visible layout change; CPU usage during idle ticker drops
significantly.

## Root Cause

`listTitleWidth()` (`picker/model.go`) is called once per visible row inside
`renderRowFull()`, which fires on every `View()`.  In `stateList` mode the
method iterates all `m.sessions` twice:

```go
for _, s := range m.sessions {
    lipgloss.Width(display.Sanitize(s.Title))       // Unicode grapheme walk
}
for _, s := range m.sessions {
    lipgloss.Width(display.Sanitize(s.CWDDisplay))  // Unicode grapheme walk
}
```

With V=30 visible rows and N=200 sessions that is 12 000 `lipgloss.Width` calls
per 120 ms tick.  By contrast `idColW` and `msgColW` are computed once in
`newModel` and never recalculated.

## Target Files

- `picker/model.go` — all changes live here
- `picker/model_test.go` — new tests

## Non-Goals

- No change to rendered column widths or layout
- No change to `stateListPreview` path (already skips the bonus)
- No change to `scrollableWidth` (separate concern, separate issue if needed)

## Implementation

### 1. Add two cached fields to `Model` (model.go ~line 119)

```go
naturalTitleW int  // max lipgloss.Width(Sanitize(s.Title))      across m.sessions
naturalDirW   int  // max lipgloss.Width(Sanitize(s.CWDDisplay)) across m.sessions
```

Place them alongside `idColW` / `msgColW`.

### 2. Add `recomputeNaturalWidths()` helper (model.go, near adaptiveIDColW)

```go
func (m *Model) recomputeNaturalWidths() {
    maxT, maxD := 0, 0
    for _, s := range m.sessions {
        if w := lipgloss.Width(display.Sanitize(s.Title)); w > maxT {
            maxT = w
        }
        if w := lipgloss.Width(display.Sanitize(s.CWDDisplay)); w > maxD {
            maxD = w
        }
    }
    m.naturalTitleW = maxT
    m.naturalDirW   = maxD
}
```

Single pass, two max-finds.

### 3. Call `recomputeNaturalWidths()` from `newModel`

After setting `idColW` / `msgColW`:

```go
m := Model{
    ...
    idColW:  adaptiveIDColW(sessions),
    msgColW: display.AdaptiveMsgWidth(sessions),
}
m.recomputeNaturalWidths()
return m
```

Because `newModel` returns a value (not a pointer), call on a local variable
before returning:

```go
result := Model{ ... }
result.recomputeNaturalWidths()
return result
```

### 4. Call `recomputeNaturalWidths()` from `applyRefresh`

`applyRefresh` (`picker/model.go`) is the only path that modifies `m.sessions`
at runtime (new or updated sessions).  Add the call right after the
`sort.Slice` that re-sorts sessions and before `m.reguessActive()`:

```go
sort.Slice(m.sessions, ...)
m.recomputeNaturalWidths()   // ← add here
m.reguessActive()
```

### 5. Simplify `listTitleWidth()` to use cached values

Replace the two scan loops in the `stateList` branch with reads of the cached
fields:

```go
// Before (in listTitleWidth, stateList branch):
maxNaturalTitle := 0
for _, s := range m.sessions {
    if w := lipgloss.Width(display.Sanitize(s.Title)); w > maxNaturalTitle {
        maxNaturalTitle = w
    }
}
maxBonus := maxNaturalTitle - titleColWidth
if maxBonus > 0 {
    maxDirW := 0
    for _, s := range m.sessions {
        if w := lipgloss.Width(display.Sanitize(s.CWDDisplay)); w > maxDirW {
            maxDirW = w
        }
    }
    ...
}

// After:
maxBonus := m.naturalTitleW - titleColWidth
if maxBonus > 0 {
    maxDirW := m.naturalDirW
    ...
}
```

The rest of the bonus-calculation logic is unchanged.

## Tests

Add to `picker/model_test.go`:

### `TestRecomputeNaturalWidths`

Construct a `Model` with a small hand-crafted `sessions` slice containing ASCII
and multi-byte (CJK) titles/CWDs.  Call `recomputeNaturalWidths()` directly.
Assert `naturalTitleW` and `naturalDirW` equal the expected maximum widths.

### `TestListTitleWidthStableAfterRefresh`

- Build a model via `newModel` with N sessions.
- Record `listTitleWidth()` result.
- Call `applyRefresh` with a mutated copy of one session (title changed).
- Re-call `listTitleWidth()`.
- Assert the returned width is still consistent with the new maximum (i.e. the
  cache was updated, not stale).

### Existing tests

Run `go test ./picker/...` — no existing test should change behaviour.

## Verification

1. `go build .` passes
2. `go test ./picker/...` passes including new tests
3. `go vet ./...` clean
4. Manual: start `aps` against a live session directory; confirm the title
   column width is identical to pre-patch behaviour (no visual regression)
5. Optional: re-run `sample` against the patched binary and confirm
   `listTitleWidth` no longer appears in the hot path
