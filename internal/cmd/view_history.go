package cmd

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// usageHistoryFile is the on-disk sample log, relative to the town root.
// Persisted (rather than kept in memory) so the dashboard's usage chart
// survives an `ms view` restart.
const usageHistoryFile = ".usage-history.jsonl"

// usageHistoryRetention bounds the log. Samples older than this are dropped
// on the next write, so the file self-prunes and never needs rotation.
const usageHistoryRetention = 7 * 24 * time.Hour

// usageSampleInterval is how often the sampler records a point. fetchUsage
// caches for a minute, so sampling faster would just re-record the same value.
const usageSampleInterval = time.Minute

// usagePoint is one sample of the usage windows.
type usagePoint struct {
	TS      time.Time `json:"ts"`
	Pct     float64   `json:"pct"`      // 5h session window, 0-100
	WeekPct float64   `json:"week_pct"` // 7-day window, 0-100
}

// usageHistoryMu serialises read-modify-write of the history file. Only this
// process writes it, so a mutex is sufficient — no file locking needed.
var usageHistoryMu sync.Mutex

func usageHistoryPath(townRoot string) string {
	return filepath.Join(townRoot, usageHistoryFile)
}

// startUsageSampler records a usage point on an interval for the life of the
// process. Errors are ignored: a missing history file degrades the chart, it
// should never take down the view server.
func startUsageSampler(townRoot string) {
	go func() {
		appendUsagePoint(townRoot, fetchUsage())
		for range time.Tick(usageSampleInterval) {
			appendUsagePoint(townRoot, fetchUsage())
		}
	}()
}

// appendUsagePoint adds one sample and prunes anything past the retention
// window. It rewrites the whole file, which is fine at this size: one point a
// minute for seven days is ~10k lines.
func appendUsagePoint(townRoot string, u viewUsage) {
	if !u.OK {
		return // don't record points while the usage API is unreachable
	}

	usageHistoryMu.Lock()
	defer usageHistoryMu.Unlock()

	points := readUsageHistory(townRoot)
	points = append(points, usagePoint{
		TS:      time.Now().UTC(),
		Pct:     u.Utilization,
		WeekPct: u.WeekUtilization,
	})

	cutoff := time.Now().UTC().Add(-usageHistoryRetention)
	kept := points[:0]
	for _, p := range points {
		if p.TS.After(cutoff) {
			kept = append(kept, p)
		}
	}

	path := usageHistoryPath(townRoot)
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, p := range kept {
		if err := enc.Encode(p); err != nil {
			f.Close()
			os.Remove(tmp)
			return
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		os.Remove(tmp)
		return
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return
	}
	// Rename over the original so a crash mid-write can't truncate history.
	// Windows won't replace an existing file via Rename, so drop it first.
	os.Remove(path)
	os.Rename(tmp, path)
}

// readUsageHistory loads all recorded points, oldest first. A malformed line
// is skipped rather than failing the whole read.
func readUsageHistory(townRoot string) []usagePoint {
	f, err := os.Open(usageHistoryPath(townRoot))
	if err != nil {
		return nil
	}
	defer f.Close()

	var points []usagePoint
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var p usagePoint
		if err := json.Unmarshal(line, &p); err != nil {
			continue
		}
		points = append(points, p)
	}
	return points
}
