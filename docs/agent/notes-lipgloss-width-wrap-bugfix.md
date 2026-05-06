# Bug Fix: Preview Mode List Rows Word-Wrapping

## Symptom

Pressing Space to toggle preview mode caused every list row to break into multiple lines.
The right-side preview pane was invisible. The layout looked completely broken.

## Root Cause

`lipgloss.NewStyle().Width(N).Render(s)` is a **layout constraint, not a truncator**.
When content `s` is wider than `N` display columns, lipgloss **word-wraps** it onto
multiple lines — it does NOT clip or truncate.

In preview mode the list column narrows to `lw = width*6/10` (63 cols on a 105-col
terminal). `renderRow` was still generating ~87-col rows (prefix + time + sep + title(40)
+ sep + id(12) + sep + msg(6) + sep + dir). `View()` wrapped this in `Width(lw)`, which
triggered word-wrap on every row.

## Fix

`picker/model.go` — two changes:

1. **`listTitleWidth()`**: returns a dynamically computed title column width in
   `stateListPreview` — `lw - fixedCols` (where fixedCols = 45 = prefix + time + 3×sep
   + id + msg). Falls back to `titleColWidth` (40) in list-only mode.

2. **`renderRow`**: uses `listTitleWidth()` instead of the fixed `titleColWidth`; omits
   the dir column in `stateListPreview` (dir is already shown in the SESSION INFO pane).

## Lesson — ⚠️ lipgloss Width(N) wraps, it does NOT clip

```go
// WRONG — content longer than N will wrap onto multiple lines:
style := lipgloss.NewStyle().Width(N)
style.Render(longString)

// CORRECT — pre-truncate before handing to lipgloss:
style := lipgloss.NewStyle().Width(N)
style.Render(display.TruncateWidth(longString, N, "…"))
```

This is especially dangerous when `N` is **dynamic** (derived from terminal width at
runtime). A value of `N` that looks fine in a wide terminal silently breaks at narrower
widths or when the layout changes (e.g., preview pane splitting the horizontal space).

Rule: any time you write `lipgloss.NewStyle().Width(N)` where `N` can vary, ask:
"is the content guaranteed to be ≤ N columns?" If not, add a `TruncateWidth` call first.
