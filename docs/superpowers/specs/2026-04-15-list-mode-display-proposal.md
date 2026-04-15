# aps -l Display Mode: Design Discussion Proposal

**Date:** 2026-04-15  
**Status:** Open for decision  
**Author:** Claude Code  

---

## What is aps

`aps` (AI Picker for Sessions) is a Go CLI tool that lists and launches saved Claude Code / Opencode sessions. It replaces a bash+fzf script with a pure Go binary.

Key modes:
- `aps` — opens an interactive full-screen TUI picker (bubbletea + alt-screen); user selects a session and the tool `exec`s into the client.
- `aps -l` — prints a plain table of all sessions to stdout and exits. No interaction, no launching.
- `aps -n` — dry-run; shows what would be launched without launching.

The table printed by `aps -l` has five columns: TIME, TITLE, ID, MSG count, DIRECTORY.

---

## Background

`aps -l` currently prints a fixed-width table to stdout and exits immediately (one-shot output). It has no knowledge of terminal width, so long rows wrap on narrow terminals, making the output unreadable.

This document explores three design directions and asks the reader to decide which fits their mental model of the tool.

---

## The Core Technical Constraint

Before evaluating options, one hard physical constraint must be understood:

> **Terminal scrollback content cannot be redrawn.**

When a TUI program renders inline (without alt-screen), it redraws by issuing `CursorUp(N)` ANSI sequences to move back to the start of its output, then overwriting in place. This only works while all N lines remain in the **visible viewport**. Lines that have scrolled into the scrollback buffer are unreachable — no program can move the cursor there.

This constraint is universal. It explains every trade-off below.

**Implication:** A program that wants live resize-redraw must guarantee it never exceeds the terminal height. fzf solves this with `--height 40%` — it self-limits so it always fits in the viewport.

---

## The Problem Being Solved

1. **Row wrapping on narrow terminals** — columns exceed terminal width, rows spill to next line, table structure collapses.
2. **Wide rows can't be seen fully** — even on a wide terminal, DIRECTORY column may be cut off.

---

## Three Design Options

---

### Option A: One-Shot, Width-Aware Output

**What it feels like:** Exactly like `ls` or `git log --oneline`. Prints, exits, stays in scrollback.

**Behavior:**
- On startup, read terminal width once via `ioctl TIOCGWINSZ` (one syscall).
- If stdout is not a TTY (pipe, redirect), skip width detection and print full-width rows as today.
- Truncate rows to fit terminal width. Column priority for truncation:
  1. DIRECTORY truncated first (rightmost)
  2. TITLE truncated second (down to a minimum of ~20 cols)
  3. ID shortened to 8-char prefix if still too tight
- Print header + all rows, exit.
- No resize response. If user resizes after printing, the output in scrollback stays as-is (same as any shell command).

**Technical implementation:**
- `golang.org/x/term.GetSize(int(os.Stdout.Fd()))` — already a transitive dependency.
- No bubbletea involvement. ~30 lines added to `display/format.go`.
- Pipe-safe by design (TTY check gates the width query).

**Pros:**
- Zero UX friction — behaves exactly like every other Unix list command.
- Output is pipe-friendly (clean text, no ANSI control sequences when piped).
- Trivially composable: `aps -l | grep foo`, `aps -l | fzf`, `aps -l > file`.
- No "exit to return to shell" ceremony.

