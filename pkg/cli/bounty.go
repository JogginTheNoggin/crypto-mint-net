package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/bounty"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wallet"
)

// poo bounty: the bounty market (D196). A node posts offers to pay for
// layers on its entries; a peer leases one, grinds the layer and claims
// it; the payer pays with an ordinary wallet payment. The market is off
// until node.json's bounty section turns it on.

const bountyUsage = `usage:
  poo bounty post -entry HASH [-pool K | -serial N] -strength S -amount A [-count N] [-open SECONDS]
  poo bounty list [-ask] [-done N]
  poo bounty work ID -to ADDR [-max-seconds N]
  poo bounty pay ID [-claim NONCE] [-wallet FILE]
  poo bounty cancel ID`

// BountyAskWait is how long poo bounty list -ask waits for peers'
// answers.
var BountyAskWait = 2 * time.Second

func bountyCmd(args []string, out io.Writer) error {
	if len(args) == 0 || isHelpWord(args[0]) {
		fmt.Fprintln(out, bountyUsage)
		return nil
	}
	switch args[0] {
	case "post":
		return bountyPost(args[1:], out)
	case "list":
		return bountyList(args[1:], out)
	case "work":
		return bountyWork(args[1:], out)
	case "pay":
		return bountyPay(args[1:], out)
	case "cancel":
		return bountyCancel(args[1:], out)
	}
	return fmt.Errorf("unknown bounty subcommand %q\n%s", args[0], bountyUsage)
}

func bountyPost(args []string, out io.Writer) error {
	fs := newFlagSet("bounty post", out)
	c := addClient(fs)
	pf := addPool(fs)
	entry := fs.String("entry", "", "the entry to add a layer to (needed)")
	strength := fs.Uint("strength", 0, "the least strength in bits a layer must have (needed; at least the series' layer floor)")
	amount := fs.Uint64("amount", 0, "units of the series paid for each layer (needed)")
	count := fs.Uint("count", 1, "how many layers to pay for")
	open := fs.Uint("open", 3600, "seconds the offer stays open")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("poo bounty post takes flags only")
	}
	e, err := hash.Parse(*entry)
	if err != nil {
		return fmt.Errorf("-entry: %w", err)
	}
	if *strength == 0 || *strength > 255 || *amount == 0 || *count == 0 || *count > 65535 || *open > 1<<31 {
		return errors.New("poo bounty post needs -strength 1 to 255, -amount above 0 and -count 1 to 65535")
	}
	cl, err := c.dial(out)
	if err != nil {
		return err
	}
	s, err := c.seriesOf(cl)
	if err != nil {
		return err
	}
	pool, err := pf.resolve(fs, s)
	if err != nil {
		return err
	}
	// With neither -pool nor -serial the node finds the pool that holds
	// the entry, rather than taking pool 0 (GREEN-061).
	named := false
	fs.Visit(func(f *flag.Flag) { named = named || f.Name == "pool" || f.Name == "serial" })
	rep, err := cl.BountyPost(rpc.BountyPostRequest{PoolRef: rpc.PoolRef{Series: s.ID(), Pool: pool}, Entry: e,
		Strength: uint8(*strength), Amount: *amount, Count: uint16(*count), OpenSeconds: uint32(*open), FindPool: !named})
	if err != nil {
		return err
	}
	return writeJSON(out, rep)
}

func bountyList(args []string, out io.Writer) error {
	fs := newFlagSet("bounty list", out)
	c := addClient(fs)
	ask := fs.Bool("ask", false, "first ask connected peers for their open offers, and wait a moment for answers")
	done := fs.Int("done", 0, "also list this node's last N finished offers (filled or closed, every layer paid; at most 256), which leave the list")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("poo bounty list takes flags only")
	}
	cl, err := c.dial(out)
	if err != nil {
		return err
	}
	if *ask {
		if viewOnlyOf(out) {
			return errors.New("-ask sends a request to peers, which view-only mode does not do")
		}
		if _, err := cl.BountyAsk(hash.Hash{}); err != nil {
			return err
		}
		time.Sleep(BountyAskWait)
	}
	var rep rpc.BountyReply
	if *done > 0 {
		rep, err = cl.BountyDone(*done)
	} else {
		rep, err = cl.Bounty()
	}
	if err != nil {
		return err
	}
	return writeJSON(out, rep)
}

