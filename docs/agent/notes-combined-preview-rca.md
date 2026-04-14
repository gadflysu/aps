# RCA: Combined Mode Preview Shows No Session Info

## Phenomenon

When running `aps -a` (combined Claude + Opencode mode), the fzf preview pane
displayed only a directory listing. The session info block (title, time, message
count, recent messages) was silently absent for all sessions regardless of source.

## Root Cause

The bash implementation reused `preview_claude_session` for combined mode:

```bash
# select_all_session() — combined mode fzf invocation
--preview="bash -c 'source \"$SCRIPT_SELF\" && preview_claude_session {1} {2} {3}'"
```

TAB fields in combined mode are `source\tid\tdirectory\tdisplay`, so:

| fzf field | value passed | function expects |
|-----------|-------------|-----------------|
| `{1}` | `"Claude Code"` or `"OpenCode"` | `session_id` |
| `{2}` | session UUID / Opencode ID | `project_path` |
| `{3}` | working directory | `working_dir` |

`preview_claude_session` constructs the JSONL path as:

```bash
local jsonl_file="$project_path/$session_id.jsonl"
# → e.g. "<uuid>/Claude Code.jsonl"  — file does not exist
```

The `[[ -f "$jsonl_file" ]]` guard fails, the entire session-info block is
skipped, and only the `eza`/`ls` directory listing is printed.

This affects both Claude and Opencode sessions in combined mode.

## Fix (Go rewrite)

The Go rewrite adds `project_path` as an explicit 3rd TAB field in combined mode:

```
source\tsession_id\tproject_path\tcwd\tdisplay_string   (5 fields, --with-nth=5)
```

A dedicated internal subcommand `--_preview-all <source> <id> <project_path> <cwd>`
routes to `preview.Claude(id, projectPath, cwd)` or `preview.Opencode(id, cwd)`
based on source. For Opencode sessions, `project_path` is an empty string and
is ignored by the Opencode preview path.
