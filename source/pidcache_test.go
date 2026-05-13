package source

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestPIDCache_GC_RemovesDeadPID(t *testing.T) {
	dir := t.TempDir()
	c := &PIDCache{
		path:    filepath.Join(dir, "pid-session.txt"),
		entries: map[string]string{
			"99999999|999": "some-session-id",
		},
	}
	var wg sync.WaitGroup
	wg.Add(1)
	c.GC(&wg)
	wg.Wait()
	if _, ok := c.entries["99999999|999"]; ok {
		t.Error("GC should have removed the dead PID entry")
	}
}

func TestCollectProcs_ReturnsValidShape(t *testing.T) {
	procs := CollectProcs()
	// May be empty if no claude/opencode running — that's fine.
	for _, p := range procs {
		if p.PID == "" {
			t.Error("ProcInfo.PID must not be empty")
		}
		if p.LStart == "" {
			t.Error("ProcInfo.LStart must not be empty")
		}
		if p.CWD == "" {
			t.Error("ProcInfo.CWD must not be empty")
		}
	}
}
