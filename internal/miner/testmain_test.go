package miner

import (
	"os"
	"testing"

	"github.com/steveyegge/excavation/internal/testutil"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testutil.TerminateDoltContainer()
	os.Exit(code)
}