// bountyID parses args, flags before or after the offer id, and returns
// the id.
func bountyID(fs *flag.FlagSet, args []string, what string) (hash.Hash, error) {
	pos, err := parseMixed(fs, args)
	if err != nil {
		return hash.Hash{}, err
	}
	if len(pos) != 1 {
		return hash.Hash{}, fmt.Errorf("usage: poo bounty %s ID", what)
	}
	id, err := hash.Parse(pos[0])
	if err != nil {
		return hash.Hash{}, fmt.Errorf("the offer id: %w", err)
	}
	return id, nil
}

func bountyWork(args []string, out io.Writer) error {
	fs := newFlagSet("bounty work", out)
	c := addClient(fs)
	to := fs.String("to", "", "the address to be paid (needed): a fresh one from poo wallet address")
	maxSec := fs.Int("max-seconds", 0, "stop grinding after this long (default and most: 240; the lease may end it sooner)")
	id, err := bountyID(fs, args, "work")
	if err != nil {
		return err
	}
	payTo, err := hash.Parse(*to)
	if err != nil {
		return fmt.Errorf("-to: %w", err)
	}
	cl, err := c.dial(out)
	if err != nil {
		return err
	}
	rep, err := cl.BountyWork(rpc.BountyWorkRequest{ID: id, PayTo: payTo, MaxSeconds: *maxSec})
	if err != nil {
		return err
	}
	return writeJSON(out, rep)
}

func bountyCancel(args []string, out io.Writer) error {
	fs := newFlagSet("bounty cancel", out)
	c := addClient(fs)
	id, err := bountyID(fs, args, "cancel")
	if err != nil {
		return err
	}
	cl, err := c.dial(out)
	if err != nil {
		return err
	}
	m, err := cl.BountyCancel(id)
	if err != nil {
		return err
	}
	return writeJSON(out, m)
}

// BountyPayReply is what poo bounty pay did.
type BountyPayReply struct {
	Offer  hash.Hash    `json:"offer"`
	Nonce  uint64       `json:"nonce"`
	PayTo  hash.Hash    `json:"pay_to"`
	Amount uint64       `json:"amount"`
	Sent   *wallet.Sent `json:"sent"`
	Told   bool         `json:"told"`
	// Recorded says the wallet had sent this payment already, and the
	// node had not recorded it: it was recorded now, and nothing was sent
	// again (RED-119).
	Recorded bool `json:"recorded,omitempty"`
}

// sentFor finds the wallet's payment of claim c of offer m: its memo,
// address and amount, every leg sent, and no leg already recorded on
// another claim. An earlier payment sent only in part is an error, as
// paying again would pay more than once (RED-119).
func sentFor(w *wallet.Wallet, m *bounty.Mine, c *bounty.ClaimRecord) (*wallet.Sent, error) {
	used := map[hash.Hash]bool{}
	for _, r := range m.Claims {
		for _, t := range r.Paid {
			used[t] = true
		}
	}
	memo := "bounty " + m.ID.Short()
	for _, p := range w.Sent() {
		if p.Claim || p.Memo != memo || p.Series != m.Offer.Series || p.To != c.PayTo || p.Amount != m.Offer.Amount || len(p.Legs) == 0 {
			continue
		}
		sent, mine := 0, false
		for _, l := range p.Legs {
			if l.State == wallet.StatePending || l.State == wallet.StateSelected {
				sent++
			}
			mine = mine || used[l.TID]
		}
		switch {
		case mine || sent == 0:
		case sent < len(p.Legs):
			return nil, fmt.Errorf("an earlier payment of this claim (tid %s) was sent only in part: settle it by hand before paying again", p.Legs[0].TID.Short())
		default:
			return p, nil
		}
	}
	return nil, nil
}

