package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// RED-076: a signed series file whose series_id is not its params' id is
// refused without writing its text to the terminal raw.
func TestSeriesIDTextIsQuoted(t *testing.T) {
	b, err := json.Marshal(map[string]any{
		"series_id":      "\x1b[2J\x1b[31mFAKE\x07",
		"params":         testutil.ExampleParams(),
		"issuer_witness": "",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = readSeriesInfo(b)
	if err == nil || !strings.Contains(err.Error(), "is not the id of its params") {
		t.Fatalf("got %v", err)
	}
	for _, c := range err.Error() {
		if c < 0x20 || c == 0x7f {
			t.Fatalf("control byte %#x in %q", c, err.Error())
		}
	}
}

// RED-074: a release file with a key poon does not write is refused; the
// keys poon series release adds for people are read.
func TestReleaseFileKeys(t *testing.T) {
	base := `{"series":"` + strings.Repeat("00", 32) + `","part":2,"previous":"` + strings.Repeat("00", 32) + `","witness":"00"`
	var r series.Release
	if err := json.Unmarshal([]byte(base+`,"extra":1}`), &r); !errors.Is(err, series.ErrUnknownKey) {
		t.Fatalf("unknown key: %v", err)
	}
	if err := json.Unmarshal([]byte(base+`,"part_id":"x","first_serial":1,"last_serial":2,"first_pool":0,"last_pool":0,"mints":4,"units":4}`), &r); errors.Is(err, series.ErrUnknownKey) {
		t.Fatalf("poon's own keys: %v", err)
	}
}
