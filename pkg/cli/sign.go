package cli

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/fsutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wallet"
)

// poo sign (D193): adds a key's signatures to a payment waiting for them
// (the file poo wallet pay -out writes), with no node, no wallet and no
// node policy, so it can run on a machine that is never online. The key
// comes from a file, never the command line, and is either a poo seed (as
// poo keygen makes) or, with -raw, the private key in its algorithm's
// usual form as another tool keeps it.

// SignReport is what poo sign prints.
type SignReport struct {
	// Series is the payment's series, the same in every leg (poo sign
	// refuses a file where they differ), and every amount is in its
	// units; Hash is the hash function each tid is worked out with, and
	// Variable says the series is variable supply (RED-104, RED-105).
	Series   hash.Hash `json:"series"`
	Hash     string    `json:"hash"`
	Variable bool      `json:"variable,omitempty"`
	// Key is the signing key's public key and Address its 1-of-1 address
	// under the payment's hash.
	Alg     string `json:"alg"`
	Key     string `json:"public_key"`
	Address string `json:"address"`
	// Legs says what each transaction pays: read them before signing.
	Legs []SignLeg `json:"legs"`
	// Added counts the signatures this key added; Missing, per leg and
	// input, those still needed.
	Added   int     `json:"added"`
	Missing [][]int `json:"missing"`
	// Shown is set by -show: nothing was signed.
	Shown bool   `json:"shown,omitempty"`
	Wrote string `json:"wrote,omitempty"`
}

// SignLeg is one transaction of the payment.
type SignLeg struct {
	Serial uint64    `json:"serial"`
	TID    hash.Hash `json:"tid"`
	Inputs int       `json:"inputs"`
	// Spends lists each input the leg spends as tid:index, to compare
	// with what the watch-only wallet says it spends (RED-104).
	Spends []string `json:"spends,omitempty"`
	// Claim marks a leg that claims a whole mint, which spends no input
	// (poo wallet claim -to-policy, GREEN-060).
	Claim   bool        `json:"claim,omitempty"`
	Outputs []tx.Output `json:"outputs"`
}

func signCmd(args []string, out io.Writer) error {
	fs := newFlagSet("sign", out)
	keyFile := fs.String("key-file", "", "file holding the key in hex, - to type it hidden (needed)")
	algFlag := fs.String("alg", "ed25519", "the key's algorithm, by name or number (poo crypto)")
	raw := fs.Bool("raw", false, "the key file holds the private key in its algorithm's usual form, as other tools keep it, not a poo seed")
	hashName := fs.String("hash", "", "the series' hash, when the file does not name it")
	outFile := fs.String("out", "", "write the signed payment here (default: back into FILE)")
	show := fs.Bool("show", false, "only show what the payment pays and what this key would sign; sign nothing")
	pos, err := parseMixed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: poo sign FILE|- -key-file KEYFILE [-alg NAME] [-raw] [-hash NAME] [-out FILE] [-show]")
	}
	file := pos[0]
	if *keyFile == "" {
		return errors.New("poo sign needs -key-file (see poo keygen); a key is never taken on the command line")
	}
	if file == "-" && *keyFile == "-" {
		return errors.New("the payment and the key cannot both come from standard input")
	}
	if file == "-" && *outFile == "" && !*show {
		return errors.New("a payment read from standard input needs -out")
	}
	alg, err := parseAlg(*algFlag)
	if err != nil {
		return fmt.Errorf("-alg: %w", err)
	}
	var b []byte
	if file == "-" {
		b, err = io.ReadAll(io.LimitReader(inputOf(out), 64<<20))
	} else {
		b, err = os.ReadFile(file)
	}
	if err != nil {
		return err
	}
	var u wallet.Unsigned
	if err := json.Unmarshal(b, &u); err != nil {
		return fmt.Errorf("not a payment waiting for signatures (poo wallet pay -out): %w", err)
	}
	if len(u.Legs) == 0 {
		return errors.New("the payment has no transactions")
	}
	hn := u.Hash
	if *hashName != "" {
		hn = *hashName
	}
	if hn == "" {
		return errors.New("the payment does not name its series' hash; give -hash (poo series info names it)")
	}
	hid, err := parseHash(hn)
	if err != nil {
		return err
	}
	h, _ := hash.Lookup(hid)

	sf := &seedFlags{file: keyFile, flag: new(string), what: "key"}
	keyHex, err := sf.read(out, false)
	if err != nil {
		return err
	}
	key, err := hex.DecodeString(strings.TrimSpace(keyHex))
	if err != nil || len(key) == 0 {
		return errors.New("the key file must hold the key in hex")
	}
	var signer policy.Signer
	if *raw {
		signer, err = policy.NewRawSigner(alg, key)
	} else if len(key) != policy.SeedSize {
		err = fmt.Errorf("a poo seed is %d hex digits; for a key made by another tool, add -raw", 2*policy.SeedSize)
	} else {
		signer, err = policy.NewSigner(alg, key)
	}
	clear(key)
	if err != nil {
		return err
	}

	rep := SignReport{Series: u.Series, Hash: hid.Name(), Variable: u.Variable, Alg: algName(alg), Key: hex.EncodeToString(signer.Public()), Address: policy.Address(h, policy.SingleKey(alg, signer.Public())).String(), Shown: *show}
	for i, l := range u.Legs {
		// Every leg must be of the payment's series and mint (RED-104).
		t, err := u.DecodeLeg(i)
		if err != nil {
			return err
		}
		leg := SignLeg{Serial: l.Serial, TID: t.TID(h), Inputs: len(t.Inputs), Claim: t.IsClaim(), Outputs: t.Outputs}
		for _, in := range t.Inputs {
			leg.Spends = append(leg.Spends, fmt.Sprintf("%s:%d", in.TID, in.Index))
		}
		rep.Legs = append(rep.Legs, leg)
	}
	if !*show {
		if rep.Added, err = wallet.SignWith(&u, h, alg, signer); err != nil {
			return err
		}
	}
	rep.Missing = u.Missing()
	if !*show {
		if rep.Added == 0 {
			return fmt.Errorf("this key (%s %s) is in none of the payment's inputs, or has signed them all already: check -alg and -raw", rep.Alg, rep.Key)
		}
		dst := *outFile
		if dst == "" {
			dst = file
		}
		nb, err := json.MarshalIndent(&u, "", "  ")
		if err != nil {
			return err
		}
		if err := fsutil.WriteFile(dst, append(nb, '\n'), 0o600); err != nil {
			return err
		}
		rep.Wrote = dst
	}
	return writeJSON(out, rep)
}
