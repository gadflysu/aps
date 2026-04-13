#!/bin/bash

# Record script path for preview functions (needed for sourcing in fzf subshells)
SCRIPT_SELF="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"
export SCRIPT_SELF

# ai-pick-session.sh - Session Picker for AI Coding Tools (Claude Code, Opencode)
#
# MAINTENANCE NOTE:
#   Comments in this file document architecture, data flow, and known gotchas.
#   When modifying code, please update corresponding comments to reflect changes.
#   Stale comments are worse than no comments.
#
# ARCHITECTURE:
#   ┌─────────────┐     ┌──────────────┐     ┌───────────┐
#   │ Data Source │ --> │ Python Proc  │ --> │ fzf/List  │
#   │ SQLite/JSONL│     │ Format/Color │     │ Display   │
#   └─────────────┘     └──────────────┘     └───────────┘
#
# DATA SOURCES:
#   - Opencode: SQLite database (~/.local/share/opencode/opencode.db)
#   - Claude:   JSONL files (~/.claude/projects/*/*.jsonl)
#
# OUTPUT MODES:
#   - Interactive: fzf with preview (default)
#   - List: plain text output (-l)
#
# KEY FUNCTIONS:
#   generate_*_interactive_list() - fzf input (hidden fields + display string)
#   generate_*_list_with_time()   - plain list output
#   select_*_session()            - fzf invocation and preview

# Help message function
help_message() {
	cat <<EOF
Usage: $(basename "$0") [-n | --no-launch] [-v | --verbose] [-l | --list] [-c | --claude] [-o | --opencode] [-a | --all] [-d | --danger] [-r | --recursive] [-h | --help] [PATH_FILTER]

Select an Opencode or Claude Code session to resume or get its directory path.

Options:
  -n, --no-launch   Do not launch. Instead, print the target directory.
  -v, --verbose     (Used with -n) Print the full launch command.
  -l, --list        List all sessions in table format and exit (non-interactive).
  -c, --claude      Enable Claude Code sessions.
  -o, --opencode    Enable Opencode sessions (default if no client specified).
  -a, --all         Show all sessions from both clients (same as -c -o).
  -d, --danger      (Claude only) Launch with --dangerously-skip-permissions flag.
  -r, --recursive   Include subdirectories when filtering by path.
  -h, --help        Display this help message.

Notes:
  - Default behavior (no -c/-o/-a): Show Claude Code sessions only.
  - Combine -c and -o (or use -a) to show sessions from both clients.

Arguments:
  PATH_FILTER       Filter sessions by directory path.
                    Default: exact path match only (no subdirectories).
                    With -r: include subdirectories in matching.

Examples:
  # Default sessions (Claude Code)
  $(basename "$0")                          # Select and launch Claude Code session (interactive)
  $(basename "$0") -n                       # Select and print target directory
  cd \$($(basename "$0") -n)               # Select and change directory
  $(basename "$0") -nv                      # Select and print full launch command
  eval \$($(basename "$0") -nv)             # Select and execute full launch command
  $(basename "$0") -l                       # List all Claude Code sessions (non-interactive)
  $(basename "$0") -l .                     # List Claude sessions in current directory

  # Opencode sessions (-o / --opencode)
  $(basename "$0") -o                       # Select and launch Opencode session (interactive)
  $(basename "$0") -o -n                    # Select Opencode session and print directory
  $(basename "$0") -o -nv                   # Select Opencode session and print launch command
  $(basename "$0") -lo                      # List all Opencode sessions (non-interactive)
  $(basename "$0") -lo .                    # List Opencode sessions in current directory
  $(basename "$0") scripts -lo              # List Opencode sessions in scripts directory
  $(basename "$0") /path/to/project -o      # Select Opencode session for specific path

  # Combined sessions (-a / -c -o)
  $(basename "$0") -a                       # Select from both clients and launch (interactive)
  $(basename "$0") -c -o                    # Same as -a (select from merged list)
  $(basename "$0") -a -n                    # Select from both clients and print directory
  $(basename "$0") -a -nv                   # Select from both clients and print launch command
  $(basename "$0") -a -d                    # Select from both (danger mode applies to Claude only)
  $(basename "$0") -la                      # List all sessions from both clients (non-interactive)
  $(basename "$0") -la .                    # List both clients' sessions in current directory
EOF
	exit 0
}

# --- Configuration ---
# OPENCODE_DATA_DIR: Get from environment variable with default value for testing convenience
OPENCODE_DATA_DIR="${OPENCODE_DATA_DIR:-$HOME/.local/share/opencode}"
DB_PATH="$OPENCODE_DATA_DIR/opencode.db"
JSON_STORAGE_PATH="$OPENCODE_DATA_DIR/storage"
DEFAULT_SHELL="${SHELL:-/bin/bash}"

# --- Storage Format Detection ---
# Detect opencode storage format: "sqlite" | "json" | ""
detect_opencode_storage() {
	if [ -f "$DB_PATH" ]; then
		echo "sqlite"
	elif [ -d "$JSON_STORAGE_PATH/session/global" ]; then
		echo "json"
	else
		echo ""
	fi
}

# --- Display Configuration ---
PREVIEW_WIDTH="40%"     # fzf preview window width
# COLUMN_SPACING: Display separator between columns in output.
#   IMPORTANT: This is for VISUAL separation only (inside DisplayString).
#   Do NOT confuse with fzf --delimiter (which uses TAB for field parsing).
#   Using the same char for both will break fzf field extraction!
COLUMN_SPACING="｜"

# Parse arguments
NO_LAUNCH=false
VERBOSE=false
LIST_ONLY=false
CLAUDE_MODE=false
OPENCODE_MODE=false
ALL_MODE=false
DANGER_MODE=false
STRICT_MATCH=true
PATH_FILTER=""
while [[ "$#" -gt 0 ]]; do
	case "$1" in
	--no-launch)
		NO_LAUNCH=true
		shift
		;;
	--verbose)
		VERBOSE=true
		shift
		;;
	--list)
		LIST_ONLY=true
		shift
		;;
	--claude)
		CLAUDE_MODE=true
		shift
		;;
	--opencode)
		OPENCODE_MODE=true
		shift
		;;
	--all)
		ALL_MODE=true
		shift
		;;
	--danger)
		DANGER_MODE=true
		shift
		;;
	--recursive)
		STRICT_MATCH=false
		shift
		;;
	--help)
		help_message
		;;
	-*)
		# Handle combined short options like -nv
		arg="$1"
		shift
		for (( i=1; i<${#arg}; i++ )); do
			case "${arg:$i:1}" in
			n)
				NO_LAUNCH=true
				;;
			v)
				VERBOSE=true
				;;
			l)
				LIST_ONLY=true
				;;
			c)
				CLAUDE_MODE=true
				;;
			o)
				OPENCODE_MODE=true
				;;
			a)
				ALL_MODE=true
				;;
			d)
				DANGER_MODE=true
				;;
			r)
				STRICT_MATCH=false
				;;
			h)
				help_message
				;;
			*)
				echo "Unknown option: -${arg:$i:1}"
				exit 1
				;;
			esac
		done
		;;
	*)
		# First positional argument is path filter
		if [ -z "$PATH_FILTER" ]; then
			PATH_FILTER="$1"
			shift
		else
			# Additional positional arguments not expected
			break
		fi
		;;
	esac
done

# Set default mode: Claude if no client specified
if ! $CLAUDE_MODE && ! $OPENCODE_MODE && ! $ALL_MODE; then
	CLAUDE_MODE=true
fi

# Normalize path filter: convert '.' to current working directory
if [ "$PATH_FILTER" = "." ]; then
	PATH_FILTER=$(pwd)
fi

# Check requirements
# python3 is always required
if ! command -v python3 &>/dev/null; then
	echo "Error: python3 is required but not installed."
	exit 1
fi

# fzf is only required for interactive mode (not -l)
if ! $LIST_ONLY; then
	if ! command -v fzf &>/dev/null; then
		echo "Error: fzf is required for interactive mode but not installed."
		exit 1
	fi
fi

# Check sqlite3 and Opencode storage availability
OPENCODE_AVAILABLE=false
if $ALL_MODE || $OPENCODE_MODE; then
	# For -a/-o mode, check if Opencode is actually available
	if command -v sqlite3 &>/dev/null; then
		STORAGE_FORMAT=$(detect_opencode_storage)
		if [ -n "$STORAGE_FORMAT" ]; then
			OPENCODE_AVAILABLE=true
		fi
	fi

	# In pure Opencode mode (-o without -c), must have Opencode data
	if $OPENCODE_MODE && ! $CLAUDE_MODE && ! $ALL_MODE; then
		if ! $OPENCODE_AVAILABLE; then
			echo "Error: Opencode storage not found or sqlite3 not installed."
			echo "  Checked: $DB_PATH (SQLite)"
			echo "  Checked: $JSON_STORAGE_PATH (JSON)"
			exit 1
		fi
	fi

	# In -a mode, at least one client must be available
	if $ALL_MODE; then
		CLAUDE_AVAILABLE=false
		if [ -d "$HOME/.claude/projects" ] && [ -n "$(ls -A "$HOME/.claude/projects" 2>/dev/null)" ]; then
			CLAUDE_AVAILABLE=true
		fi

		if ! $OPENCODE_AVAILABLE && ! $CLAUDE_AVAILABLE; then
			echo "Error: No sessions found from either Opencode or Claude Code."
			exit 1
		fi

		# Adjust modes based on what's actually available
		if ! $OPENCODE_AVAILABLE; then
			OPENCODE_MODE=false
			CLAUDE_MODE=true
		elif ! $CLAUDE_AVAILABLE; then
			OPENCODE_MODE=true
			CLAUDE_MODE=false
			ALL_MODE=false
		fi
	fi
fi