**Cons:**
- Static. If user resizes terminal after the output, the layout is not updated (it's in scrollback).
- Wide content that doesn't fit is silently truncated; user cannot reveal it without re-running.

**Verdict:** Solves the original problem (wrapping). Minimal complexity. Trade-off: no post-print adaptability.

---

### Option B: Inline Interactive Viewer (fzf-style)

**What it feels like:** Like `fzf --height 40%`. A live table occupies the bottom N rows of the terminal. Resizing reflows the columns. Left/right arrow keys pan horizontally to reveal truncated content. `q` or `Ctrl-C` exits and returns to shell.

**Behavior:**
- Detect TTY; if not TTY, fall back to Option A (one-shot plain text).
- Occupy `min(rows + 1, terminal_height - 1)` rows — **always fits in viewport**.
- On `WindowSizeMsg` (SIGWINCH): recalculate column widths, rerender in place via `CursorUp`.
- On `←`/`→`: shift a horizontal viewport offset, re-render visible columns.
- On `q`/`Enter`/`Ctrl-C`: exit, cursor lands below the table (output stays visible in terminal).
- No scrollback clearing. No flicker. Because height is always ≤ terminal height, `CursorUp` always reaches line 0.

**Rows exceeding terminal height:**
- Only `terminal_height - 1` rows are shown at a time.
- `j`/`k` or `↑`/`↓` scroll the row viewport (like fzf).
- This means `-l` gains a scroll interaction, which some users may find surprising for a "list" command.

**Technical implementation:**
- New bubbletea model in `display/` or a new `listview/` package (~200 lines).
- TTY detection gates the two paths.
- Existing `display.FormatListRow` reused for cell content; bubbletea handles layout and input.
- Horizontal panning: track `xOffset int` in model; `View()` slices each rendered row string by display columns starting at `xOffset`.

**Pros:**
- Fully adapts to terminal width at all times.
- Horizontal panning reveals content that doesn't fit.
- Resize always works cleanly (self-limited height).

**Cons:**
- User must press `q` to exit — breaks the "just a list command" mental model.
- Adds interactive dependency to a non-interactive workflow.
- Slightly more complex code path; two behaviors (TTY vs pipe) to maintain.
- Scroll interaction on a "list" command may feel surprising.

**Verdict:** Best UX for terminal-only use. Wrong UX if the user expects `aps -l` to behave like `ls`.

---

### Option C: Two Modes — Keep -l as One-Shot, Add New -i Flag

**What it feels like:** `-l` stays exactly as in Option A (one-shot, width-aware). A new `-i` flag (or `--interactive`) opens the inline viewer from Option B.

**Behavior:**
- `aps -l` → Option A (one-shot, width-aware, pipe-safe).
- `aps -i` (or `aps --interactive`) → Option B (inline TUI, height-limited, resize-aware, horizontal pan).
- Both share the same session data and `FormatListRow` rendering logic.

**Technical implementation:**
- Option A: ~30 lines in `display/`.
- Option B: ~200 lines new package.
- `cmd/` adds the new flag.
- Total: moderate addition, clean separation.

**Pros:**
- Each mode has a coherent mental model — no compromises.
- `-l` remains fully composable and script-safe.
- `-i` gives the interactive experience when wanted.
- Users who want fzf-style browsing can use `-i`; users scripting use `-l`.

**Cons:**
- Two features to implement and maintain instead of one.
- Risk of `-i` being redundant with the existing interactive picker (`aps` without flags, which already lets you pick a session). The difference would be: `-i` is read-only browsing, `aps` launches a session.
- Adds surface area to the CLI.

**Verdict:** Cleanest separation of concerns, but only justified if there is genuine demand for both behaviors.

---

## Comparison Table

| | Option A | Option B | Option C |
|---|---|---|---|
| Solves wrapping | Yes | Yes | Yes (both) |
| Pipe-safe | Yes | Fallback only | Yes (-l) |
| Resize-aware | No | Yes | Yes (-i) |
| Horizontal pan | No | Yes | Yes (-i) |
| Requires `q` to exit | No | Yes | -l: No / -i: Yes |
| Scroll through long lists | Scrollback | j/k in viewer | -l: Scrollback / -i: j/k |
| Implementation effort | Small | Medium | Medium-Large |
| New dependency | No | No (bubbletea already used) | No |

---

## The Deciding Question

The answer depends on how `aps -l` is primarily used:

**"I run `aps -l` to quickly scan sessions, sometimes pipe it."**
→ Option A. One-shot width-aware is the right tool.

**"I want to browse the list interactively in the terminal without launching a session."**
→ Option B. The inline viewer is the right tool; rename or repurpose `-l` as needed.

**"Both — I script with `-l` and browse with something else."**
→ Option C. Keep `-l` clean, add `-i` for browsing.

---

## Recommendation

Start with **Option A**. It directly solves the reported problem (wrapping on narrow terminals) with minimal complexity and no behavioral change. Option B or C can be layered on top later if interactive browsing turns out to be a real need.

The risk of skipping to Option B is introducing interactive ceremony (`q` to exit) into a workflow that currently has none, solving a problem (resize-redraw) that the user may not actually care about once wrapping is fixed.
