# RED-009: Pre-lock signature verification runs on items the cheap checks would reject, does not stop at the first bad signature, and never remembers a failure

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | open |
| Found on | `claude/m10-watch` @ 0ac8e4e (Source Round 4 Part A) |
| Component | pkg/engine/engine.go (PreverifyTx, PreverifyEntry), pkg/tx/tx.go (Preverify), pkg/policy/verified.go (Preverify, Verified), pkg/node/node.go (HandleTxFrom, HandleEntryFrom) |
| Surface | peer |
| Fixed by | — |

## Summary

ORANGE-004 (D105) verifies an item's witnesses before taking the node's
lock, so that the checks under the lock find them remembered. D105 says it
does this "only for what `Handle*` would verify too". It does not. Before
the change, a transaction or entry was rejected by cheap checks (mint not
claimed, input missing, parent not held, conservation) **before any
signature was looked at**, and the first bad signature ended the check.
`PreverifyTx` and `PreverifyEntry` run none of those checks, and
`tx.Preverify` verifies **every** witness of the item whether or not an
earlier one failed:

```go
// pkg/tx/tx.go
func Preverify(s *series.Series, t *Tx, tid hash.Hash) {
    for _, w := range t.Witnesses {          // no stop at the first failure
        s.Preverify(w, tid)
    }
}

// pkg/policy/verified.go
func (r *Registry) Preverify(rules Rules, w Witness, tid hash.Hash, v *Verified) {
    ...                                      // no address check, no state
    if t.Satisfy(r, w.Policy.Params, w.Data, Message(tid)) == nil {
        v.add(k)                             // only a pass is remembered
    }
}
```

So a peer can make the node run the full satisfaction function on every
witness of a frame it fully controls, at a cost to the peer of a few
hundred bytes of bandwidth per signature, and the node never learns that
the item was bad until later, if at all:

- **Transactions.** `PreverifyTx` gates only on "pool followed, tid not
  seen". `seen` is added to only after a transaction is *accepted*
  (`HandleTx`: `e.seen.Add(tid)` follows `pending.Add` succeeding), so a
  rejected transaction is never "seen" and can be sent again and again.
  Nothing requires the inputs to exist, the mint to be claimed, or a
  witness's policy to match an input's address (the check
  `Address(h, w.Policy) == addr` in `CheckCached` is not in `Preverify`).
  A transaction of up to `tx.MaxInputs` (65,536) inputs carries one witness
  per input; the attacker can repeat one witness that is valid for another
  tid (same bytes, so nothing to compute) and the node verifies and fails it
  65,536 times. The old path rejected the same transaction at
  `ErrNotClaimed` / `ErrMissingInput` before the first signature.
- **Entries.** `PreverifyEntry` gates on `entry.CheckShape`, which the
  proposal calls "the entry's work". CheckShape's work gate is
  `Strength() >= MinEntryStrength`, and `MinEntryStrength` is **0 by
  default** (`poo series new` defaults `-min-entry-strength 0`, "0: none";
  `EntriesHaveNonce()` is `MinEntryStrength > 0`), so for such a series the
  gate is empty: any entry of the right series and pool with its
  transactions in order passes it. An entry whose parent the node does not
  hold is an ordinary honest case that `handlePushed` deliberately answers
  with `return nil` (no offence), and the old path (`AddEntry`) returned
  `ErrUnknownParent` before verifying any non-claim signature. Now every
  signature in it is verified first.
- **No penalty.** `handlePushed` says a transaction "can be refused for
  many honest reasons ... none of them is the peer's fault", and only
  `credit`s a success or a repeat: a refused transaction is never an
  offence, so nothing in the tracker charges the sender. The per-address
  budget (1,200 transactions a minute) is far above what bandwidth allows
  for large frames, so bandwidth is the real limit.

ORANGE-004 notes that a bad-signature transaction "is now verified twice"
and that this "doubles the CPU a peer can make the node spend ... still
bounded by peer scoring". Both statements understate this: the factor is
not 2 but up to the number of witnesses in the item (65,536 for a
transaction), because the old path stopped at the first failure or before
any signature, and there is no peer scoring on this path.

A second, smaller effect of the same missing gate: `Preverify` remembers
every *pass*, and the list is a 16,384-entry FIFO ring (`DefaultVerifiedSize`).
A peer can send transactions with valid signatures under policies of its
own choosing, for inputs that do not exist, and fill the ring with its own
keys, evicting the witnesses of real pending transactions so that the
verification ORANGE-002/004 moved out of the lock happens under it again.

## Reproduction

