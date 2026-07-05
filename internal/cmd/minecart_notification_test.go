package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNotifyMinecartCompletion_StampsAndSkipsDuplicate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	binDir := t.TempDir()
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	statePath := filepath.Join(binDir, "notified.state")
	mailLogPath := filepath.Join(binDir, "mail.log")
	exportLogPath := filepath.Join(binDir, "export.log")
	bdPath := filepath.Join(binDir, "bd")
	gtPath := filepath.Join(binDir, "ms")

	bdScript := `#!/bin/sh
STATE="` + statePath + `"
EXPORT_LOG="` + exportLogPath + `"
if [ "$1" = "--allow-stale" ]; then
  shift
fi
case "$1" in
  version)
    exit 0
    ;;
  show)
    if [ -f "$STATE" ]; then
      printf '%s\n' '[{"id":"hq-cv-dup","description":"Owner: overseer/\ncompletion_notified_at: 2026-05-25T02:30:00Z","created_at":"2026-05-25T02:00:00Z"}]'
    else
      printf '%s\n' '[{"id":"hq-cv-dup","description":"Owner: overseer/","created_at":"2026-05-25T02:00:00Z"}]'
    fi
    exit 0
    ;;
  update)
    touch "$STATE"
    exit 0
    ;;
  export)
    echo "$@" >> "$EXPORT_LOG"
    exit 0
    ;;
  sql)
    printf '%s\n' '[]'
    exit 0
    ;;
esac
exit 0
`
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	gtScript := `#!/bin/sh
if [ "$1" = "mail" ] && [ "$2" = "send" ]; then
  echo "$@" >> "` + mailLogPath + `"
fi
exit 0
`
	if err := os.WriteFile(gtPath, []byte(gtScript), 0755); err != nil {
		t.Fatalf("write ms stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	notifyMinecartCompletion(townRoot, "hq-cv-dup", "Duplicate Guard")
	notifyMinecartCompletion(townRoot, "hq-cv-dup", "Duplicate Guard")

	data, err := os.ReadFile(mailLogPath)
	if err != nil {
		t.Fatalf("read mail log: %v", err)
	}
	if got := strings.Count(string(data), "mail send"); got != 1 {
		t.Fatalf("mail sends = %d, want 1; log:\n%s", got, string(data))
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("completion notification state was not recorded: %v", err)
	}
	exportData, err := os.ReadFile(exportLogPath)
	if err != nil {
		t.Fatalf("read export log: %v", err)
	}
	if got := strings.Count(string(exportData), "export -o"); got != 1 {
		t.Fatalf("bd export calls = %d, want 1; log:\n%s", got, string(exportData))
	}
	if !strings.Contains(string(exportData), filepath.Join(townRoot, ".beads", "issues.jsonl")) {
		t.Fatalf("bd export did not target town issues.jsonl; log:\n%s", string(exportData))
	}
}

func TestCloseMinecartIfComplete_ExportsJSONLBeforeNotification(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	binDir := t.TempDir()
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	orderPath := filepath.Join(binDir, "order.log")
	bdPath := filepath.Join(binDir, "bd")
	gtPath := filepath.Join(binDir, "ms")

	bdScript := `#!/bin/sh
ORDER="` + orderPath + `"
if [ "$1" = "--allow-stale" ]; then
  shift
fi
case "$1" in
  version)
    exit 0
    ;;
  close)
    echo close >> "$ORDER"
    exit 0
    ;;
  export)
    echo export:"$@" >> "$ORDER"
    exit 0
    ;;
  show)
    printf '%s\n' '[{"id":"hq-cv-done","description":"Owner: overseer/","created_at":"2026-05-25T02:00:00Z"}]'
    exit 0
    ;;
  update)
    echo update >> "$ORDER"
    exit 0
    ;;
  sql)
    printf '%s\n' '[]'
    exit 0
    ;;
esac
exit 1
`
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	gtScript := `#!/bin/sh
if [ "$1" = "mail" ] && [ "$2" = "send" ]; then
  echo mail >> "` + orderPath + `"
fi
exit 0
`
	if err := os.WriteFile(gtPath, []byte(gtScript), 0755); err != nil {
		t.Fatalf("write ms stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	closed, err := closeMinecartIfComplete(townRoot, "hq-cv-done", "Done Minecart", []trackedIssueInfo{
		{ID: "ms-done", Status: "closed"},
	}, false)
	if err != nil {
		t.Fatalf("closeMinecartIfComplete returned error: %v", err)
	}
	if !closed {
		t.Fatal("closeMinecartIfComplete returned closed=false, want true")
	}

	data, err := os.ReadFile(orderPath)
	if err != nil {
		t.Fatalf("read order log: %v", err)
	}
	got := strings.TrimSpace(string(data))
	want := strings.Join([]string{
		"close",
		"export:export -o " + filepath.Join(townRoot, ".beads", "issues.jsonl"),
		"mail",
		"update",
		"export:export -o " + filepath.Join(townRoot, ".beads", "issues.jsonl"),
	}, "\n")
	if got != want {
		t.Fatalf("operation order mismatch:\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestNotifyMinecartCompletion_ExportFailureDoesNotPreventMail(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	binDir := t.TempDir()
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	orderPath := filepath.Join(binDir, "order.log")
	bdPath := filepath.Join(binDir, "bd")
	gtPath := filepath.Join(binDir, "ms")

	bdScript := `#!/bin/sh
ORDER="` + orderPath + `"
if [ "$1" = "--allow-stale" ]; then
  shift
fi
case "$1" in
  version)
    exit 0
    ;;
  show)
    printf '%s\n' '[{"id":"hq-cv-export-fail","description":"Owner: overseer/","created_at":"2026-05-25T02:00:00Z"}]'
    exit 0
    ;;
  sql)
    printf '%s\n' '[]'
    exit 0
    ;;
  update)
    echo update >> "$ORDER"
    exit 0
    ;;
  export)
    echo export:"$@" >> "$ORDER"
    exit 1
    ;;
esac
exit 1
`
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	gtScript := `#!/bin/sh
if [ "$1" = "mail" ] && [ "$2" = "send" ]; then
  echo mail >> "` + orderPath + `"
fi
exit 0
`
	if err := os.WriteFile(gtPath, []byte(gtScript), 0755); err != nil {
		t.Fatalf("write ms stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	notifyMinecartCompletion(townRoot, "hq-cv-export-fail", "Export Failure")

	data, err := os.ReadFile(orderPath)
	if err != nil {
		t.Fatalf("read order log: %v", err)
	}
	got := strings.TrimSpace(string(data))
	want := strings.Join([]string{
		"mail",
		"update",
		"export:export -o " + filepath.Join(townRoot, ".beads", "issues.jsonl"),
	}, "\n")
	if got != want {
		t.Fatalf("operation order mismatch:\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestCloseMinecartIfComplete_CloseExportFailureRequiresDurableRetryBeforeNotification(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	binDir := t.TempDir()
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	exportFailedPath := filepath.Join(binDir, "export.failed")
	orderPath := filepath.Join(binDir, "order.log")
	bdPath := filepath.Join(binDir, "bd")
	gtPath := filepath.Join(binDir, "ms")

	bdScript := `#!/bin/sh
EXPORT_FAILED="` + exportFailedPath + `"
ORDER="` + orderPath + `"
if [ "$1" = "--allow-stale" ]; then
  shift
fi
case "$1" in
  version)
    exit 0
    ;;
  close)
    echo close >> "$ORDER"
    exit 0
    ;;
  export)
    echo export:"$@" >> "$ORDER"
    if [ ! -f "$EXPORT_FAILED" ]; then
      touch "$EXPORT_FAILED"
      exit 1
    fi
    exit 0
    ;;
  show)
    printf '%s\n' '[{"id":"hq-cv-close-export-fail","description":"Owner: overseer/","created_at":"2026-05-25T02:00:00Z"}]'
    exit 0
    ;;
  update)
    echo update >> "$ORDER"
    exit 0
    ;;
  sql)
    printf '%s\n' '[]'
    exit 0
    ;;
esac
exit 1
`
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	gtScript := `#!/bin/sh
if [ "$1" = "mail" ] && [ "$2" = "send" ]; then
  echo mail >> "` + orderPath + `"
fi
exit 0
`
	if err := os.WriteFile(gtPath, []byte(gtScript), 0755); err != nil {
		t.Fatalf("write ms stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	closed, err := closeMinecartIfComplete(townRoot, "hq-cv-close-export-fail", "Close Export Failure", []trackedIssueInfo{
		{ID: "ms-done", Status: "closed"},
	}, false)
	if err == nil {
		t.Fatal("closeMinecartIfComplete returned nil error after close JSONL export failure")
	}
	if closed {
		t.Fatal("closeMinecartIfComplete returned closed=true after close JSONL export failure")
	}
	if err := persistAndNotifyMinecartCompletion(townRoot, "hq-cv-close-export-fail", "Close Export Failure"); err != nil {
		t.Fatalf("persistAndNotifyMinecartCompletion returned error: %v", err)
	}

	data, err := os.ReadFile(orderPath)
	if err != nil {
		t.Fatalf("read order log: %v", err)
	}
	got := strings.TrimSpace(string(data))
	want := strings.Join([]string{
		"close",
		"export:export -o " + filepath.Join(townRoot, ".beads", "issues.jsonl"),
		"export:export -o " + filepath.Join(townRoot, ".beads", "issues.jsonl"),
		"mail",
		"update",
		"export:export -o " + filepath.Join(townRoot, ".beads", "issues.jsonl"),
	}, "\n")
	if got != want {
		t.Fatalf("operation order mismatch:\n got:\n%s\nwant:\n%s", got, want)
	}
}