func bountyPay(args []string, out io.Writer) error {
	fs := newFlagSet("bounty pay", out)
	f := addWallet(fs)
	claim := fs.Uint64("claim", 0, "the nonce of the accepted claim to pay (default: the offer's only unpaid one)")
	id, err := bountyID(fs, args, "pay")
	if err != nil {
		return err
	}
	gotClaim := false
	fs.Visit(func(fl *flag.Flag) { gotClaim = gotClaim || fl.Name == "claim" })
	cl, err := f.c.dial(out)
	if err != nil {
		return err
	}
	rep, err := cl.Bounty()
	if err != nil {
		return err
	}
	var mine *bounty.Mine
	for i := range rep.Mine {
		if rep.Mine[i].ID == id {
			mine = &rep.Mine[i]
		}
	}
	if mine == nil {
		return errors.New("no offer of this node has that id")
	}
	var pick *bounty.ClaimRecord
	unpaid := 0
	for i := range mine.Claims {
		c := &mine.Claims[i]
		if len(c.Paid) > 0 {
			continue
		}
		unpaid++
		if !gotClaim || c.Nonce == *claim {
			pick = c
		}
	}
	switch {
	case pick == nil:
		return errors.New("no unpaid accepted claim to pay on that offer")
	case !gotClaim && unpaid > 1:
		return fmt.Errorf("%d claims are unpaid; name one with -claim NONCE (poo bounty list shows them)", unpaid)
	}
	s, err := f.series()
	if err != nil {
		return err
	}
	if s.ID() != mine.Offer.Series {
		return errors.New("the wallet's series is not the offer's; give -series")
	}
	w, err := f.unlocked(out)
	if err != nil {
		return err
	}
	if w.WatchOnly() {
		return errors.New("poo bounty pay needs a wallet that signs: a watch-only wallet cannot pay a bounty yet")
	}
	n, err := f.node(out, s)
	if err != nil {
		return err
	}
	release, err := holdWallet(w, out)
	if err != nil {
		return err
	}
	defer release()
	// The wallet's own record comes first: a payment it sent that the
	// node did not record (a failed save, a stop in between, a restart)
	// is recorded now, never paid twice (RED-119).
	if prev, err := sentFor(w, mine, pick); err != nil {
		return err
	} else if prev != nil {
		var tids []hash.Hash
		for _, l := range prev.Legs {
			tids = append(tids, l.TID)
		}
		paid, err := cl.BountyPaid(rpc.BountyPaidRequest{ID: id, Nonce: pick.Nonce, TIDs: tids})
		if err != nil {
			return fmt.Errorf("the wallet paid this claim already (tid %s), and the node did not record it: %w", tids[0].Short(), err)
		}
		return writeJSON(out, BountyPayReply{Offer: id, Nonce: pick.Nonce, PayTo: pick.PayTo, Amount: mine.Offer.Amount, Sent: prev, Told: paid.Told, Recorded: true})
	}
	sent, u, err := w.Pay(s, n, pick.PayTo, mine.Offer.Amount, wallet.PayOptions{Memo: "bounty " + id.Short()})
	if err == nil && u != nil {
		return errPayNeedsOut(u.Missing())
	}
	if sent != nil {
		if serr := saveSent(w, out, "payment", sent, err); serr != nil {
			return serr
		}
	}
	if errors.Is(err, wallet.ErrNoFunds) && len(w.Sent()) > 0 {
		// A payer with several claims meets this at once: the change of
		// the last payment is spendable only once an entry holds it
		// (GUI-GREEN-062).
		return fmt.Errorf("%w; the change of an earlier payment is spendable once an entry holds that payment: wait for one (or build one with poo build), then pay again", err)
	}
	if err != nil {
		return err
	}
	var tids []hash.Hash
	for _, l := range sent.Legs {
		tids = append(tids, l.TID)
	}
	paid, err := cl.BountyPaid(rpc.BountyPaidRequest{ID: id, Nonce: pick.Nonce, TIDs: tids})
	if err != nil {
		return fmt.Errorf("paid, but the node did not record it: %w", err)
	}
	return writeJSON(out, BountyPayReply{Offer: id, Nonce: pick.Nonce, PayTo: pick.PayTo, Amount: mine.Offer.Amount, Sent: sent, Told: paid.Told})
}