Measured on `claude/m10-watch` @ deecc80 (the code of 0ac8e4e), 4 vCPU,
Go 1.27.3. A temporary test in `pkg/engine` (not committed), using the
example series (`MinEntryStrength` 0):

```go
s := testutil.ExampleSeries()
k := testutil.NewKeys(s)
e := engine.New(engine.DefaultConfig())
e.AddSeries(s); e.FollowSeries(s.ID())
first, _ := s.PoolRange(1)
ins := make([]testutil.In, 5000)
for i := range ins {
    ins[i] = testutil.In{Input: tx.Input{TID: hash.Hash{9}, Index: uint32(i)}, Owner: "alice"}
}
bad := k.Spend(first, ins, testutil.P("bob", 1))
bad.Outputs[0].Amount = 2   // the tid changes: every witness now signs another tid
e.PreverifyTx(bad)          // what HandleTxFrom runs before it takes the lock
e.HandleTx(bad, engine.Pushed)
```

Result:

```
witnesses=5000 bytes=860092
preverify=261.8ms (52.4us/witness)   handleTx=0.67ms  err=mempool: invalid ... mint not claimed
entry on an unknown parent: preverify=246.6ms   handleEntry=0.77ms  err=ledger: parent not held
resend of the same failing tx: 334us (first), 52us, 48us  (per witness 1; failures not remembered)
20000 valid txs on nonexistent inputs: remembered=16384 (the ring cap)
```

So for an 860 KB frame the node spends about 262 ms of CPU where it used to
spend 0.7 ms (about 390 times as much), and the cost repeats on every resend.
Scaled to the 65,536-input maximum (about 11 MB, under `wire.MaxFrame`) one
frame costs about 3.4 CPU-seconds. At roughly 0.3 CPU-seconds per megabyte,
one peer sending at 100 Mbit/s (12.5 MB/s) keeps about 3.8 cores busy; a
handful of inbound peers (each address has its own budget) saturates a node.
The work runs on the message-handling goroutines, outside the engine lock,
so the node does not wedge as in RED-006/008, but it is starved of CPU, and
every other peer's items, the control API and the chase machinery slow with
it.

## Impact

Medium. A remote, unauthenticated peer (any inbound connection) can burn a
node's CPU at a ratio of several hundred to one against the bytes it sends,
with no offence ever recorded against it, on a node whose series has the
default `MinEntryStrength` of 0 (for transactions the series settings do not
matter at all). Before ORANGE-004 the same frames cost almost nothing. No
value is stolen or forged and consensus is untouched, so it is availability
only, and it is bounded by the peer's bandwidth and the node's cores, hence
medium. The cache-eviction effect alone is a performance degradation, not an
attack on correctness.

## Recommendation

Keep ORANGE-004's goal (verify outside the lock) and bring back the order of
the old checks:

- **Stop at the first failure**, in both `tx.Preverify` and the entry loop. An
  item with one bad witness is invalid as a whole, so nothing is lost, and
  for an attacker to make the node verify N signatures it must now supply N
  that pass (each remembered, so a resend is free). This alone removes the
  amplification.
- **Preverify only what the checks would reach.** Under the brief engine
  lock that `PreverifyTx` already takes, resolve the transaction the way
  `CheckAgainst` does (mint claimed, every input unspent at the selected
  tip, same mint, conservation) and skip it if any cheap check fails; verify
  witness `i` only when `Address(h, w.Policy)` equals the address of input
  `i`, as `CheckCached` does first. For an entry, skip it when its parent is
  not held (the `ErrUnknownParent` case), and verify only transactions that
  pass the same cheap checks against the entry's parent view.
- **Do not count on `MinEntryStrength`** as the entry's gate: it is 0 by
  default. The two points above make the gate independent of it.
- **Charge the sender for a bad signature.** A witness that fails
  `Satisfy` is not one of the "honest reasons" a transaction is refused for.
  Let `Preverify` report a failed witness and have `handlePushed` record an
  `InvalidItem` offence for it (entries already do for `ErrInvalidEntry`).
- Optionally, a small list of recent failures keyed like `Verified` (policy,
  data, tid), as ORANGE-004 offers, so a resent bad item costs only a hash.
- Fix the text of ORANGE-004/D105: preverify did not verify "only what
  `Handle*` would verify".

Tests: in `pkg/engine`, a transaction whose inputs do not exist (and one
whose first witness is bad) must cause no, respectively one, call of the
satisfaction function (count calls with a registered counting policy type);
an entry on an unknown parent must cause none; `Remembered()` must not grow
for a transaction that spends no real input. An end-to-end `pkg/p2p` test
that a peer sending transactions with bad signatures earns an offence.
