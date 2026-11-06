package cli

import "testing"

// BLUE-041 (RED-122): $POON_VIEW_ONLY fails closed. Only empty, 0,
// false, no and off leave view-only mode off; any other value turns it
// on.
func TestBlue041ViewOnlyFailsClosed(t *testing.T) {
	for v, want := range map[string]bool{
		"": false, "0": false, "false": false, "No": false, " off ": false,
		"1": true, "true": true, "yes": true, "on": true,
		"2": true, "y": true, "enabled": true, "ture": true, "-1": true,
	} {
		t.Setenv(ViewOnlyEnv, v)
		if got := viewOnlyEnv(); got != want {
			t.Errorf("%s=%q: view-only %v, want %v", ViewOnlyEnv, v, got, want)
		}
	}
}
