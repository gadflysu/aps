# Bug Report: SGR mouse sequences split at read-buffer boundaries are misrouted as `Alt+KeyRunes`

**Library:** `github.com/charmbracelet/bubbletea` — verified on v1.3.10  
**Filed against:** `detectOneMsg` in `key.go`


## Background

When `WithMouseCellMotion()` (or `WithMouseAllMotion()`) is enabled, the terminal sends mouse events as SGR sequences:

```
ESC [ < button ; col ; row M     (press)
ESC [ < button ; col ; row m     (release)
```

For a WheelDown event the full byte sequence is, for example:

```
\x1b [ < 6 5 ; 6 9 ; 1 0 M
```

(`b=65 = 0b01000001 = bitWheel(64) + WheelDown(1)`.) Sequence length varies with coordinate values — single-digit coordinates produce 11 bytes, double-digit produce 12 or more.

bubbletea parses these in `detectOneMsg` (`key.go:614`) and emits `MouseMsg` values. The bug manifests when rapid mouse events cause multiple SGR sequences to arrive in one `Read` call and the sequence parser misidentifies a subsequent `\x1b` as an Alt modifier instead of the start of a new SGR sequence.


## Bug: SGR sequences split at a read-buffer boundary are misrouted as `Alt+KeyRunes`

When the user generates rapid mouse events, multiple SGR sequences accumulate in the OS pipe buffer. A single `Read` call may return a block such as:

```
\x1b[<65;69;10M \x1b[<65;69;10M \x1b[<65;69;10M ...
```

`detectOneMsg` processes the block left-to-right, consuming one message per call. Complete sequences are correctly emitted as `MouseMsg` values. The problem occurs when the block ends with a **partial** SGR sequence. The split can fall at any byte position within the sequence:

| Remaining bytes at end of `Read` | Prefix | What `detectOneMsg` does |
|---|---|---|
| 1 (`\x1b`) | ESC only | `alt=true`, `i=1==len(b)`, existing line-686 guard fires → **`w=0` ✓ already handled** |
| 2 (`\x1b[`) | ESC+`[` | `alt=true`, reads `[`, `i=2==len(b)`, line-686 guard fires → **`w=0` ✓ already handled** |
| 3–5 (`\x1b[<`, …) | `len(b)<6`, has `\x1b[<` | `alt=true`, reads `[`, `i=2 < len(b)` → guard does NOT fire → **`KeyMsg{Alt:true, Runes:['[']}` ← BUG** |
| 6–11+ (`\x1b[<65`, …) | `len(b)>=6`, enters `case '<'` | Regex fails (incomplete payload), falls through to alt path → **same misroute ← BUG** |

After a misrouted `Alt+[` message is emitted, the **next `Read`** starts from the remainder (`<65;69;10M\x1b[<65;...`). No `\x1b` prefix → rune loop collects all printable bytes up to the next `\x1b`:

```go
KeyMsg{Type: KeyRunes, Runes: ['<','6','5',';','6','9',';','1','0','M']}
```

This cycle repeats for every remaining SGR sequence. The application receives alternating `Alt+[` and bare `<65;69;10M` `KeyRunes` messages instead of `MouseMsg` values.


## Affected conditions

1. `WithMouseCellMotion()` or `WithMouseAllMotion()` is enabled.
2. Mouse events arrive fast enough to fill the 256-byte `readAnsiInputs` buffer — causing `canHaveMoreData = true` — before the parser has consumed them all.
3. The buffer boundary happens to fall inside an SGR sequence rather than between two sequences.

Conditions 2 and 3 are both probabilistic but compound quickly under sustained fast input. A macOS trackpad generating continuous scroll events, or any smooth-scrolling driver that synthesises many events per physical tick, makes this nearly certain to trigger within seconds. Plain fast mouse-wheel scrolling is sufficient on most systems; no special hardware or software is required.


## Minimal reproduction

Self-contained — requires only `bubbletea v1.3.10` and Go 1.21+:

```sh
mkdir bt-sgr-repro && cd bt-sgr-repro
go mod init btrepro
go get github.com/charmbracelet/bubbletea@v1.3.10
# paste main.go below, then:
go run .
```

