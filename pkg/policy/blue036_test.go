package policy_test

import (
	"crypto/rand"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/cloudflare/circl/sign/slhdsa"
)

// RED-106: a raw SLH-DSA key whose PK.root does not match its seeds is
// refused, rather than giving an address no signature can spend.
func TestBlue036RawKeySelfTested(t *testing.T) {
	_, slh, _ := slhdsa.GenerateKey(rand.Reader, slhdsa.SHA2_128s)
	raw, _ := slh.MarshalBinary()
	if _, err := policy.NewRawSigner(policy.AlgSLHDSASHA2128s, raw); err != nil {
		t.Fatalf("a good key: %v", err)
	}
	raw[len(raw)-1] ^= 1 // the last byte of PK.root
	if _, err := policy.NewRawSigner(policy.AlgSLHDSASHA2128s, raw); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("a key with a wrong root: %v", err)
	}
}
