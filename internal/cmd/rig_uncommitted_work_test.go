package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/steveyegge/excavation/internal/git"
	"github.com/steveyegge/excavation/internal/miner"
	"github.com/steveyegge/excavation/internal/rig"
)

func stubUncommittedWorkCheckDeps(
	t *testing.T,
	listFn func(*rig.Rig) ([]*miner.Miner, error),
	checkFn func(string) (*git.UncommittedWorkStatus, error),
	isTTYFn func() bool,
	promptFn func(string) bool,
) {
	t.Helper()

	oldList := listMinersForWorkCheck
	oldCheck := checkMinerWorkStatus
	oldIsTTY := isStdinTerminal
	oldPrompt := promptYesNoUnsafeProceed

	listMinersForWorkCheck = listFn
	checkMinerWorkStatus = checkFn
	isStdinTerminal = isTTYFn
	promptYesNoUnsafeProceed = promptFn

	t.Cleanup(func() {
		listMinersForWorkCheck = oldList
		checkMinerWorkStatus = oldCheck
		isStdinTerminal = oldIsTTY
		promptYesNoUnsafeProceed = oldPrompt
	})
}

func testRig() *rig.Rig {
	return &rig.Rig{
		Name: "testrig",
		Path: "/tmp/testrig",
	}
}

func TestCheckUncommittedWork_ListErrorBlocksWithoutForce(t *testing.T) {
	stubUncommittedWorkCheckDeps(
		t,
		func(*rig.Rig) ([]*miner.Miner, error) {
			return nil, errors.New("list failed")
		},
		func(string) (*git.UncommittedWorkStatus, error) {
			t.Fatalf("check should not be called when list fails")
			return nil, nil
		},
		func() bool { return false },
		func(string) bool {
			t.Fatalf("prompt should not be called without --force")
			return false
		},
	)

	var proceed bool
	output := captureStdout(t, func() {
		proceed = checkUncommittedWork(testRig(), "testrig", "stop", false)
	})

	if proceed {
		t.Fatal("expected proceed=false when miner listing fails without --force")
	}
	if !strings.Contains(output, "Could not check miners for uncommitted work") {
		t.Fatalf("expected list-error warning, got: %q", output)
	}
	if !strings.Contains(output, "--force") || !strings.Contains(output, "--nuclear") {
		t.Fatalf("expected override hint, got: %q", output)
	}
}

func TestCheckUncommittedWork_ListErrorForceTTYPrompts(t *testing.T) {
	stubUncommittedWorkCheckDeps(
		t,
		func(*rig.Rig) ([]*miner.Miner, error) {
			return nil, errors.New("list failed")
		},
		func(string) (*git.UncommittedWorkStatus, error) {
			t.Fatalf("check should not be called when list fails")
			return nil, nil
		},
		func() bool { return true },
		func(question string) bool {
			if question != "Proceed anyway?" {
				t.Fatalf("unexpected prompt question: %q", question)
			}
			return true
		},
	)

	proceed := checkUncommittedWork(testRig(), "testrig", "shutdown", true)
	if !proceed {
		t.Fatal("expected proceed=true after force+TTY confirmation")
	}
}

func TestCheckUncommittedWork_MinerStatusErrorBlocks(t *testing.T) {
	stubUncommittedWorkCheckDeps(
		t,
		func(*rig.Rig) ([]*miner.Miner, error) {
			return []*miner.Miner{
				{Name: "alpha", ClonePath: "/tmp/alpha"},
			}, nil
		},
		func(string) (*git.UncommittedWorkStatus, error) {
			return nil, errors.New("git status failed")
		},
		func() bool { return false },
		func(string) bool {
			t.Fatalf("prompt should not be called without --force")
			return false
		},
	)

	var proceed bool
	output := captureStdout(t, func() {
		proceed = checkUncommittedWork(testRig(), "testrig", "restart", false)
	})

	if proceed {
		t.Fatal("expected proceed=false when miner status check fails")
	}
	if !strings.Contains(output, "Could not verify uncommitted work for") {
		t.Fatalf("expected status-check error warning, got: %q", output)
	}
	if !strings.Contains(output, "alpha") {
		t.Fatalf("expected miner name in warning, got: %q", output)
	}
}

func TestCheckUncommittedWork_DirtyForceNonTTYBlocks(t *testing.T) {
	stubUncommittedWorkCheckDeps(
		t,
		func(*rig.Rig) ([]*miner.Miner, error) {
			return []*miner.Miner{
				{Name: "alpha", ClonePath: "/tmp/alpha"},
			}, nil
		},
		func(string) (*git.UncommittedWorkStatus, error) {
			return &git.UncommittedWorkStatus{
				HasUncommittedChanges: true,
				ModifiedFiles:         []string{"README.md"},
			}, nil
		},
		func() bool { return false },
		func(string) bool {
			t.Fatalf("prompt should not be called in non-TTY mode")
			return false
		},
	)

	var proceed bool
	output := captureStdout(t, func() {
		proceed = checkUncommittedWork(testRig(), "testrig", "stop", true)
	})

	if proceed {
		t.Fatal("expected proceed=false for force in non-TTY mode")
	}
	if !strings.Contains(output, "--force") || !strings.Contains(output, "interactive terminal") {
		t.Fatalf("expected non-TTY force hint, got: %q", output)
	}
}

func TestCheckUncommittedWork_DirtyForceTTYPrompts(t *testing.T) {
	stubUncommittedWorkCheckDeps(
		t,
		func(*rig.Rig) ([]*miner.Miner, error) {
			return []*miner.Miner{
				{Name: "alpha", ClonePath: "/tmp/alpha"},
			}, nil
		},
		func(string) (*git.UncommittedWorkStatus, error) {
			return &git.UncommittedWorkStatus{
				HasUncommittedChanges: true,
				ModifiedFiles:         []string{"README.md"},
			}, nil
		},
		func() bool { return true },
		func(question string) bool {
			if question != "Proceed anyway?" {
				t.Fatalf("unexpected prompt question: %q", question)
			}
			return true
		},
	)

	proceed := checkUncommittedWork(testRig(), "testrig", "stop", true)
	if !proceed {
		t.Fatal("expected proceed=true after force+TTY confirmation")
	}
}