```go
package main

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
)

type model struct{}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyRunes {
			fmt.Printf("UNEXPECTED KeyRunes: Alt=%-5v Runes=%q\n", msg.Alt, string(msg.Runes))
		}
		if msg.Type == tea.KeyEsc || msg.String() == "q" {
			return m, tea.Quit
		}
	case tea.MouseMsg:
		// expected — ignore
	}
	return m, nil
}

func (m model) View() string { return "Scroll fast. Press q/Esc to quit.\n" }

func main() {
	p := tea.NewProgram(model{}, tea.WithMouseCellMotion())
	p.Run()
}
```

**Expected:** no output while scrolling.  
**Actual:** with rapid scrolling, `UNEXPECTED KeyRunes` lines appear directly in the terminal:

```
UNEXPECTED KeyRunes: Alt=false Runes="<65;69;10M"
UNEXPECTED KeyRunes: Alt=true  Runes="["
UNEXPECTED KeyRunes: Alt=false Runes="<65;69;10M"
UNEXPECTED KeyRunes: Alt=true  Runes="["
UNEXPECTED KeyRunes: Alt=false Runes="<65;69;10M"
```

The first line (`Alt=false`) is the payload tail of a split that occurred before the capture window — its preceding `\x1b[` was already emitted as `Alt=true Runes="["` in an earlier message. From then on the pattern alternates: `Alt+[` then bare payload.

In applications with a text input widget, these fragments are inserted into the input buffer, producing visible garbage like `[<65;69;10M[<65;69;10M[<65;69;10M`.
In applications without text input, mouse events are silently dropped (the scroll has no effect) — no visible corruption, but functionality is degraded.


## Root cause: missing `w=0` fallback for partial SGR sequences

`detectOneMsg` returns `w=0` to signal "I need more bytes; buffer this and retry." The bracketed-paste detector (`key_sequences.go:98–102`) uses this correctly:

```go
if idx == -1 {
    // Tell the outer loop we have done a short read and we want more.
    return true, 0, nil   // ← w=0 triggers leftOverFromPrevIteration
}
```

The SGR mouse detector does not. When `b` starts with `\x1b[<` but the regex `(\d+);(\d+);(\d+)([Mm])` fails to match (because the sequence is incomplete), the `case '<':` block simply falls through:

```go
case '<':
    if matchIndices := mouseSGRRegex.FindSubmatchIndex(b[3:]); matchIndices != nil {
        return mouseEventSGRLen, MouseMsg(parseSGRMouseEvent(b))
    }
    // ← no else: falls through to focus/paste/sequence detection
```

The `\x1b` is then re-processed by the generic Alt-modifier path.

The `canHaveMoreData` flag is available at this point, but is not consulted for the SGR case.


## Proposed fix

Two guards are needed, covering different split positions within the sequence (length varies: 11–14 bytes depending on coordinate values):

**Guard 1** — prefix `\x1b[<` is present but `len(b) < 6`, so the outer `if`-block is never entered (split at bytes 3–5):

```go
// Before the existing mouse-detection if-block:
if canHaveMoreData && len(b) >= 3 && b[0] == '\x1b' && b[1] == '[' && b[2] == '<' {
    return 0, nil
}
```

**Guard 2** — `len(b) >= 6` and `case '<'` is entered, but the regex fails because the payload is incomplete (split at bytes 6–10):

```go
case '<':
    if matchIndices := mouseSGRRegex.FindSubmatchIndex(b[3:]); matchIndices != nil {
        mouseEventSGRLen := matchIndices[1] + 3
        return mouseEventSGRLen, MouseMsg(parseSGRMouseEvent(b))
    }
    // Partial payload — wait for more bytes.
    if canHaveMoreData {
        return 0, nil
    }
```

In both cases, returning `w=0` triggers `leftOverFromPrevIteration` in the outer loop, which prepends the fragment to the next `Read` result so the complete sequence can be parsed correctly.

### Coverage