# Python script to generate structured data:
# Format: ID <TAB> Directory <TAB> Formatted_Visual_String
# This ensures robust data extraction regardless of display formatting
generate_opencode_interactive_list() {
	local path_filter="$1"
	local strict_match="$2"
	python3 -c "
# --- Data Processing Pipeline ---
# 1. Query/Read: Fetch sessions from SQLite database or JSON storage
# 2. Filter: Apply path filter (exact match, symlink resolution, substring)
# 3. Transform: Format time, abbreviate paths (~ for HOME)
# 4. Width Calc: Adaptive column width based on actual content
# 5. Output: TAB-separated fields for fzf (ID \t Path \t DisplayString)
#
# GOTCHA: Wide characters (CJK) occupy 2 display columns.
#   - Use wcswidth() for display width, NOT len()
#   - Use truncate_to_width() for truncation, NOT string slicing
import sqlite3
import sys
import os
import json
import glob
from datetime import datetime
import unicodedata

HOME_DIR = os.path.expanduser('~') # Get home directory in Python
COL_SPACING = '$COLUMN_SPACING'  # Column spacing from shell config

# ANSI Colors
C_ID = '\033[36m'    # Cyan
C_TITLE = '\033[33m' # Yellow
C_DIR = '\033[90m'   # Grey
C_TIME = '\033[32m'  # Green
C_END = '\033[0m'

def wcswidth(s):
    # Calculate display width using Unicode East Asian Width property.
    # Only F (Fullwidth) and W (Wide) characters occupy 2 columns.
    # Other categories (Na, H, A, N) occupy 1 column.
    width = 0
    for c in s:
        ea = unicodedata.east_asian_width(c)
        width += 2 if ea in ('F', 'W') else 1
    return width

def pad(s, width):
    w = wcswidth(s)
    return s + ' ' * (width - w)

def truncate_to_width(s, max_width, suffix='...'):
    # Truncate string by DISPLAY width, not character count.
    # IMPORTANT: For CJK text, len() counts chars but wcswidth() counts display width.
    # Using s[:n] for truncation will cause column misalignment.
    suffix_width = wcswidth(suffix)
    target_width = max_width - suffix_width
    width = 0
    for i, c in enumerate(s):
        char_width = 2 if ord(c) > 127 else 1
        if width + char_width > target_width:
            return s[:i] + suffix
        width += char_width
    return s

def load_json_sessions(storage_path):
    \"\"\"Load sessions from JSON storage format.\"\"\"
    sessions = []
    session_dir = os.path.join(storage_path, 'session', 'global')
    if not os.path.exists(session_dir):
        return sessions
    for json_file in glob.glob(os.path.join(session_dir, 'ses_*.json')):
        try:
            with open(json_file, 'r') as f:
                data = json.load(f)
                sessions.append((
                    data.get('id', ''),
                    data.get('title', ''),
                    data.get('directory', ''),
                    data.get('time', {}).get('updated', 0)
                ))
        except:
            continue
    sessions.sort(key=lambda x: x[3], reverse=True)
    return sessions

try:
    # Load sessions based on storage format
    storage_format = '$STORAGE_FORMAT'
    if storage_format == 'sqlite':
        conn = sqlite3.connect('$DB_PATH')
        cur = conn.cursor()
        cur.execute('''
            SELECT s.id, s.title, s.directory, s.time_updated, COUNT(m.id) as message_count
            FROM session s
            LEFT JOIN message m ON s.id = m.session_id
            GROUP BY s.id, s.title, s.directory, s.time_updated
            ORDER BY s.time_updated DESC
        ''')
        rows = cur.fetchall()
    elif storage_format == 'json':
        rows = load_json_sessions('$JSON_STORAGE_PATH')
    else:
        rows = []

    max_id = 30
    MAX_TITLE_LIMIT = 40
    max_time = 20
    max_msg = 6

    # Get path filter from command line
    path_filter = sys.argv[1] if len(sys.argv) > 1 else ''
    strict_match = sys.argv[2].lower() == 'true' if len(sys.argv) > 2 else False

    # First pass: collect filtered sessions
    filtered_sessions = []

    for rid, title, directory, time_updated, message_count in rows:
        # Sanitize inputs (remove tabs/newlines to prevent breaking structure)
        s_rid = str(rid).replace('\t', ' ').replace('\n', ' ')
        s_title = str(title).replace('\t', ' ').replace('\n', ' ')
        s_dir = str(directory).replace('\t', ' ').replace('\n', ' ')

        # Apply path filter if provided
        if path_filter:
            match_found = False

            # Try to resolve path_filter to absolute path
            try:
                # Try to resolve as a real path (handles relative paths, symlinks, etc.)
                resolved_filter = os.path.realpath(os.path.expanduser(path_filter))

                # Only use resolved path if it actually exists, otherwise fall back to string matching
                if os.path.exists(resolved_filter):
                    # Check if resolved path matches session directory exactly
                    if resolved_filter == s_dir:
                        match_found = True
                    else:
                        # Also try to resolve the session directory to handle symlinks
                        try:
                            resolved_dir = os.path.realpath(s_dir) if os.path.exists(s_dir) else s_dir
                            if resolved_filter == resolved_dir:
                                match_found = True
                            # Check substring matches with resolved paths
                            elif not strict_match and len(resolved_filter) > 10 and (resolved_filter in s_dir or resolved_filter in resolved_dir):
                                match_found = True
                        except (OSError, ValueError):
                            # If we can't resolve session dir, try substring match with resolved filter
                            if not strict_match and len(resolved_filter) > 10 and resolved_filter in s_dir:
                                match_found = True
                # If resolved path doesn't exist, we'll fall through to string matching

            except (OSError, ValueError):
                # Path resolution failed, continue to substring matching
                pass

            # If no path-based match found, try substring matching with original filter
            if not match_found:
                # Only do substring matching if the filter is reasonably specific (length > 2)
                if not strict_match and len(path_filter) > 2 and path_filter in s_dir:
                    match_found = True

            # If no match found at all, skip this session
            if not match_found:
                continue

        # Format time_updated
        if time_updated:
            try:
                # Convert to float first
                timestamp = float(time_updated)

                # Check if it's milliseconds (13 digits) or seconds (10 digits)
                if timestamp > 9999999999:  # More than 10 digits, likely milliseconds
                    timestamp = timestamp / 1000.0

                dt = datetime.fromtimestamp(timestamp)
                s_time = dt.strftime('%Y-%m-%d %H:%M:%S')
            except Exception as e:
                s_time = 'Invalid time'
        else:
            s_time = 'No time'

        # Replace home directory with ~ for display
        if s_dir.startswith(HOME_DIR):
            s_dir_display = s_dir.replace(HOME_DIR, '~', 1)
        else:
            s_dir_display = s_dir

        # Store filtered session data
        filtered_sessions.append((s_rid, s_title, s_dir, s_dir_display, s_time, message_count))

    # Calculate adaptive title width
    max_title = MAX_TITLE_LIMIT
    if filtered_sessions:
        actual_max = max(wcswidth(session[1]) for session in filtered_sessions)
        max_title = min(MAX_TITLE_LIMIT, actual_max)

    # Second pass: format and output
    for s_rid, s_title, s_dir, s_dir_display, s_time, s_msg_count in filtered_sessions:
        # Format display string
        d_title = s_title
        if wcswidth(d_title) > max_title:
            d_title = truncate_to_width(d_title, max_title)

        # Column order: Time + Title + Session ID + Message Count + Directory
        display_str = f'{C_TIME}{pad(s_time, max_time)}{C_END}{COL_SPACING}{C_TITLE}{pad(d_title, max_title)}{C_END}{COL_SPACING}{C_ID}{pad(s_rid, max_id)}{C_END}{COL_SPACING}{C_MSG}{pad(str(s_msg_count), max_msg)}{C_END}{COL_SPACING}{C_DIR}{s_dir_display}{C_END}'

        # Output: ID <TAB> Directory <TAB> DisplayString
        print(f'{s_rid}\t{s_dir}\t{display_str}')

except Exception as e:
    pass
" "$path_filter" "$strict_match"
}

# Generate list with time column for -l option
generate_opencode_list_with_time() {
	local path_filter="$1"
	local strict_match="$2"
	python3 -c "
# --- Data Processing Pipeline ---
# 1. Query/Read: Fetch sessions from SQLite database or JSON storage
# 2. Filter: Apply path filter (exact match, symlink resolution, substring)
# 3. Transform: Format time, abbreviate paths (~ for HOME)
# 4. Width Calc: Adaptive column width based on actual content
# 5. Output: Plain formatted display string (no TAB fields needed for list mode)
#
# GOTCHA: Wide characters (CJK) occupy 2 display columns.
#   - Use wcswidth() for display width, NOT len()
#   - Use truncate_to_width() for truncation, NOT string slicing
import sqlite3
import sys
import os
import json
import glob
from datetime import datetime
import unicodedata

HOME_DIR = os.path.expanduser('~') # Get home directory in Python
COL_SPACING = '$COLUMN_SPACING'  # Column spacing from shell config

# ANSI Colors
C_ID = '\033[36m'    # Cyan
C_TITLE = '\033[33m' # Yellow
C_DIR = '\033[37m'   # White
C_TIME = '\033[32m'  # Green
C_END = '\033[0m'

def wcswidth(s):
    # Calculate display width using Unicode East Asian Width property.
    # Only F (Fullwidth) and W (Wide) characters occupy 2 columns.
    # Other categories (Na, H, A, N) occupy 1 column.
    width = 0
    for c in s:
        ea = unicodedata.east_asian_width(c)
        width += 2 if ea in ('F', 'W') else 1
    return width

def pad(s, width):
    w = wcswidth(s)
    return s + ' ' * (width - w)

def truncate_to_width(s, max_width, suffix='...'):
    # Truncate string by DISPLAY width, not character count.
    # IMPORTANT: For CJK text, len() counts chars but wcswidth() counts display width.
    # Using s[:n] for truncation will cause column misalignment.
    suffix_width = wcswidth(suffix)
    target_width = max_width - suffix_width
    width = 0
    for i, c in enumerate(s):
        char_width = 2 if ord(c) > 127 else 1
        if width + char_width > target_width:
            return s[:i] + suffix
        width += char_width
    return s

def load_json_sessions(storage_path):
    \"\"\"Load sessions from JSON storage format with message count.\"\"\"
    sessions = []
    session_dir = os.path.join(storage_path, 'session', 'global')
    if not os.path.exists(session_dir):
        return sessions
    for json_file in glob.glob(os.path.join(session_dir, 'ses_*.json')):
        try:
            with open(json_file, 'r') as f:
                data = json.load(f)
                session_id = data.get('id', '')

                # Count messages
                message_dir = os.path.join(storage_path, 'message', session_id)
                if os.path.exists(message_dir):
                    message_files = glob.glob(os.path.join(message_dir, 'msg_*.json'))
                    message_count = len(message_files)
                else:
                    message_count = 0

                sessions.append((
                    session_id,
                    data.get('title', ''),
                    data.get('directory', ''),
                    data.get('time', {}).get('updated', 0),
                    message_count
                ))
        except:
            continue
    sessions.sort(key=lambda x: x[3], reverse=True)
    return sessions

try:
    # Load sessions based on storage format
    storage_format = '$STORAGE_FORMAT'
    if storage_format == 'sqlite':
        conn = sqlite3.connect('$DB_PATH')
        cur = conn.cursor()
        cur.execute('''
            SELECT s.id, s.title, s.directory, s.time_updated, COUNT(m.id) as message_count
            FROM session s
            LEFT JOIN message m ON s.id = m.session_id
            GROUP BY s.id, s.title, s.directory, s.time_updated
            ORDER BY s.time_updated DESC
        ''')
        rows = cur.fetchall()
    elif storage_format == 'json':
        rows = load_json_sessions('$JSON_STORAGE_PATH')
    else:
        rows = []

    max_time = 19
    max_id = 30
    max_msg = 6
    MAX_TITLE_LIMIT = 40

    # Get path filter from command line
    path_filter = sys.argv[1] if len(sys.argv) > 1 else ''

    # First pass: collect filtered sessions
    filtered_sessions = []

    for rid, title, directory, time_updated, message_count in rows:
        # Sanitize inputs (remove tabs/newlines to prevent breaking structure)
        s_rid = str(rid).replace('\t', ' ').replace('\n', ' ')
        s_title = str(title).replace('\t', ' ').replace('\n', ' ')
        s_dir = str(directory).replace('\t', ' ').replace('\n', ' ')

        # Apply path filter if provided
        if path_filter:
            match_found = False

            # Try to resolve path_filter to absolute path
            try:
                # Try to resolve as a real path (handles relative paths, symlinks, etc.)
                resolved_filter = os.path.realpath(os.path.expanduser(path_filter))

                # Only use resolved path if it actually exists, otherwise fall back to string matching
                if os.path.exists(resolved_filter):
                    # Check if resolved path matches session directory exactly
                    if resolved_filter == s_dir:
                        match_found = True
                    else:
                        # Also try to resolve the session directory to handle symlinks
                        try:
                            resolved_dir = os.path.realpath(s_dir) if os.path.exists(s_dir) else s_dir
                            if resolved_filter == resolved_dir:
                                match_found = True
                            # Check substring matches with resolved paths
                            elif not strict_match and len(resolved_filter) > 10 and (resolved_filter in s_dir or resolved_filter in resolved_dir):
                                match_found = True
                        except (OSError, ValueError):
                            # If we can't resolve session dir, try substring match with resolved filter
                            if not strict_match and len(resolved_filter) > 10 and resolved_filter in s_dir:
                                match_found = True
                # If resolved path doesn't exist, we'll fall through to string matching

            except (OSError, ValueError):
                # Path resolution failed, continue to substring matching
                pass

            # If no path-based match found, try substring matching with original filter
            if not match_found:
                # Only do substring matching if the filter is reasonably specific (length > 2)
                if not strict_match and len(path_filter) > 2 and path_filter in s_dir:
                    match_found = True

            # If no match found at all, skip this session
            if not match_found:
                continue

        # Format time_updated to YYYY-MM-DD HH:MM:SS
        if time_updated:
            try:
                # Convert to float first
                timestamp = float(time_updated)

                # Check if it's milliseconds (13 digits) or seconds (10 digits)
                if timestamp > 9999999999:  # More than 10 digits, likely milliseconds
                    timestamp = timestamp / 1000.0

                dt = datetime.fromtimestamp(timestamp)
                s_time = dt.strftime('%Y-%m-%d %H:%M:%S')
            except Exception as e:
                s_time = 'Invalid time'
        else:
            s_time = 'No time'

        # Replace home directory with ~ for display
        if s_dir.startswith(HOME_DIR):
            s_dir_display = s_dir.replace(HOME_DIR, '~', 1)
        else:
            s_dir_display = s_dir

        # Store filtered session data
        filtered_sessions.append((s_rid, s_title, s_dir_display, s_time, message_count))

    # Calculate adaptive title width
    max_title = MAX_TITLE_LIMIT
    if filtered_sessions:
        actual_max = max(wcswidth(session[1]) for session in filtered_sessions)
        max_title = min(MAX_TITLE_LIMIT, actual_max)

    # Print header
    C_HEADER = '\033[4;37m'  # Underline white
    C_MSG = '\033[35m'  # Magenta for message count
    header = f'{C_HEADER}{pad(\"TIME\", max_time)}{C_END}{COL_SPACING}{C_HEADER}{pad(\"TITLE\", max_title)}{C_END}{COL_SPACING}{C_HEADER}{pad(\"ID\", max_id)}{C_END}{COL_SPACING}{C_HEADER}{pad(\"MSG\", max_msg)}{C_END}{COL_SPACING}{C_HEADER}DIRECTORY{C_END}'
    print(header)

    # Second pass: format and output
    for s_rid, s_title, s_dir_display, s_time, s_msg_count in filtered_sessions:
        # Format display string
        d_title = s_title
        if wcswidth(d_title) > max_title:
            d_title = truncate_to_width(d_title, max_title)

        # Column order: Time + Title + Session ID + Message Count + Directory
        display_str = f'{C_TIME}{pad(s_time, max_time)}{C_END}{COL_SPACING}{C_TITLE}{pad(d_title, max_title)}{C_END}{COL_SPACING}{C_ID}{pad(s_rid, max_id)}{C_END}{COL_SPACING}{C_MSG}{pad(str(s_msg_count), max_msg)}{C_END}{COL_SPACING}{C_DIR}{s_dir_display}{C_END}'

        # Just output the formatted display string for list mode
        print(display_str)

except Exception as e:
    pass
" "$path_filter" "$strict_match"
}

