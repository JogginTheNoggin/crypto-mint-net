package cli

import (
	"os"
	"testing"
)

// TestMain points the home folder and POON_HOME at an empty folder, so no
// test reads the machine's own node policies (found on jerry's machine,
// D208).
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "poon-cli-test-")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	os.Setenv("USERPROFILE", dir)
	os.Setenv("POON_HOME", dir+"/.poon")
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