| Split position | Remaining prefix | Status |
|---|---|---|
| byte 1 (`\x1b`) | ESC only | **Already handled** — existing line-686 guard returns `w=0` |
| byte 2 (`\x1b[`) | ESC+`[` | **Already handled** — same guard: `alt=true`, reads `[`, `i==len(b)` → `w=0` |
| bytes 3–5 (`\x1b[<`…) | `len(b)<6`, unambiguously SGR | **Guard 1 fixes this** |
| bytes 6–11+ | `len(b)>=6`, regex fails | **Guard 2 fixes this** |

All split positions are covered by the two guards together. No known limitation remains.

### Why increasing the buffer is not a fix

Enlarging `buf` from 256 to any size N only changes *which* byte position the split falls at. SGR sequences vary in length (11–14+ bytes depending on coordinates), so there is no buffer size that avoids splits for all sequences simultaneously. It is not a correct fix.


## Workaround (until fix is released)

Applications that use a `textinput` widget alongside mouse reporting can intercept `KeyRunes` messages in their `Update` method and strip SGR fragments before forwarding to the input:

```go
var (
    reSGRFull = regexp.MustCompile(`\[<[\d;]+[Mm]`)
    reSGRTail = regexp.MustCompile(`\[<?[\d;]*$`)
)

// consumeSGRFragments strips SGR mouse fragments from s.
// Returns (remainder, tail): remainder is safe to pass to the search box;
// tail is a dangling prefix to accumulate across consecutive messages.
func consumeSGRFragments(s string) (remainder, tail string) {
    s = reSGRFull.ReplaceAllString(s, "")
    if loc := reSGRTail.FindStringIndex(s); loc != nil {
        return s[:loc[0]], s[loc[0]:]
    }
    return s, ""
}

// In Model:
//   sgrBuf string  // accumulates partial prefix across messages

// In Update, default KeyMsg branch:
//   combined := m.sgrBuf + string(msg.Runes)
//   remainder, tail := consumeSGRFragments(combined)
//   m.sgrBuf = tail
//   if remainder == "" { return m, nil }
//   // forward remainderMsg to textinput ...
```

Note: `string(msg.Runes)` must be used instead of `msg.String()` because `msg.String()` prepends `"alt+"` when `msg.Alt == true`, causing the `[` fragment to appear as `"alt+["` and defeating the pattern match.


## Status in bubbletea v2 (HEAD / v2.0.6)

**Fixed.** bubbletea v2 delegates all input parsing to
`github.com/charmbracelet/ultraviolet v0.0.0-20260416155717-489999b90468`.
The architectural change eliminates the bug at all split positions:

| Split position | v1 outcome | v2 outcome |
|---|---|---|
| byte 1 (`\x1b`) | Already handled — line-686 guard → `w=0` | `Decode` returns `KeyPressEvent(Escape)`, `esc && n<=2 && !expired` guard → stops, buffer retained |
| bytes 2–11 (any truncation) | **BUG** — fallthrough to alt path | `parseCsi` returns `UnknownEvent`, `scanEvents` stops on `!expired` → buffer retained |

**Root cause of the fix** — two architectural differences from v1:

1. **Unbounded accumulation buffer** — `StreamEvents` maintains a `bytes.Buffer` that grows across `Read` calls. There is no fixed 256-byte limit that causes split.

2. **`parseCsi` returns `UnknownEvent` for incomplete sequences** — When the final byte (`0x40–0x7E`) is absent because the buffer ends mid-sequence, `parseCsi` exits at line 373 with `return i, UnknownEvent(b[:i])` instead of falling through to an alt-modifier path.

3. **`scanEvents` stops on `UnknownEvent` when `!expired`** — Any incomplete sequence causes the event loop to return early, leaving the fragment in `bytes.Buffer`. The next `Read` appends to it, completing the sequence.

The 50ms ESC timeout (`DefaultEscTimeout`) serves a related purpose: it prevents an isolated `\x1b` byte from blocking indefinitely. After the timeout, `expired=true` and incomplete sequences are emitted as-is (typically as `KeyEscape` or `UnknownEvent`). This is a separate, correct mechanism — not a workaround for the split bug.

**Migration note:** Applications on v1.3.10 that applied the `consumeSGRFragments` workaround should remove it when upgrading to v2, as it is no longer necessary and would incorrectly filter legitimate `KeyRunes` events.