# Generate Claude Code interactive list for fzf selection
generate_claude_interactive_list() {
	local path_filter="$1"
	local strict_match="$2"
	python3 -c "
# --- Data Processing Pipeline ---
# 1. Query/Read: Scan JSONL files in ~/.claude/projects/*/*.jsonl
# 2. Filter: Apply path filter (exact match, symlink resolution, substring)
# 3. Transform: Extract title (custom-title > first user message), format time
# 4. Width Calc: Adaptive column width based on actual content
# 5. Output: TAB-separated fields (SessionID \t ProjectPath \t WorkingDir \t DisplayString)
#
# GOTCHA: Wide characters (CJK) occupy 2 display columns.
#   - Use wcswidth() for display width, NOT len()
#   - Use truncate_to_width() for truncation, NOT string slicing
import os
import json
import glob
import sys
from datetime import datetime
import urllib.parse
import unicodedata

HOME_DIR = os.path.expanduser('~')
CLAUDE_PROJECTS_DIR = os.path.join(HOME_DIR, '.claude', 'projects')
COL_SPACING = '$COLUMN_SPACING'  # Column spacing from shell config

# ANSI Colors
C_ID = '\033[36m'    # Cyan
C_TITLE = '\033[33m' # Yellow
C_DIR = '\033[90m'   # Grey
C_TIME = '\033[32m'  # Green
C_MSG = '\033[35m'   # Magenta for message count
C_END = '\033[0m'

def wcswidth(s):
    # Calculate display width using Unicode East Asian Width property.
    # Only F (Fullwidth) and W (Wide) characters occupy 2 columns.
    # Other categories (Na, H, A, N) occupy 1 column.
    width = 0
    for c in s:
        ea = unicodedata.east_asian_width(c)
        width += 2 if ea in ('F', 'W') else 1
    return width

def pad(s, width):
    w = wcswidth(s)
    return s + ' ' * (width - w)

def truncate_to_width(s, max_width, suffix='...'):
    # Truncate string by DISPLAY width, not character count.
    # IMPORTANT: For CJK text, len() counts chars but wcswidth() counts display width.
    # Using s[:n] for truncation will cause column misalignment.
    suffix_width = wcswidth(suffix)
    target_width = max_width - suffix_width
    width = 0
    for i, c in enumerate(s):
        char_width = 2 if ord(c) > 127 else 1
        if width + char_width > target_width:
            return s[:i] + suffix
        width += char_width
    return s

def decode_project_path(encoded_path):
    # Decode URL-encoded project path
    try:
        return urllib.parse.unquote(encoded_path)
    except:
        return encoded_path

def extract_title_from_jsonl(jsonl_path):
    # Extract title from JSONL file with priority: custom-title > first user message
    custom_title = None
    first_user_message_title = None

    try:
        with open(jsonl_path, 'r', encoding='utf-8') as f:
            for line in f:
                try:
                    data = json.loads(line.strip())

                    # Priority 1: Check for custom-title record
                    if data.get('type') == 'custom-title':
                        custom_title = data.get('customTitle')
                        if custom_title:
                            custom_title = custom_title.strip()

                    # Priority 2: Collect first user message as fallback (only if not found yet)
                    if first_user_message_title is None and data.get('type') == 'user' and data.get('message', {}).get('role') == 'user':
                        message = data['message']
                        content = message.get('content')

                        # Extract content string from various structures
                        content_str = ''
                        if isinstance(content, list) and len(content) > 0:
                            # Look for text content in list
                            for item in content:
                                if isinstance(item, dict) and item.get('type') == 'text':
                                    text = item.get('text', '').strip()
                                    if text:
                                        content_str = text
                                        break
                        elif isinstance(content, str):
                            content_str = content.strip()

                        # Skip system messages
                        if content_str and not (
                            content_str.startswith('<local-command-caveat>') or
                            content_str.startswith('<command-message>') or
                            content_str.startswith('<command-name>') or
                            content_str.startswith('<local-command-stdout>') or
                            content_str.startswith('<bash-input>') or
                            content_str.startswith('<bash-stdout>') or
                            content_str.startswith('<task-notification>') or
                            content_str.startswith('[Request interrupted') or
                            content_str.startswith(\"[{'type': 'tool_result'\")
                        ):
                            # Special handling for 'Implement the following plan:'
                            lines = content_str.split('\n')
                            first_line = lines[0].strip()

                            if first_line == 'Implement the following plan:' and len(lines) > 1:
                                # Skip empty lines, find first non-empty line
                                for i in range(1, len(lines)):
                                    second_line = lines[i].strip()
                                    if second_line:
                                        title = 'Plan: ' + second_line
                                        break
                                else:
                                    title = first_line
                            else:
                                title = first_line

                            first_user_message_title = title[:50] if title else None

                except json.JSONDecodeError:
                    continue

        # Return with priority: custom_title > first_user_message_title > 'Untitled'
        return custom_title or first_user_message_title or 'Untitled'

    except:
        return 'Untitled'

def extract_cwd_from_jsonl(jsonl_path):
    # Extract working directory from JSONL file
    try:
        with open(jsonl_path, 'r', encoding='utf-8') as f:
            for line in f:
                try:
                    data = json.loads(line.strip())
                    if 'cwd' in data:
                        return data['cwd']
                except json.JSONDecodeError:
                    continue
        return None
    except:
        return None

try:
    if not os.path.exists(CLAUDE_PROJECTS_DIR):
        # No Claude projects directory found
        exit(0)

    sessions = []

    # Get path filter from command line
    path_filter = sys.argv[1] if len(sys.argv) > 1 else ''
    strict_match = sys.argv[2].lower() == 'true' if len(sys.argv) > 2 else False

    # Traverse all project directories
    for project_dir in os.listdir(CLAUDE_PROJECTS_DIR):
        project_path = os.path.join(CLAUDE_PROJECTS_DIR, project_dir)
        if not os.path.isdir(project_path):
            continue

        # Find all .jsonl files (session files)
        jsonl_pattern = os.path.join(project_path, '*.jsonl')
        jsonl_files = glob.glob(jsonl_pattern)

        for jsonl_file in jsonl_files:
            session_id = os.path.splitext(os.path.basename(jsonl_file))[0]

            # Get file modification time
            try:
                mtime = os.path.getmtime(jsonl_file)
                dt = datetime.fromtimestamp(mtime)
                time_str = dt.strftime('%Y-%m-%d %H:%M:%S')
            except:
                time_str = 'Unknown'

            # Extract title and working directory
            title = extract_title_from_jsonl(jsonl_file)
            cwd = extract_cwd_from_jsonl(jsonl_file)

            if not cwd:
                # Fallback: decode project directory name as path
                cwd = decode_project_path(project_dir)
                if not cwd.startswith('/'):
                    continue  # Skip invalid paths

            # Replace home directory with ~ for display
            if cwd.startswith(HOME_DIR):
                cwd_display = cwd.replace(HOME_DIR, '~', 1)
            else:
                cwd_display = cwd

            # Apply path filter if provided
            if path_filter:
                match_found = False
                path_exists = False

                # Try to resolve path_filter to absolute path
                try:
                    # Try to resolve as a real path (handles relative paths, symlinks, etc.)
                    resolved_filter = os.path.realpath(os.path.expanduser(path_filter))

                    # Only use resolved path if it actually exists
                    if os.path.exists(resolved_filter):
                        path_exists = True
                        # Check if resolved path matches session cwd exactly
                        if resolved_filter == cwd:
                            match_found = True
                        else:
                            # Also try to resolve the session cwd to handle symlinks
                            try:
                                resolved_cwd = os.path.realpath(cwd) if os.path.exists(cwd) else cwd
                                if resolved_filter == resolved_cwd:
                                    match_found = True
                                # Check substring matches with resolved paths
                                elif not strict_match and len(resolved_filter) > 10 and (resolved_filter in cwd or resolved_filter in resolved_cwd):
                                    match_found = True
                            except (OSError, ValueError):
                                # If we can't resolve cwd, try substring match with resolved filter
                                if not strict_match and len(resolved_filter) > 10 and resolved_filter in cwd:
                                    match_found = True

                except (OSError, ValueError):
                    # Path resolution failed, continue to substring matching
                    pass

                # If no path-based match found, try substring matching with original filter
                if not match_found:
                    # Only do substring matching if the filter is reasonably specific (length > 2)
                    # Allow substring matching even in strict mode if the path doesn't exist (user likely means substring)
                    if len(path_filter) > 2 and path_filter in cwd and (not strict_match or not path_exists):
                        match_found = True

                # If no match found at all, skip this session
                if not match_found:
                    continue

            # Count user messages
            user_message_count = 0
            try:
                with open(jsonl_file, 'r') as f:
                    for line in f:
                        try:
                            data = json.loads(line.strip())
                            if data.get('type') == 'user':
                                user_message_count += 1
                        except:
                            continue
            except:
                user_message_count = 0

            sessions.append((mtime, time_str, session_id, title, cwd, cwd_display, project_path, user_message_count))

    # Sort by modification time (newest first)
    sessions.sort(key=lambda x: x[0], reverse=True)

    # Calculate adaptive title width
    max_id = 12  # Only show first 12 chars of UUID for readability
    MAX_TITLE_LIMIT = 40
    max_time = 19
    max_msg = 6  # MSG column width

    max_title = MAX_TITLE_LIMIT
    if sessions:
        actual_max = max(wcswidth(session[3]) for session in sessions)  # session[3] is title
        max_title = min(MAX_TITLE_LIMIT, actual_max)

    # Format output: SessionID <TAB> ProjectPath <TAB> WorkingDir <TAB> DisplayString
    for _, time_str, session_id, title, cwd, cwd_display, project_path, user_message_count in sessions:
        # Sanitize inputs
        s_id = str(session_id).replace('\t', ' ').replace('\n', ' ')
        s_title = str(title).replace('\t', ' ').replace('\n', ' ')
        s_cwd = str(cwd_display).replace('\t', ' ').replace('\n', ' ')

        # Truncate Session ID for display (show first 12 chars)
        d_id = s_id[:12]

        # Truncate title if too long
        d_title = s_title
        if wcswidth(d_title) > max_title:
            d_title = truncate_to_width(d_title, max_title)

        # Column order: Time + Title + Session ID + Message Count + Directory
        display_str = f'{C_TIME}{pad(time_str, max_time)}{C_END}{COL_SPACING}{C_TITLE}{pad(d_title, max_title)}{C_END}{COL_SPACING}{C_ID}{pad(d_id, max_id)}{C_END}{COL_SPACING}{C_MSG}{pad(str(user_message_count), max_msg)}{C_END}{COL_SPACING}{C_DIR}{s_cwd}{C_END}'

        # Output: SessionID <TAB> ProjectPath <TAB> WorkingDir <TAB> DisplayString
        print(f'{s_id}\t{project_path}\t{cwd}\t{display_str}')

except Exception as e:
    pass
" "$path_filter" "$strict_match"
}

# Generate Claude Code session list with time column for -l --claude option
generate_claude_list_with_time() {
	local path_filter="$1"
	local strict_match="$2"
	python3 -c "
# --- Data Processing Pipeline ---
# 1. Query/Read: Scan JSONL files in ~/.claude/projects/*/*.jsonl
# 2. Filter: Apply path filter (exact match, symlink resolution, substring)
# 3. Transform: Extract title (custom-title > first user message), format time, count user messages
# 4. Width Calc: Adaptive column width based on actual content
# 5. Output: Plain formatted display string (no TAB fields needed for list mode)
#
# GOTCHA: Wide characters (CJK) occupy 2 display columns.
#   - Use wcswidth() for display width, NOT len()
#   - Use truncate_to_width() for truncation, NOT string slicing
import os
import json
import glob
import sys
from datetime import datetime
import urllib.parse
import unicodedata

HOME_DIR = os.path.expanduser('~')
CLAUDE_PROJECTS_DIR = os.path.join(HOME_DIR, '.claude', 'projects')
COL_SPACING = '$COLUMN_SPACING'  # Column spacing from shell config

# ANSI Colors
C_ID = '\033[36m'    # Cyan
C_TITLE = '\033[33m' # Yellow
C_DIR = '\033[37m'   # White
C_TIME = '\033[32m'  # Green
C_END = '\033[0m'

def wcswidth(s):
    # Calculate display width using Unicode East Asian Width property.
    # Only F (Fullwidth) and W (Wide) characters occupy 2 columns.
    # Other categories (Na, H, A, N) occupy 1 column.
    width = 0
    for c in s:
        ea = unicodedata.east_asian_width(c)
        width += 2 if ea in ('F', 'W') else 1
    return width

def pad(s, width):
    w = wcswidth(s)
    return s + ' ' * (width - w)

def truncate_to_width(s, max_width, suffix='...'):
    # Truncate string by DISPLAY width, not character count.
    # IMPORTANT: For CJK text, len() counts chars but wcswidth() counts display width.
    # Using s[:n] for truncation will cause column misalignment.
    suffix_width = wcswidth(suffix)
    target_width = max_width - suffix_width
    width = 0
    for i, c in enumerate(s):
        char_width = 2 if ord(c) > 127 else 1
        if width + char_width > target_width:
            return s[:i] + suffix
        width += char_width
    return s

def decode_project_path(encoded_path):
    # Decode URL-encoded project path
    try:
        return urllib.parse.unquote(encoded_path)
    except:
        return encoded_path

def extract_title_from_jsonl(jsonl_path):
    # Extract title from JSONL file with priority: custom-title > first user message
    custom_title = None
    first_user_message_title = None

    try:
        with open(jsonl_path, 'r', encoding='utf-8') as f:
            for line in f:
                try:
                    data = json.loads(line.strip())

                    # Priority 1: Check for custom-title record
                    if data.get('type') == 'custom-title':
                        custom_title = data.get('customTitle')
                        if custom_title:
                            custom_title = custom_title.strip()

                    # Priority 2: Collect first user message as fallback (only if not found yet)
                    if first_user_message_title is None and data.get('type') == 'user' and data.get('message', {}).get('role') == 'user':
                        message = data['message']
                        content = message.get('content')

                        # Extract content string from various structures
                        content_str = ''
                        if isinstance(content, list) and len(content) > 0:
                            # Look for text content in list
                            for item in content:
                                if isinstance(item, dict) and item.get('type') == 'text':
                                    text = item.get('text', '').strip()
                                    if text:
                                        content_str = text
                                        break
                        elif isinstance(content, str):
                            content_str = content.strip()

                        # Skip system messages (based on real data analysis of 392 messages)
                        if content_str and not (
                            content_str.startswith('<local-command-caveat>') or      # 37 occurrences
                            content_str.startswith('<command-message>') or           # 13 occurrences
                            content_str.startswith('<command-name>') or              # 32 occurrences
                            content_str.startswith('<local-command-stdout>') or      # 5+ occurrences
                            content_str.startswith('<bash-input>') or               # bash commands
                            content_str.startswith('<bash-stdout>') or              # bash output
                            content_str.startswith('<task-notification>') or        # task notifications
                            content_str.startswith('[Request interrupted') or       # user interruptions (in list structure)
                            content_str.startswith(\"[{'type': 'tool_result'\")      # tool result messages
                        ):
                            # Special handling for 'Implement the following plan:'
                            lines = content_str.split('\n')
                            first_line = lines[0].strip()

                            if first_line == 'Implement the following plan:' and len(lines) > 1:
                                # Skip empty lines, find first non-empty line
                                for i in range(1, len(lines)):
                                    second_line = lines[i].strip()
                                    if second_line:
                                        title = 'Plan: ' + second_line
                                        break
                                else:
                                    title = first_line
                            else:
                                title = first_line

                            first_user_message_title = title[:50] if title else None

                except json.JSONDecodeError:
                    continue

        # Return with priority: custom_title > first_user_message_title > 'Untitled'
        return custom_title or first_user_message_title or 'Untitled'

    except:
        return 'Untitled'

def extract_cwd_from_jsonl(jsonl_path):
    # Extract working directory from JSONL file
    try:
        with open(jsonl_path, 'r', encoding='utf-8') as f:
            for line in f:
                try:
                    data = json.loads(line.strip())
                    if 'cwd' in data:
                        return data['cwd']
                except json.JSONDecodeError:
                    continue
        return None
    except:
        return None

try:
    if not os.path.exists(CLAUDE_PROJECTS_DIR):
        # No Claude projects directory found
        exit(0)

    sessions = []

    # Get path filter from command line
    path_filter = sys.argv[1] if len(sys.argv) > 1 else ''
    strict_match = sys.argv[2].lower() == 'true' if len(sys.argv) > 2 else False

    # Traverse all project directories
    for project_dir in os.listdir(CLAUDE_PROJECTS_DIR):
        project_path = os.path.join(CLAUDE_PROJECTS_DIR, project_dir)
        if not os.path.isdir(project_path):
            continue

        # Find all .jsonl files (session files)
        jsonl_pattern = os.path.join(project_path, '*.jsonl')
        jsonl_files = glob.glob(jsonl_pattern)

        for jsonl_file in jsonl_files:
            session_id = os.path.splitext(os.path.basename(jsonl_file))[0]

            # Get file modification time
            try:
                mtime = os.path.getmtime(jsonl_file)
                dt = datetime.fromtimestamp(mtime)
                time_str = dt.strftime('%Y-%m-%d %H:%M:%S')
            except:
                time_str = 'Unknown'

            # Extract title and working directory
            title = extract_title_from_jsonl(jsonl_file)
            cwd = extract_cwd_from_jsonl(jsonl_file)

            if not cwd:
                # Fallback: decode project directory name as path
                cwd = decode_project_path(project_dir)
                if not cwd.startswith('/'):
                    continue  # Skip invalid paths

            # Replace home directory with ~ for display
            if cwd.startswith(HOME_DIR):
                cwd_display = cwd.replace(HOME_DIR, '~', 1)
            else:
                cwd_display = cwd

            # Apply path filter if provided
            if path_filter:
                match_found = False
                path_exists = False

                # Try to resolve path_filter to absolute path
                try:
                    # Try to resolve as a real path (handles relative paths, symlinks, etc.)
                    resolved_filter = os.path.realpath(os.path.expanduser(path_filter))

                    # Only use resolved path if it actually exists
                    if os.path.exists(resolved_filter):
                        path_exists = True
                        # Check if resolved path matches session cwd exactly
                        if resolved_filter == cwd:
                            match_found = True
                        else:
                            # Also try to resolve the session cwd to handle symlinks
                            try:
                                resolved_cwd = os.path.realpath(cwd) if os.path.exists(cwd) else cwd
                                if resolved_filter == resolved_cwd:
                                    match_found = True
                                # Check substring matches with resolved paths
                                elif not strict_match and len(resolved_filter) > 10 and (resolved_filter in cwd or resolved_filter in resolved_cwd):
                                    match_found = True
                            except (OSError, ValueError):
                                # If we can't resolve cwd, try substring match with resolved filter
                                if not strict_match and len(resolved_filter) > 10 and resolved_filter in cwd:
                                    match_found = True

                except (OSError, ValueError):
                    # Path resolution failed, continue to substring matching
                    pass

                # If no path-based match found, try substring matching with original filter
                if not match_found:
                    # Only do substring matching if the filter is reasonably specific (length > 2)
                    # Allow substring matching even in strict mode if the path doesn't exist (user likely means substring)
                    if len(path_filter) > 2 and path_filter in cwd and (not strict_match or not path_exists):
                        match_found = True

                # If no match found at all, skip this session
                if not match_found:
                    continue

            # Count user messages
            user_message_count = 0
            try:
                with open(jsonl_file, 'r') as f:
                    for line in f:
                        try:
                            data = json.loads(line.strip())
                            if data.get('type') == 'user':
                                user_message_count += 1
                        except:
                            continue
            except:
                user_message_count = 0

            sessions.append((mtime, time_str, session_id, title, cwd_display, user_message_count))

    # Sort by modification time (newest first)
    sessions.sort(key=lambda x: x[0], reverse=True)

    # Format output
    max_time = 19
    max_id = 36  # UUID length
    max_msg = 6
    MAX_TITLE_LIMIT = 40

    # Calculate adaptive title width
    max_title = MAX_TITLE_LIMIT
    if sessions:
        actual_max = max(wcswidth(session[3]) for session in sessions)  # session[3] is title
        max_title = min(MAX_TITLE_LIMIT, actual_max)

    # Print header
    C_HEADER = '\033[4;37m'  # Underline white
    C_MSG = '\033[35m'  # Magenta for message count
    header = f'{C_HEADER}{pad(\"TIME\", max_time)}{C_END}{COL_SPACING}{C_HEADER}{pad(\"TITLE\", max_title)}{C_END}{COL_SPACING}{C_HEADER}{pad(\"ID\", max_id)}{C_END}{COL_SPACING}{C_HEADER}{pad(\"MSG\", max_msg)}{C_END}{COL_SPACING}{C_HEADER}DIRECTORY{C_END}'
    print(header)

    for _, time_str, session_id, title, cwd_display, user_message_count in sessions:
        # Sanitize inputs
        s_id = str(session_id).replace('\t', ' ').replace('\n', ' ')
        s_title = str(title).replace('\t', ' ').replace('\n', ' ')
        s_cwd = str(cwd_display).replace('\t', ' ').replace('\n', ' ')

        # Truncate title if too long
        d_title = s_title
        if wcswidth(d_title) > max_title:
            d_title = truncate_to_width(d_title, max_title)

        # Column order: Time + Title + Session ID + Message Count + Directory
        display_str = f'{C_TIME}{pad(time_str, max_time)}{C_END}{COL_SPACING}{C_TITLE}{pad(d_title, max_title)}{C_END}{COL_SPACING}{C_ID}{pad(s_id, max_id)}{C_END}{COL_SPACING}{C_MSG}{pad(str(user_message_count), max_msg)}{C_END}{COL_SPACING}{C_DIR}{s_cwd}{C_END}'
        print(display_str)

except Exception as e:
    pass
" "$path_filter" "$strict_match"
}

# Generate merged list with time column for -a (all) option
generate_all_list_with_time() {
	local path_filter="$1"
	local strict_match="$2"
	local path_filter="$1"
	python3 -c "
# --- Data Processing Pipeline for Merged List ---
# 1. Query/Read: Fetch sessions from both Opencode SQLite and Claude JSONL files
# 2. Filter: Apply path filter to both sources
# 3. Transform: Add source identifier (OC/CC), format consistently
# 4. Merge: Combine and sort by time (newest first)
# 5. Output: Plain formatted display string with SRC column
import os
import json
import glob
import sys
from datetime import datetime
import unicodedata

# Try to import sqlite3, but don't fail if it's not available
try:
    import sqlite3
    SQLITE3_AVAILABLE = True
except ImportError:
    SQLITE3_AVAILABLE = False

HOME_DIR = os.path.expanduser('~')
CLAUDE_PROJECTS_DIR = os.path.join(HOME_DIR, '.claude', 'projects')
COL_SPACING = '$COLUMN_SPACING'

# ANSI Colors
C_ID = '\033[36m'      # Cyan
C_TITLE = '\033[33m'   # Yellow
C_DIR = '\033[37m'     # White
C_TIME = '\033[32m'    # Green
C_SRC = '\033[35m'     # Magenta
C_MSG = '\033[35m'     # Magenta
C_END = '\033[0m'
C_HEADER = '\033[4;37m'  # Underline white

def wcswidth(s):
    width = 0
    for c in s:
        ea = unicodedata.east_asian_width(c)
        width += 2 if ea in ('F', 'W') else 1
    return width

def pad(s, width):
    w = wcswidth(s)
    return s + ' ' * (width - w)

def truncate_to_width(s, max_width, suffix='...'):
    suffix_width = wcswidth(suffix)
    target_width = max_width - suffix_width
    if target_width <= 0:
        return suffix[:max_width]
    current_width = 0
    for i, c in enumerate(s):
        char_width = 2 if unicodedata.east_asian_width(c) in ('F', 'W') else 1
        if current_width + char_width > target_width:
            return s[:i] + suffix
        current_width += char_width
    return s

path_filter = sys.argv[1] if len(sys.argv) > 1 else ''
strict_match = sys.argv[2].lower() == 'true' if len(sys.argv) > 2 else False

# Collect all sessions from both sources
all_sessions = []

# --- Load Opencode sessions ---
try:
    db_path = os.path.join(HOME_DIR, '.local', 'share', 'opencode', 'opencode.db')
    if SQLITE3_AVAILABLE and os.path.exists(db_path):
        conn = sqlite3.connect(db_path)
        cursor = conn.cursor()
        cursor.execute('''
            SELECT s.id, s.title, s.directory, s.time_updated, COUNT(m.id) as message_count
            FROM session s
            LEFT JOIN message m ON s.id = m.session_id
            GROUP BY s.id, s.title, s.directory, s.time_updated
            ORDER BY s.time_updated DESC
            LIMIT 50
        ''')
        for rid, title, directory, time_updated, message_count in cursor.fetchall():
            # Apply path filter
            if path_filter:
                match_found = False
                path_exists = False
                try:
                    resolved_filter = os.path.realpath(os.path.expanduser(path_filter))
                    if os.path.exists(resolved_filter):
                        path_exists = True
                        if resolved_filter == directory:
                            match_found = True
                        else:
                            try:
                                resolved_dir = os.path.realpath(directory) if os.path.exists(directory) else directory
                                if resolved_filter == resolved_dir or (not strict_match and len(resolved_filter) > 10 and (resolved_filter in directory or resolved_filter in resolved_dir)):
                                    match_found = True
                            except:
                                if not strict_match and len(resolved_filter) > 10 and resolved_filter in directory:
                                    match_found = True
                except:
                    pass
                if not match_found and len(path_filter) > 2 and path_filter in directory and (not strict_match or not path_exists):
                    match_found = True
                if not match_found:
                    continue

            # Format time
            if time_updated:
                try:
                    timestamp = float(time_updated)
                    if timestamp > 9999999999:
                        timestamp = timestamp / 1000.0
                    dt = datetime.fromtimestamp(timestamp)
                    time_str = dt.strftime('%Y-%m-%d %H:%M:%S')
                    time_sort = timestamp
                except:
                    time_str = 'Invalid time'
                    time_sort = 0
            else:
                time_str = 'No time'
                time_sort = 0

            # Format directory
            dir_display = directory.replace(HOME_DIR, '~', 1) if directory.startswith(HOME_DIR) else directory

            all_sessions.append({
                'source': 'OpenCode',
                'time_sort': time_sort,
                'time_str': time_str,
                'title': str(title).replace('\t', ' ').replace('\n', ' '),
                'id': str(rid).replace('\t', ' ').replace('\n', ' '),
                'msg_count': message_count,
                'directory': dir_display
            })
        conn.close()
except Exception as e:
    pass

# --- Load Claude sessions ---
try:
    if os.path.exists(CLAUDE_PROJECTS_DIR):
        for jsonl_file in glob.glob(os.path.join(CLAUDE_PROJECTS_DIR, '*', '*.jsonl')):
            try:
                # Extract session ID from filename
                session_id = os.path.basename(jsonl_file).replace('.jsonl', '')

                # Get working directory from first entry
                cwd = None
                title = None
                mtime = os.path.getmtime(jsonl_file)
                time_str = datetime.fromtimestamp(mtime).strftime('%Y-%m-%d %H:%M:%S')

                with open(jsonl_file, 'r') as f:
                    for line in f:
                        try:
                            data = json.loads(line.strip())
                            # Check for cwd in any type of entry
                            if cwd is None and 'cwd' in data:
                                cwd = data['cwd']

                            # Extract title: LAST custom-title > first user message
                            if data.get('type') == 'custom-title':
                                # Always update to get LAST custom-title (user may rename multiple times)
                                title = data.get('customTitle', '')
                            elif data.get('type') == 'user' and title is None:
                                # Try message.content first, then content directly
                                content = data.get('message', {}).get('content', '') if 'message' in data else data.get('content', '')
                                if isinstance(content, str):
                                    title = content[:50]
                                elif isinstance(content, list):
                                    for item in content:
                                        if isinstance(item, dict) and item.get('type') == 'text':
                                            title = item.get('text', '')[:50]
                                            break
                        except:
                            continue

                if not cwd:
                    continue

                if not title:
                    title = 'Untitled'

                # Apply path filter
                if path_filter:
                    match_found = False
                    path_exists = False
                    try:
                        resolved_filter = os.path.realpath(os.path.expanduser(path_filter))
                        if os.path.exists(resolved_filter):
                            path_exists = True
                            if resolved_filter == cwd:
                                match_found = True
                            else:
                                try:
                                    resolved_cwd = os.path.realpath(cwd) if os.path.exists(cwd) else cwd
                                    if resolved_filter == resolved_cwd or (not strict_match and len(resolved_filter) > 10 and (resolved_filter in cwd or resolved_filter in resolved_cwd)):
                                        match_found = True
                                except:
                                    if not strict_match and len(resolved_filter) > 10 and resolved_filter in cwd:
                                        match_found = True
                    except:
                        pass
                    if not match_found and len(path_filter) > 2 and path_filter in cwd and (not strict_match or not path_exists):
                        match_found = True
                    if not match_found:
                        continue

                # Count user messages
                user_message_count = 0
                with open(jsonl_file, 'r') as f:
                    for line in f:
                        try:
                            data = json.loads(line.strip())
                            if data.get('type') == 'user':
                                user_message_count += 1
                        except:
                            continue

                # Format directory
                cwd_display = cwd.replace(HOME_DIR, '~', 1) if cwd.startswith(HOME_DIR) else cwd

                all_sessions.append({
                    'source': 'Claude Code',
                    'time_sort': mtime,
                    'time_str': time_str,
                    'title': str(title).replace('\t', ' ').replace('\n', ' '),
                    'id': str(session_id).replace('\t', ' ').replace('\n', ' '),
                    'msg_count': user_message_count,
                    'directory': cwd_display
                })
            except Exception as e:
                continue
except Exception as e:
    pass

# Sort by time (newest first) and limit to 50
all_sessions.sort(key=lambda x: x['time_sort'], reverse=True)
all_sessions = all_sessions[:50]

# Calculate column widths
max_time = 19
max_id = 36  # Full UUID width
max_msg = 6
max_src = 11  # 'Claude Code' width
MAX_TITLE_LIMIT = 40

max_title = MAX_TITLE_LIMIT
if all_sessions:
    actual_max = max(wcswidth(s['title']) for s in all_sessions)
    max_title = min(MAX_TITLE_LIMIT, actual_max)

# Print header
header = f'{C_HEADER}{pad(\"TIME\", max_time)}{C_END}{COL_SPACING}{C_HEADER}{pad(\"TITLE\", max_title)}{C_END}{COL_SPACING}{C_HEADER}{pad(\"ID\", max_id)}{C_END}{COL_SPACING}{C_HEADER}{pad(\"MSG\", max_msg)}{C_END}{COL_SPACING}{C_HEADER}{pad(\"SRC\", max_src)}{C_END}{COL_SPACING}{C_HEADER}DIRECTORY{C_END}'
print(header)

# Print sessions
for session in all_sessions:
    # Truncate title if needed
    d_title = session['title']
    if wcswidth(d_title) > max_title:
        d_title = truncate_to_width(d_title, max_title)

    # Use full ID (no truncation)
    d_id = session['id']

    # Format output
    display_str = f\"{C_TIME}{pad(session['time_str'], max_time)}{C_END}{COL_SPACING}{C_TITLE}{pad(d_title, max_title)}{C_END}{COL_SPACING}{C_ID}{pad(d_id, max_id)}{C_END}{COL_SPACING}{C_MSG}{pad(str(session['msg_count']), max_msg)}{C_END}{COL_SPACING}{C_SRC}{pad(session['source'], max_src)}{C_END}{COL_SPACING}{C_DIR}{session['directory']}{C_END}\"
    print(display_str)
" "$path_filter" "$strict_match"
}

# Generate merged interactive list for fzf with unified format
generate_all_interactive_list() {
	local path_filter="$1"
	local strict_match="$2"
	python3 -c "
# --- Data Processing Pipeline for Merged Interactive List ---
# 1. Query/Read: Fetch sessions from both Opencode SQLite and Claude JSONL files
# 2. Filter: Apply path filter to both sources
# 3. Transform: Add source identifier (OC/CC), format consistently
# 4. Merge: Combine and sort by time (newest first)
# 5. Output: TAB-separated fields for fzf (Source \t ID \t Directory \t DisplayString)
import os
import json
import glob
import sys
from datetime import datetime
import unicodedata

# Try to import sqlite3, but don't fail if it's not available
try:
    import sqlite3
    SQLITE3_AVAILABLE = True
except ImportError:
    SQLITE3_AVAILABLE = False

HOME_DIR = os.path.expanduser('~')
CLAUDE_PROJECTS_DIR = os.path.join(HOME_DIR, '.claude', 'projects')
COL_SPACING = '$COLUMN_SPACING'

# ANSI Colors
C_ID = '\033[36m'      # Cyan
C_TITLE = '\033[33m'   # Yellow
C_DIR = '\033[90m'     # Grey
C_TIME = '\033[32m'    # Green
C_SRC = '\033[35m'     # Magenta
C_END = '\033[0m'

def wcswidth(s):
    width = 0
    for c in s:
        ea = unicodedata.east_asian_width(c)
        width += 2 if ea in ('F', 'W') else 1
    return width

def pad(s, width):
    w = wcswidth(s)
    return s + ' ' * (width - w)

def truncate_to_width(s, max_width, suffix='...'):
    suffix_width = wcswidth(suffix)
    target_width = max_width - suffix_width
    if target_width <= 0:
        return suffix[:max_width]
    current_width = 0
    for i, c in enumerate(s):
        char_width = 2 if unicodedata.east_asian_width(c) in ('F', 'W') else 1
        if current_width + char_width > target_width:
            return s[:i] + suffix
        current_width += char_width
    return s

path_filter = sys.argv[1] if len(sys.argv) > 1 else ''
strict_match = sys.argv[2].lower() == 'true' if len(sys.argv) > 2 else False

# Collect all sessions from both sources
all_sessions = []

# --- Load Opencode sessions ---
try:
    db_path = os.path.join(HOME_DIR, '.local', 'share', 'opencode', 'opencode.db')
    if SQLITE3_AVAILABLE and os.path.exists(db_path):
        conn = sqlite3.connect(db_path)
        cursor = conn.cursor()
        cursor.execute('''
            SELECT s.id, s.title, s.directory, s.time_updated, COUNT(m.id) as message_count
            FROM session s
            LEFT JOIN message m ON s.id = m.session_id
            GROUP BY s.id, s.title, s.directory, s.time_updated
            ORDER BY s.time_updated DESC
            LIMIT 50
        ''')
        for rid, title, directory, time_updated, message_count in cursor.fetchall():
            # Apply path filter
            if path_filter:
                match_found = False
                path_exists = False
                try:
                    resolved_filter = os.path.realpath(os.path.expanduser(path_filter))
                    if os.path.exists(resolved_filter):
                        path_exists = True
                        if resolved_filter == directory:
                            match_found = True
                        else:
                            try:
                                resolved_dir = os.path.realpath(directory) if os.path.exists(directory) else directory
                                if resolved_filter == resolved_dir or (not strict_match and len(resolved_filter) > 10 and (resolved_filter in directory or resolved_filter in resolved_dir)):
                                    match_found = True
                            except:
                                if not strict_match and len(resolved_filter) > 10 and resolved_filter in directory:
                                    match_found = True
                except:
                    pass
                if not match_found and len(path_filter) > 2 and path_filter in directory and (not strict_match or not path_exists):
                    match_found = True
                if not match_found:
                    continue

            # Format time
            if time_updated:
                try:
                    timestamp = float(time_updated)
                    if timestamp > 9999999999:
                        timestamp = timestamp / 1000.0
                    dt = datetime.fromtimestamp(timestamp)
                    time_str = dt.strftime('%Y-%m-%d %H:%M:%S')
                    time_sort = timestamp
                except:
                    time_str = 'Invalid time'
                    time_sort = 0
            else:
                time_str = 'No time'
                time_sort = 0

            # Format directory
            dir_display = directory.replace(HOME_DIR, '~', 1) if directory.startswith(HOME_DIR) else directory

            all_sessions.append({
                'source': 'OpenCode',
                'time_sort': time_sort,
                'time_str': time_str,
                'title': str(title).replace('\t', ' ').replace('\n', ' '),
                'id': str(rid).replace('\t', ' ').replace('\n', ' '),
                'msg_count': message_count,
                'directory': directory,  # Keep full path for cd
                'directory_display': dir_display
            })
        conn.close()
except Exception as e:
    pass

# --- Load Claude sessions ---
try:
    if os.path.exists(CLAUDE_PROJECTS_DIR):
        for jsonl_file in glob.glob(os.path.join(CLAUDE_PROJECTS_DIR, '*', '*.jsonl')):
            try:
                # Extract session ID from filename
                session_id = os.path.basename(jsonl_file).replace('.jsonl', '')

                # Get working directory from first entry
                cwd = None
                title = None
                mtime = os.path.getmtime(jsonl_file)
                time_str = datetime.fromtimestamp(mtime).strftime('%Y-%m-%d %H:%M:%S')

                with open(jsonl_file, 'r') as f:
                    for line in f:
                        try:
                            data = json.loads(line.strip())
                            # Check for cwd in any type of entry
                            if cwd is None and 'cwd' in data:
                                cwd = data['cwd']

                            # Extract title: LAST custom-title > first user message
                            if data.get('type') == 'custom-title':
                                # Always update to get LAST custom-title (user may rename multiple times)
                                title = data.get('customTitle', '')
                            elif data.get('type') == 'user' and title is None:
                                # Try message.content first, then content directly
                                content = data.get('message', {}).get('content', '') if 'message' in data else data.get('content', '')
                                if isinstance(content, str):
                                    title = content[:50]
                                elif isinstance(content, list):
                                    for item in content:
                                        if isinstance(item, dict) and item.get('type') == 'text':
                                            title = item.get('text', '')[:50]
                                            break
                        except:
                            continue

                if not cwd:
                    continue

                if not title:
                    title = 'Untitled'

                # Apply path filter
                if path_filter:
                    match_found = False
                    path_exists = False
                    try:
                        resolved_filter = os.path.realpath(os.path.expanduser(path_filter))
                        if os.path.exists(resolved_filter):
                            path_exists = True
                            if resolved_filter == cwd:
                                match_found = True
                            else:
                                try:
                                    resolved_cwd = os.path.realpath(cwd) if os.path.exists(cwd) else cwd
                                    if resolved_filter == resolved_cwd or (not strict_match and len(resolved_filter) > 10 and (resolved_filter in cwd or resolved_filter in resolved_cwd)):
                                        match_found = True
                                except:
                                    if not strict_match and len(resolved_filter) > 10 and resolved_filter in cwd:
                                        match_found = True
                    except:
                        pass
                    if not match_found and len(path_filter) > 2 and path_filter in cwd and (not strict_match or not path_exists):
                        match_found = True
                    if not match_found:
                        continue

                # Count user messages
                user_message_count = 0
                with open(jsonl_file, 'r') as f:
                    for line in f:
                        try:
                            data = json.loads(line.strip())
                            if data.get('type') == 'user':
                                user_message_count += 1
                        except:
                            continue

                # Format directory
                cwd_display = cwd.replace(HOME_DIR, '~', 1) if cwd.startswith(HOME_DIR) else cwd

                all_sessions.append({
                    'source': 'Claude Code',
                    'time_sort': mtime,
                    'time_str': time_str,
                    'title': str(title).replace('\t', ' ').replace('\n', ' '),
                    'id': str(session_id).replace('\t', ' ').replace('\n', ' '),
                    'msg_count': user_message_count,
                    'directory': cwd,  # Keep full path for cd
                    'directory_display': cwd_display
                })
            except Exception as e:
                continue
except Exception as e:
    pass

# Sort by time (newest first) and limit to 50
all_sessions.sort(key=lambda x: x['time_sort'], reverse=True)
all_sessions = all_sessions[:50]

# Calculate column widths
max_time = 19
max_id = 36  # Full UUID width
max_msg = 6
max_src = 11  # 'Claude Code' width
MAX_TITLE_LIMIT = 40

max_title = MAX_TITLE_LIMIT
if all_sessions:
    actual_max = max(wcswidth(s['title']) for s in all_sessions)
    max_title = min(MAX_TITLE_LIMIT, actual_max)

# Output sessions in fzf format: Source \t ID \t Directory \t DisplayString
for session in all_sessions:
    # Truncate title if needed
    d_title = session['title']
    if wcswidth(d_title) > max_title:
        d_title = truncate_to_width(d_title, max_title)

    # Use full ID (no truncation)
    d_id = session['id']

    # Format display string with SRC column
    display_str = f\"{C_TIME}{pad(session['time_str'], max_time)}{C_END}{COL_SPACING}{C_TITLE}{pad(d_title, max_title)}{C_END}{COL_SPACING}{C_ID}{pad(d_id, max_id)}{C_END}{COL_SPACING}{C_MSG}{pad(str(session['msg_count']), max_msg)}{C_END}{COL_SPACING}{C_SRC}{pad(session['source'], max_src)}{C_END}{COL_SPACING}{C_DIR}{session['directory_display']}{C_END}\"

    # Output: Source \t ID \t Directory \t DisplayString
    print(f\"{session['source']}\t{session['id']}\t{session['directory']}\t{display_str}\")
" "$path_filter" "$strict_match"
}

# Check if fzf supports border-left option (introduced in fzf 0.27.0)
fzf_supports_border_left() {
	local version=$(fzf --version 2>/dev/null | awk '{print $1}')
	local major=$(echo "$version" | cut -d. -f1)
	local minor=$(echo "$version" | cut -d. -f2)

	# border-left introduced in fzf 0.27.0
	if [ "$major" -gt 0 ]; then
		return 0  # Any version >= 1.0.0 supports it
	elif [ "$major" -eq 0 ] && [ "$minor" -ge 27 ]; then
		return 0  # Version 0.27.0 or higher supports it
	else
		return 1  # Older versions don't support it
	fi
}

# Check if fzf supports focus event (introduced in fzf 0.31.0)
fzf_supports_focus_event() {
	local version=$(fzf --version 2>/dev/null | awk '{print $1}')
	local major=$(echo "$version" | cut -d. -f1)
	local minor=$(echo "$version" | cut -d. -f2)

	# focus event introduced in fzf 0.31.0
	if [ "$major" -gt 0 ]; then
		return 0  # Any version >= 1.0.0 supports it
	elif [ "$major" -eq 0 ] && [ "$minor" -ge 31 ]; then
		return 0  # Version 0.31.0 or higher supports it
	else
		return 1  # Older versions don't support it
	fi
}

# Generate preview command with fallback chain: eza -> ls (BSD) -> ls (GNU)
#
# GOTCHA: ls -l output format differs between BSD (macOS) and GNU (Linux).
# We use sed to strip permission and link-count columns for cleaner display.
# Preview functions for fzf (exported for subshell access)
preview_opencode_session() {
	local session_id="$1"
	local directory="$2"

	# Find database
	local DB_PATH=""
	[[ -f "$HOME/.opencode/storage" ]] && DB_PATH="$HOME/.opencode/storage"
	[[ -f "$HOME/.config/Opencode/storage" ]] && DB_PATH="$HOME/.config/Opencode/storage"

	# Show session info
	if [[ -n "$DB_PATH" ]]; then
		local info=$(sqlite3 "$DB_PATH" "SELECT title, time_updated, (SELECT COUNT(*) FROM message WHERE session_id='$session_id') FROM session WHERE id='$session_id' LIMIT 1" 2>/dev/null)
		if [[ -n "$info" ]]; then
			local title=$(echo "$info" | cut -d"|" -f1)
			local time=$(echo "$info" | cut -d"|" -f2)
			local msg_count=$(echo "$info" | cut -d"|" -f3)

			# Format time
			if [[ -n "$time" ]]; then
				if [[ "$OSTYPE" == darwin* ]]; then
					time=$(date -r "$time" "+%Y-%m-%d %H:%M:%S" 2>/dev/null)
				else
					time=$(date -d "@$time" "+%Y-%m-%d %H:%M:%S" 2>/dev/null)
				fi
			fi

			echo -e "\033[1;36m━━━━━━━━━━━━━━━ SESSION INFO ━━━━━━━━━━━━━━━\033[0m"
			echo -e "\033[1;33mTitle:\033[0m     $title"
			echo -e "\033[1;32mTime:\033[0m      $time"
			echo -e "\033[1;35mMessages:\033[0m  $msg_count"
			echo -e "\033[1;90mDirectory:\033[0m $directory"
			echo -e "\033[1;36m━━━━━━━━━━━━━━ DIRECTORY LIST ━━━━━━━━━━━━━━\033[0m"
			echo ""
		fi
	fi

	# Show directory listing
	if command -v eza >/dev/null 2>&1; then
		eza -lF --time-style="+%Y-%m-%d %H:%M:%S" --group-directories-first --binary --color=always --no-permissions --no-user -M "$directory" 2>/dev/null
	elif [[ "$OSTYPE" == darwin* ]]; then
		ls -lF -D "%Y-%m-%d %H:%M:%S" -h --color=always -o -g "$directory" 2>/dev/null | sed -E "s/^[^ ]+ +[^ ]+ //"
	else
		ls -lF --time-style="+%Y-%m-%d %H:%M:%S" --group-directories-first -h --color=always -o -g "$directory" 2>/dev/null | sed -E "s/^[^ ]+ +[^ ]+ //"
	fi
}

preview_claude_session() {
	local session_id="$1"
	local project_path="$2"
	local working_dir="$3"

	local jsonl_file="$project_path/$session_id.jsonl"

	if [[ -f "$jsonl_file" ]]; then
		# Get modification time
		if [[ "$OSTYPE" == darwin* ]]; then
			local time=$(stat -f "%Sm" -t "%Y-%m-%d %H:%M:%S" "$jsonl_file" 2>/dev/null)
		else
			local time=$(stat -c "%y" "$jsonl_file" 2>/dev/null | cut -d"." -f1)
		fi

		# Use jq to extract title, message count, and recent messages in one pass
		local info=$(jq -s -r '
			# Get LAST custom-title (user may rename multiple times)
			(map(select(.type == "custom-title")) | last | .customTitle) //
			# Fallback: first user message
			(map(select(.type == "user" and .message.role == "user"))[0].message.content |
			 if type == "string" then .[:50]
			 elif type == "array" then (.[0].text // "Untitled")[:50]
			 else "Untitled" end) //
			"Untitled" as $title |
			# Count user messages
			[.[] | select(.type == "user" and .message.role == "user")] | length as $count |
			# Get last 10 user messages (text only, reverse order for newest first)
			([.[] | select(.type == "user" and .message.role == "user" and
				(.message.content | type == "string") and
				(.message.content | startswith("<local-command-caveat>") or
				 startswith("<command-message>") or
				 startswith("<command-name>") or
				 startswith("[Request interrupted") | not))] |
			 .[-10:] | reverse | map(.message.content[:80]) | join("|||")) as $recent |
			"\($title)|\($count)|\($recent)"
		' "$jsonl_file" 2>/dev/null)

		# Parse jq output
		local title="${info%%|*}"
		local rest="${info#*|}"
		local msg_count="${rest%%|*}"
		local recent="${rest#*|}"

		echo -e "\033[1;36m━━━ SESSION INFO ━━━\033[0m"
		echo -e "\033[1;33mTitle:\033[0m     $title"
		echo -e "\033[1;32mTime:\033[0m      $time"
		echo -e "\033[1;35mMessages:\033[0m  $msg_count"
		echo -e "\033[1;90mDirectory:\033[0m $working_dir"

		# Display recent messages if available
		if [[ -n "$recent" ]]; then
			echo -e "\033[1;36m━━━ RECENT MESSAGES ━━━\033[0m"
			IFS='|||' read -ra msgs <<< "$recent"
			for msg in "${msgs[@]}"; do
				[[ -n "$msg" ]] && echo -e "\033[1;90m•\033[0m $msg"
			done
		fi

		echo -e "\033[1;36m━━━ DIRECTORY LIST ━━━\033[0m"
		echo ""
	fi

	# Show directory listing
	if command -v eza >/dev/null 2>&1; then
		eza -lF --time-style="+%Y-%m-%d %H:%M:%S" --group-directories-first --binary --color=always --no-permissions --no-user -M "$working_dir" 2>/dev/null
	elif [[ "$OSTYPE" == darwin* ]]; then
		ls -lF -D "%Y-%m-%d %H:%M:%S" -h --color=always -o -g "$working_dir" 2>/dev/null | sed -E "s/^[^ ]+ +[^ ]+ //"
	else
		ls -lF --time-style="+%Y-%m-%d %H:%M:%S" --group-directories-first -h --color=always -o -g "$working_dir" 2>/dev/null | sed -E "s/^[^ ]+ +[^ ]+ //"
	fi
}

select_opencode_session() {
	local path_filter="$1"
	local strict_match="$2"
	# --- fzf Field Structure ---
	# Input format (TAB separated):
	#   Field 1: ID (Hidden)
	#   Field 2: Directory (Hidden, used for preview)
	#   Field 3: Display String (Visible)
	#
	# fzf options:
	#   --delimiter='\t'  : Parse fields by TAB (NOT by COLUMN_SPACING!)
	#   --with-nth=3      : Show only field 3 (the DisplayString)
	#   --preview='..{2}.': Use field 2 (directory) for preview
	#
	# After selection: Extract field 1 (ID) to launch session

	# Build preview-window option based on fzf version
	local preview_opts="right:${PREVIEW_WIDTH}:wrap"
	if fzf_supports_border_left; then
		preview_opts="${preview_opts}:border-left"
	fi

	# Build optional bind args array
	local bind_args=()
	if fzf_supports_focus_event; then
		bind_args+=(--bind 'focus:transform-preview-label:echo " [ {2} ] "')
	fi

	generate_opencode_interactive_list "$path_filter" "$strict_match" | fzf --ansi \
		--header="Select Session (Enter to Launch)" \
		--reverse \
		--delimiter='\t' \
		--with-nth=3 \
		--preview="bash -c 'source \"\$SCRIPT_SELF\" && preview_opencode_session {1} {2}'" \
		--preview-window="$preview_opts" \
		"${bind_args[@]}" \
		--height=90% \
		--border
}

select_claude_session() {
	local path_filter="$1"
	local strict_match="$2"
	# --- fzf Field Structure ---
	# Input format (TAB separated):
	#   Field 1: Session ID (Hidden)
	#   Field 2: Project Path (Hidden, for launching)
	#   Field 3: Working Directory (Hidden, used for preview)
	#   Field 4: Display String (Visible)
	#
	# fzf options:
	#   --delimiter='\t'  : Parse fields by TAB (NOT by COLUMN_SPACING!)
	#   --with-nth=4      : Show only field 4 (the DisplayString)
	#   --preview='..{3}.': Use field 3 (working directory) for preview
	#
	# After selection: Extract field 1 (Session ID) to launch session

	# Build preview-window option based on fzf version
	local preview_opts="right:${PREVIEW_WIDTH}:wrap"
	if fzf_supports_border_left; then
		preview_opts="${preview_opts}:border-left"
	fi

	# Build optional bind args array
	local bind_args=()
	if fzf_supports_focus_event; then
		bind_args+=(--bind 'focus:transform-preview-label:echo " [ {3} ] "')
	fi

	generate_claude_interactive_list "$path_filter" "$strict_match" | fzf --ansi \
		--header="Select Claude Code Session (Enter to Launch)" \
		--reverse \
		--delimiter='\t' \
		--with-nth=4 \
		--preview="bash -c 'source \"\$SCRIPT_SELF\" && preview_claude_session {1} {2} {3}'" \
		--preview-window="$preview_opts" \
		"${bind_args[@]}" \
		--height=90% \
		--border
}

select_all_session() {
	local path_filter="$1"
	local strict_match="$2"
	# --- fzf Field Structure ---
	# Input format (TAB separated):
	#   Field 1: Source (OC/CC) - client identifier
	#   Field 2: Session ID (Hidden)
	#   Field 3: Directory (Hidden, used for preview and cd)
	#   Field 4: Display String (Visible, includes SRC column)
	#
	# fzf options:
	#   --delimiter='\t'  : Parse fields by TAB (NOT by COLUMN_SPACING!)
	#   --with-nth=4      : Show only field 4 (the DisplayString with SRC column)
	#   --preview='..{3}.': Use field 3 (directory) for preview
	#
	# After selection: Extract field 1 (source) to determine which client to launch

	# Build preview-window option based on fzf version
	local preview_opts="right:${PREVIEW_WIDTH}:wrap"
	if fzf_supports_border_left; then
		preview_opts="${preview_opts}:border-left"
	fi

	# Build optional bind args array
	local bind_args=()
	if fzf_supports_focus_event; then
		bind_args+=(--bind 'focus:transform-preview-label:echo " [ {3} ] "')
	fi

	generate_all_interactive_list "$path_filter" "$strict_match" | fzf --ansi \
		--header="Select Session (Enter to Launch)" \
		--reverse \
		--delimiter='\t' \
		--with-nth=4 \
		--preview="bash -c 'source \"\$SCRIPT_SELF\" && preview_claude_session {1} {2} {3}'" \
		--preview-window="$preview_opts" \
		"${bind_args[@]}" \
		--height=90% \
		--border
}

# --- Guard: Skip main logic when sourced (for fzf preview functions) ---
# When this script is sourced (e.g., in fzf preview subshell), only define
# functions without executing main logic. This prevents recursive fzf calls.
if [[ "${BASH_SOURCE[0]}" != "${0}" ]]; then
	return
fi

# --- Main Entry Point ---
# Flow:
#   1. Parse arguments (LIST_ONLY, CLAUDE_MODE, PATH_FILTER, etc.)
#   2. If -l: Call generate_*_list_with_time() and exit
#   3. Else: Call select_*_session() for fzf interaction
#   4. On selection: Extract session ID and launch

# If list-only mode, just display the table and exit
if $LIST_ONLY; then
	# Determine which clients to show
	if $ALL_MODE || ($CLAUDE_MODE && $OPENCODE_MODE); then
		# Show both Opencode and Claude sessions
		generate_all_list_with_time "$PATH_FILTER" "$STRICT_MATCH"
	elif $CLAUDE_MODE; then
		# Show Claude sessions only
		generate_claude_list_with_time "$PATH_FILTER" "$STRICT_MATCH"
	else
		# Show Opencode sessions only (default or explicit -o)
		generate_opencode_list_with_time "$PATH_FILTER" "$STRICT_MATCH"
	fi
	exit 0
fi

# Interactive mode - select and launch session
if $ALL_MODE || ($CLAUDE_MODE && $OPENCODE_MODE); then
	# Combined selection mode - select from both Opencode and Claude sessions
	SELECTED_LINE=$(select_all_session "$PATH_FILTER" "$STRICT_MATCH")

	if [ -z "$SELECTED_LINE" ]; then
		exit 0 # User cancelled fzf
	fi

	# Extract fields: Source \t ID \t Directory \t DisplayString
	SOURCE=$(echo "$SELECTED_LINE" | cut -f1)
	SELECTED_ID=$(echo "$SELECTED_LINE" | cut -f2)
	TARGET_DIR=$(echo "$SELECTED_LINE" | cut -f3)

	# Verify directory exists
	if [ ! -d "$TARGET_DIR" ]; then
		echo "Error: Directory not found: $TARGET_DIR"
		exit 1
	fi

	# Launch based on source
	if [ "$SOURCE" = "CC" ]; then
		# Claude Code session
		CLAUDE_CMD="claude --resume \"$SELECTED_ID\""
		if $DANGER_MODE; then
			CLAUDE_CMD="claude --dangerously-skip-permissions --resume \"$SELECTED_ID\""
		fi

		if $NO_LAUNCH; then
			if $VERBOSE; then
				# If --no-launch and --verbose, print the full command
				echo "cd \"$TARGET_DIR\" && $CLAUDE_CMD"
			else
				# If only --no-launch, just print the directory
				echo "$TARGET_DIR"
			fi
			exit 0
		fi

		echo "Resuming Claude Code session: $SELECTED_ID"
		echo "Directory: $TARGET_DIR"
		if $DANGER_MODE; then
			echo -e "\033[31mWARNING: DANGER MODE: Skipping all permissions checks\033[0m"
		fi

		sleep 0.5

		# Change directory first
		cd "$TARGET_DIR" || exit 1

		# Check for claude availability
		if ! command -v claude &>/dev/null; then
			echo "Error: claude command not found in PATH."
			exec "$DEFAULT_SHELL"
		fi

		# Launch Claude Code with session ID (and optional danger flag)
		if $DANGER_MODE; then
			exec claude --dangerously-skip-permissions --resume "$SELECTED_ID"
		else
			exec claude --resume "$SELECTED_ID"
		fi
	else
		# Opencode session (SOURCE = "OC")
		if $NO_LAUNCH; then
			if $VERBOSE; then
				# If --no-launch and --verbose, print the full command
				echo "cd \"$TARGET_DIR\" && opencode -s \"$SELECTED_ID\""
			else
				# If only --no-launch, just print the directory
				echo "$TARGET_DIR"
			fi
			exit 0
		fi

		echo "Resuming Opencode session: $SELECTED_ID"
		echo "Directory: $TARGET_DIR"

		sleep 0.5

		# Change directory first
		cd "$TARGET_DIR" || exit 1

		# Check for opencode availability
		if ! command -v opencode &>/dev/null; then
			echo "Error: opencode command not found in PATH."
			exec "$DEFAULT_SHELL"
		fi

		# Launch opencode with session ID
		exec opencode -s "$SELECTED_ID"
	fi
elif $CLAUDE_MODE; then
	# Claude Code session selection
	SELECTED_LINE=$(select_claude_session "$PATH_FILTER" "$STRICT_MATCH")

	if [ -z "$SELECTED_LINE" ]; then
		exit 0 # User cancelled fzf
	fi

	# Extract fields from tab-separated output
	SELECTED_ID=$(echo "$SELECTED_LINE" | cut -f1)
	PROJECT_PATH=$(echo "$SELECTED_LINE" | cut -f2)
	TARGET_DIR=$(echo "$SELECTED_LINE" | cut -f3)

	# Verify directory exists
	if [ ! -d "$TARGET_DIR" ]; then
		echo "Error: Directory not found: $TARGET_DIR"
		exit 1
	fi

	# Prepare claude command with optional danger flag
	CLAUDE_CMD="claude --resume \"$SELECTED_ID\""
	if $DANGER_MODE; then
		CLAUDE_CMD="claude --dangerously-skip-permissions --resume \"$SELECTED_ID\""
	fi

	if $NO_LAUNCH; then
		if $VERBOSE; then
			# If --no-launch and --verbose, print the full command
			echo "cd \"$TARGET_DIR\" && $CLAUDE_CMD"
		else
			# If only --no-launch, just print the directory
			echo "$TARGET_DIR"
		fi
		exit 0
	fi

	echo "Resuming Claude Code session: $SELECTED_ID"
	echo "Directory: $TARGET_DIR"
	if $DANGER_MODE; then
		echo -e "\033[31mWARNING: DANGER MODE: Skipping all permissions checks\033[0m"
	fi

	sleep 0.5

	# Change directory first
	cd "$TARGET_DIR" || exit 1

	# Check for claude availability
	if ! command -v claude &>/dev/null; then
		echo "Error: claude command not found in PATH."
		exec "$DEFAULT_SHELL"
	fi

	# Launch Claude Code with session ID (and optional danger flag)
	if $DANGER_MODE; then
		exec claude --dangerously-skip-permissions --resume "$SELECTED_ID"
	else
		exec claude --resume "$SELECTED_ID"
	fi
else
	# Opencode session selection
	SELECTED_LINE=$(select_opencode_session "$PATH_FILTER" "$STRICT_MATCH")

	if [ -z "$SELECTED_LINE" ]; then
		exit 0 # User cancelled fzf
	fi

	# Extract ID and Directory directly from the tab-separated fields
	SELECTED_ID=$(echo "$SELECTED_LINE" | cut -f1)
	TARGET_DIR=$(echo "$SELECTED_LINE" | cut -f2)

	# Verify directory exists
	if [ ! -d "$TARGET_DIR" ]; then
		echo "Error: Directory not found: $TARGET_DIR"
		exit 1
	fi

	if $NO_LAUNCH; then
		if $VERBOSE; then
			# If --no-launch and --verbose, print the full command
			echo "cd \"$TARGET_DIR\" && opencode -s \"$SELECTED_ID\""
		else
			# If only --no-launch, just print the directory
			echo "$TARGET_DIR"
		fi
		exit 0
	fi

	echo "Resuming session: $SELECTED_ID"
	echo "Directory: $TARGET_DIR"

	sleep 0.5

	# Change directory first
	cd "$TARGET_DIR" || exit 1

	# Check for opencode availability
	if ! command -v opencode &>/dev/null; then
		echo "Error: opencode command not found in PATH."
		exec "$DEFAULT_SHELL"
	fi

	# Launch opencode with session ID
	exec opencode -s "$SELECTED_ID"
fi
