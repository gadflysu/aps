package cmd

import (
	"testing"
)

// --- expandShortFlags ---

func TestExpandShortFlags_NoChange(t *testing.T) {
	args := []string{"-n", "--verbose", "somepath"}
	got := expandShortFlags(args)
	if len(got) != len(args) {
		t.Fatalf("expandShortFlags: got %v, want %v", got, args)
	}
	for i, a := range args {
		if got[i] != a {
			t.Errorf("expandShortFlags[%d] = %q, want %q", i, got[i], a)
		}
	}
}

func TestExpandShortFlags_CombinedTwo(t *testing.T) {
	got := expandShortFlags([]string{"-nv"})
	want := []string{"-n", "-v"}
	if len(got) != len(want) {
		t.Fatalf("expandShortFlags(-nv) = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("expandShortFlags(-nv)[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestExpandShortFlags_CombinedThree(t *testing.T) {
	got := expandShortFlags([]string{"-nla"})
	want := []string{"-n", "-l", "-a"}
	if len(got) != len(want) {
		t.Fatalf("expandShortFlags(-nla) = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("expandShortFlags(-nla)[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestExpandShortFlags_LongFlagUnchanged(t *testing.T) {
	got := expandShortFlags([]string{"--no-launch"})
	if len(got) != 1 || got[0] != "--no-launch" {
		t.Errorf("expandShortFlags(--no-launch) = %v, want [--no-launch]", got)
	}
}

func TestExpandShortFlags_Mixed(t *testing.T) {
	got := expandShortFlags([]string{"-nv", "--all", "-l"})
	want := []string{"-n", "-v", "--all", "-l"}
	if len(got) != len(want) {
		t.Fatalf("expandShortFlags mixed = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d] = %q, want %q", i, got[i], w)
		}
	}
}

// --- Parse ---

func TestParse_DefaultsToClaudeWhenNoClientFlag(t *testing.T) {
	cfg := Parse([]string{})
	if !cfg.Claude {
		t.Error("Parse with no client flags should default to Claude=true")
	}
	if cfg.Opencode {
		t.Error("Parse with no client flags should not set Opencode")
	}
}

func TestParse_ClaudeFlag(t *testing.T) {
	cfg := Parse([]string{"-c"})
	if !cfg.Claude {
		t.Error("-c should set Claude=true")
	}
	cfg2 := Parse([]string{"--claude"})
	if !cfg2.Claude {
		t.Error("--claude should set Claude=true")
	}
}

func TestParse_OpencodeFlag(t *testing.T) {
	cfg := Parse([]string{"-o"})
	if !cfg.Opencode {
		t.Error("-o should set Opencode=true")
	}
	if cfg.Claude {
		t.Error("-o alone should not set Claude=true (it was explicitly set)")
	}
}

func TestParse_AllFlag(t *testing.T) {
	cfg := Parse([]string{"-a"})
	if !cfg.All {
		t.Error("-a should set All=true")
	}
	if !cfg.Claude || !cfg.Opencode {
		t.Error("-a should set both Claude=true and Opencode=true")
	}
}

func TestParse_NoLaunchFlag(t *testing.T) {
	cfg := Parse([]string{"-n"})
	if !cfg.NoLaunch {
		t.Error("-n should set NoLaunch=true")
	}
}

func TestParse_ListFlag(t *testing.T) {
	cfg := Parse([]string{"-l"})
	if !cfg.ListOnly {
		t.Error("-l should set ListOnly=true")
	}
}

func TestParse_VerboseFlag(t *testing.T) {
	cfg := Parse([]string{"-v"})
	if !cfg.Verbose {
		t.Error("-v should set Verbose=true")
	}
}

func TestParse_DangerFlag(t *testing.T) {
	cfg := Parse([]string{"-d"})
	if !cfg.DangerMode {
		t.Error("-d should set DangerMode=true")
	}
}

func TestParse_RecursiveFlag(t *testing.T) {
	cfg := Parse([]string{"-r"})
	if !cfg.Recursive {
		t.Error("-r should set Recursive=true")
	}
}

func TestParse_PathFilterPositionalArg(t *testing.T) {
	cfg := Parse([]string{"/some/path"})
	if cfg.PathFilter != "/some/path" {
		t.Errorf("PathFilter = %q, want \"/some/path\"", cfg.PathFilter)
	}
}

func TestParse_CombinedFlags(t *testing.T) {
	cfg := Parse([]string{"-nv"})
	if !cfg.NoLaunch {
		t.Error("-nv should set NoLaunch=true")
	}
	if !cfg.Verbose {
		t.Error("-nv should set Verbose=true")
	}
}

func TestParse_ClaudeCmdFlag(t *testing.T) {
	cfg := Parse([]string{"--claude-cmd", "cc"})
	if cfg.ClaudeCmd != "cc" {
		t.Errorf("ClaudeCmd = %q, want \"cc\"", cfg.ClaudeCmd)
	}
}

func TestParse_OpencodeCmdFlag(t *testing.T) {
	cfg := Parse([]string{"-o", "--opencode-cmd", "npx opencode@1.0"})
	if cfg.OpencodeCmd != "npx opencode@1.0" {
		t.Errorf("OpencodeCmd = %q, want \"npx opencode@1.0\"", cfg.OpencodeCmd)
	}
}

func TestParse_CmdFlagSingleClaudeDefault(t *testing.T) {
	cfg := Parse([]string{"--cmd", "cc"})
	if cfg.ClaudeCmd != "cc" {
		t.Errorf("ClaudeCmd = %q, want \"cc\" (default client)", cfg.ClaudeCmd)
	}
}

func TestParse_CmdFlagSingleExplicitClaude(t *testing.T) {
	cfg := Parse([]string{"-c", "--cmd", "cc"})
	if cfg.ClaudeCmd != "cc" {
		t.Errorf("ClaudeCmd = %q, want \"cc\"", cfg.ClaudeCmd)
	}
}

func TestParse_CmdFlagSingleOpencode(t *testing.T) {
	cfg := Parse([]string{"-o", "--cmd", "npx opencode@1.0"})
	if cfg.OpencodeCmd != "npx opencode@1.0" {
		t.Errorf("OpencodeCmd = %q, want \"npx opencode@1.0\"", cfg.OpencodeCmd)
	}
}
