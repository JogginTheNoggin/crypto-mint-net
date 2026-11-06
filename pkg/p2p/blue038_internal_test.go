package p2p

import "testing"

// RED-116: the market limits an address by its IPv4 address or IPv6 /64.
func TestBlue038AddressKey(t *testing.T) {
	for in, want := range map[string]string{
		"192.0.2.7:7733":              "192.0.2.7",
		"[2001:db8:1:2:aaaa::1]:7733": "2001:db8:1:2::/64",
		"[2001:db8:1:2:bbbb::9]:7733": "2001:db8:1:2::/64",
		"[::ffff:192.0.2.7]:7733":     "192.0.2.7",
		"seed.example:7733":           "seed.example",
	} {
		if got := hostOf(in); got != want {
			t.Fatalf("%s: %s, want %s", in, got, want)
		}
	}
}
