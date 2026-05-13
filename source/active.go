package source

import (
	"os"
	"sort"
	"time"

	"github.com/gadflysu/aps/dbg"
)

// ActiveResult holds the outcome of DetectActive split by confidence level.
// Confirmed: session identified via pid+lstart cache — certain.
// Guessed: session identified via CWD+mtime fallback — probable but not certain.
type ActiveResult struct {
	Confirmed map[string]bool // session IDs matched by cache
	Guessed   map[string]bool // session IDs matched by CWD fallback
}

// DetectActive returns session IDs that are currently active, split by confidence.
//
// For each running claude/opencode process:
//  1. If the cache has a pid+lstart → sessionID mapping, use it directly (Confirmed).
//  2. Otherwise fall back: session CWD must match process CWD AND
//     last-activity timestamp must be today (>= local midnight) (Guessed).
//
// procs must be pre-collected by the caller (avoids duplicate CollectProcs calls).
// Errors from CollectProcs are silently ignored — callers get empty maps on failure.
func DetectActive(sessions []Session, procs []ProcInfo, cache *PIDCache) ActiveResult {
	res := ActiveResult{
		Confirmed: make(map[string]bool),
		Guessed:   make(map[string]bool),
	}
	if len(sessions) == 0 {
		return res
	}

	todayMidnight := todayMidnight()

	// Build sessionID lookup map.
	byID := make(map[string]Session, len(sessions))
	for _, s := range sessions {
		byID[s.ID] = s
	}

	// Pass 1: cache-precise matches — any proc whose pid+lstart is known.
	if cache != nil {
		for _, p := range procs {
			sid := cache.Lookup(p)
			if sid == "" {
				continue
			}
			if _, ok := byID[sid]; ok {
				res.Confirmed[sid] = true
			}
		}
	}

	// Pass 2: fallback for procs with no cache entry.
	// Count unmapped procs per CWD — each slot can account for one guessed session.
	unmappedSlots := make(map[string]int)
	for _, p := range procs {
		if cache == nil || cache.Lookup(p) == "" {
			unmappedSlots[p.CWD]++
		}
	}

	// Filter to sessions eligible for guessing (correct CWD, today, not confirmed).
	// Sort by Time descending so most-recently-active sessions consume slots first.
	eligible := make([]Session, 0, len(sessions))
	for _, s := range sessions {
		if res.Confirmed[s.ID] {
			continue
		}
		if unmappedSlots[s.CWD] == 0 {
			continue
		}
		switch s.Client {
		case ClientClaude:
			if s.jsonlPath == "" {
				continue
			}
			info, err := os.Stat(s.jsonlPath)
			if err != nil {
				dbg.Log("[DetectActive] skip %s (stat error: %v)", s.ID, err)
				continue
			}
			if info.ModTime().Before(todayMidnight) {
				continue
			}
			eligible = append(eligible, s)
		case ClientOpencode:
			if s.Time.Before(todayMidnight) {
				continue
			}
			eligible = append(eligible, s)
		}
	}
	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].Time.After(eligible[j].Time)
	})
	for _, s := range eligible {
		if unmappedSlots[s.CWD] <= 0 {
			continue
		}
		res.Guessed[s.ID] = true
		unmappedSlots[s.CWD]--
	}
	return res
}

// todayMidnight returns 00:00:00 of today in local time.
func todayMidnight() time.Time {
	now := time.Now()
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location())
}
