# pkg/wallet

## Summary

The wallet keeps a holder's keys and spends their portions. It holds one
**seed**, sealed with a passphrase, and derives every key from it, so the
seed alone restores the wallet. It gives out a **fresh address for every
payment**, records **shared k-of-n policies**, chooses which portions to
spend, builds and signs **one transaction per mint** used, writes payments
that still need **co-signers** to a file they sign, makes **payment
requests** that carry the receiver's policy so the payer can check it, and
follows every transaction it sent or expects: **pending**, **selected**,
**dropped** or **void**, with its weight from(e) and the heaviest **known
rival**. These are the wallet habits of the design's section 17.6 and the
receiver's rule of sections 7.4 and 7.6. None of it is a protocol rule:
the protocol sees only transactions and witnesses, and the wallet never
decides validity. It talks to a full node through the `Node` interface, so
it runs against a local node (`rpc.WalletNode`), a remote one, or an
engine in a test. `poon wallet` is its command line ([poon.md](poon.md)).
**Light mode** is a full node whose node policy follows only the pools the
wallet watches (`poon wallet follow`).

## The wallet file

`wallet.json` (default in `$POON_HOME` or `~/.poon`, written with mode 0600,
atomically and synced) holds:

| Field | What |
| --- | --- |
| `alg` | the key algorithm of every key (absent: Ed25519) |
| `kdf`, `sealed` | the seed sealed with the passphrase |
| `keys` | every derived key: index, public key, label, whether given out |
| `gap` | how many unused keys to keep ahead (default 20) |
| `policies` | shared k-of-n policies the wallet holds keys of |
| `watch` | the mints, per series, the wallet looks for its portions in |
| `sent` | payments and claims made, with each leg's transaction and state |
| `expected` | payment requests given out, and transactions payers named |

`Save` writes only over the file it read. It takes a short lock beside
the file (`wallet.json.lock`; flock on Unix, LockFileEx on Windows), reads
the file again, and writes only if the bytes are those `Open` read or the
last `Save` wrote. Otherwise it writes nothing and returns `ErrChanged`,
so a command left waiting at its passphrase prompt never undoes another
command's change, such as a new passphrase. A new wallet whose file
appeared meanwhile returns `ErrExists` (GUI-019, D145).

A command that acts on the node and then saves (`poon wallet pay`,
`claim`, `submit`, `resend`) first calls `Hold`, after its passphrase
prompt: it takes the same lock until the command ends and reads the file
again, so what another command saved meanwhile is kept and the change
address is a key nobody else took. No other `Save` can come between the
send and the save, so the record of a payment that went out is never
refused; another command's `Save` waits, and is refused if it read the
file before. If the save fails anyway (a full disk, a removed file), the
command prints the record and says not to run it again, since that would
pay a second time (RED-039, D146).

The other commands that save (`passwd`, `address`, `watch`, `request`,
`receive`, `status`, `policy`) also call `Hold` after their prompts, just
before they change the wallet. So a change saved while one of them waited
at a prompt is read in and kept, and the command saves without
`ErrChanged` (D147).

The one change `Hold` does not take in is a new passphrase. If the file's
seal changed after the command was unlocked, `Hold` returns
`ErrPassphraseChanged` and the command does nothing, so changing the
passphrase also revokes commands already waiting with the old one, and a
stale `passwd` never re-seals over it (GUI-022, D149).

`status` and `receive` ask the node without holding the wallet: `status`
holds it only to save the legs whose state changed, each only where the
file still has the state it read, and `receive` only to note the
transactions `-tid` names. A command that must wait for the lock says so
on its error output ("waiting for another poon wallet command (or the GUI)
to finish with ..."), in the GUI's terminal too, then waits for as long
as the holder needs: a holder is sending a payment and saving its record,
and giving up would leave a change or a record unsaved (`OnBusy`, RED-042,
D148).

Every save is durable: the file is written to a temporary file of its
own, synced, renamed over `wallet.json`, and the directory synced
(`fsutil.WriteFile`), so a crash leaves the old file or the new one,
never an empty or short one (RED-041, D148).

Public keys are in the clear, so a locked wallet still lists its
addresses, reads balances and follows payments. Only signing, giving out a
new address, and showing the seed need the passphrase.

## Keys and the seed

- **Sealing (decisions D64, D95).** The 32-byte seed is sealed with
  AES-256-GCM under a key from Argon2id of the passphrase: a random 16-byte
  salt, 3 passes over 64 MiB in 4 lanes (stored with the file, so they can
  be raised later). Wallets made before D95 use PBKDF2-HMAC-SHA256 at
  600,000 iterations and still open; `Options.KDF` can still choose it.
  Opening refuses settings outside fixed limits, so a damaged file cannot
  make an unlock exhaust memory. The additional data `PoON-wallet-seed-v1`
  binds the ciphertext to its purpose. A wrong passphrase fails
  authentication: `ErrPassphrase`. `Rekey` seals the same seed under a
  new passphrase, moving a PBKDF2 wallet to Argon2id. When `poon wallet
  create` or `poon wallet passwd` reads a new passphrase as it is typed (a
  hidden terminal, or the GUI's masked field), it asks for it again and
  changes nothing if the two differ; one piped on standard input, or
  given by `$POON_PASSPHRASE`, `$POON_NEW_PASSPHRASE` or a file, is read
  once (GUI-020, D145).
- **Key algorithm (decision D81).** Every key of a wallet uses the
  algorithm chosen at creation (`Options.Alg`, `poon wallet create -alg`),
  Ed25519 by default. It cannot change later. A series must allow it, or
  `NewAddress` refuses to give out an address there.
- **Derivation (decisions D63, D81).** Key *i*'s 32-byte key seed is
  HMAC-SHA256(seed, `PoON-wallet-<algorithm name>-v1` ∥ U32(i)), and
  `policy.NewSigner` makes the key from it. For Ed25519 the label is
  `PoON-wallet-ed25519-v1`, so wallets made before there was a choice keep
  their keys. Each key's address is its 1-of-1 threshold policy's address
  in the series.
- **Gap limit.** The wallet always derives `gap` keys past the last one
  given out and watches their addresses, so a restored wallet finds
  payments to addresses it handed out before, as long as no more than
  `gap` in a row went unused.

## Shared policies

`AddPolicy(name, k, keys, label)` records a k-of-n policy. A key given as
`self` takes a fresh key of this wallet; others are co-signers' public
keys. The policy is checked with `CheckPolicy`: the series must allow the
threshold policy type and every key's algorithm. Its address is spendable
by this wallet only with the other signatures.

## Paying

`Pay(series, node, to, amount, options)`:

1. **Spendable portions.** The node's holdings in each watched mint, kept
   where the address is one of the wallet's (every derived key, the gap
   included, and every shared policy), minus the inputs of the wallet's own
   legs that are unsigned or pending. Holdings are read at the selected tip
   only, so this reservation keeps two payments from spending the same
   portion before the first is selected.
2. **Coin selection (decision D65).** One mint if it can: the one needing
   the fewest inputs, then the least change. Else the pool needing the
   fewest mints (a payment within one pool settles under one weight).
   Else across pools, largest totals first. Within a mint, the smallest
   single portion that covers the amount, else largest first. `Mints`
   limits the payment to some serials. The aim is the design's habit of
   keeping value in few mints.
3. **Legs.** One transaction per mint: the amount from that mint to `to`,
   the rest to a fresh `change` address.
4. **Signing.** Every input is signed with the wallet's keys behind its
   address. If every input is complete, each leg is submitted at once.
   Otherwise nothing is sent: `Pay` returns an `Unsigned` payment.

A payment request (`Request`) names the series, address, amount, memo and
the **policy behind the address**. `Request.Check` decodes the policy,
checks it hashes to the address and that the series allows it, and
declines unknown policy types, so a payer never pays an address nobody can
spend. A request without a policy is paid as a bare address, unchecked.
`PayRequest` checks, then pays.

`Claim(series, node, serial)` claims a whole mint for a fresh address and
watches the mint. `ClaimUnsigned(series, serial, name)` prepares a claim
to the address of the wallet policy `name`, unsigned, records it so
`Submit` takes it once signed, and watches the mint: how a watch-only
wallet claims with a key kept off the node (`poon wallet claim
-to-policy NAME -out FILE`, then `poon sign`, GREEN-060). A claim needs a
witness per output, not per input, so its unsigned leg lists one policy
for its one output; `SignWith` and `Finish` count witnesses that way.

## Co-signing

An `Unsigned` payment holds each leg's transaction (witnesses with the
policy and no signatures) and, per input, the signatures gathered so far.
It is plain JSON, passed between co-signers as a file.

- `Cosign(series, unsigned)` adds this wallet's signatures (by matching its
  public keys against each input's policy) and reports how many.
- `Missing()` says how many signatures each input still needs.
- `SignWith(unsigned, hash, alg, signer)` adds one key's signatures with
  no wallet at all (`poon sign`); `Unsigned.Hash` names the series' hash so
  it needs no node (D193), and `Unsigned.Variable` marks a variable-supply
  series so its transactions are read with their part (RED-105).
  `DecodeLeg(i)` reads a leg and refuses one whose series is not the
  payment's (RED-104); `SignWith` checks each signature it adds (RED-106).
- A watch-only wallet (`Options.WatchOnly`, `File.WatchOnly`) has no
  seed: it follows addresses from public keys and prepares payments, and
  every call that needs a private key returns `ErrWatchOnly` (D193).
  `PayOptions.Change` names the change address; a watch-only wallet sends
  change back to the first input's address.
- `Submit(series, node, unsigned)` sends it once complete. It must be a
  payment this wallet made: the legs are matched by tid to a recorded
  payment, so its inputs stay reserved and its states are followed.

## Following payments

Each leg has a **state**:

| State | Meaning |
| --- | --- |
| `unsigned` | waiting for co-signers |
| `pending` | held by the node, not yet in the selected ledger |
| `selected` | in an entry of the selected ledger |
| `dropped` | not held by the node, every input still unspent: it can be sent again (`Resend`) |
| `void` | not held, and another transaction spent an input: the payment did not happen (design section 6.6) |

`Refresh` asks the node for each leg (`Payment`): its state, its entry and
from(e), the weight a rival must beat after the entry's parent, and the
**rivals** the node keeps on inactive branches: entries holding a
transaction that spends one of the leg's inputs, each with its branch's
weight after its fork. A leg is **ahead** when from(e) is greater than the
heaviest rival's weight after its fork (design section 7.3).

`Settled(legs, threshold)` is the receiver's rule: every leg selected,
from(e) at least `threshold`, and ahead of every known rival. Legs in
different pools are separate transactions under separate weights, so a
payment across pools settles only when its slowest leg does (section
7.6). The threshold is the receiver's choice, by how much the payment is
worth; the default 0 means "selected and ahead".

`Receive(series, node, expected)` is the receiving side: portions at the
requested address in the watched mints, plus the transactions the payer
named (`Expected.Tell`), each leg's amount read from its transaction.
`Receipt.Complete` is set when the units received (pending or selected)
reach the amount asked; `Receipt.Settled(threshold)` applies the rule
above.

## Light mode

A light node is a full node that follows less: `poon wallet follow` writes
the node policy's follow list for the series as only the pools of the
wallet's watched mints. The node still checks every rule for what it
follows, so the wallet trusts no one for the pools it uses. Verifying only
some mints of a pool is not done: a pool's entries carry every mint of the
pool, so the pool is the smallest unit a node can follow on its own.
When the node is running, `poon wallet follow` makes it apply the new list
at once ([daemon.md](daemon.md)), in one replace request, so the series is
never off the node's list in between (GUI-018, D139); otherwise the node
applies it when it starts.

## Tests

`wallet_test.go` runs wallets against an in-process engine:

- create, open, unlock, lock, wrong passphrase, rekey, and the gap a
  locked wallet still watches;
- a claim, a payment request paid with change, the reservation of open
  legs, the receipt going from pending to selected and settled, and a
  payment found without the payer naming it;
- a payment across mints of one pool, and `Mints` restricting it;
- a 2-of-2 policy co-signed through a JSON round trip;
- a hidden transaction going dropped and resent; a rival entry spending
  the same input, seen as a rival and then winning, which makes the leg
  void;
- request checks: wrong series, a policy that does not hash to the
  address, an unknown policy type.

`select_test.go` checks coin selection. `pkg/cli/wallet_test.go` runs the
whole flow through `poon wallet` against a running node, co-signing
included, restores from the seed, and checks `follow`.

## Extending

- **Another key algorithm.** Nothing to do here: once `policy.NewSigner`
  knows it, wallets can be created with it. `TestEveryKeyAlgorithm` claims,
  pays and receives with every algorithm.
- **Spending from hashlock or k-of-policies addresses.** The wallet signs
  threshold policies only. It accepts payments to any policy the series
  allows (`CheckPolicy`), but holding one would need the wallet to build
  k-of-policies witness data.
- **Another node.** Implement `Node` (three calls) over any transport.
- **Hardware or external signers.** `Unsigned` is the hand-off: an
  external signer adds signatures to its inputs like a co-signer.
- **Another coin selection.** `choose` is the only place it is decided;
  `select_test.go` pins the current behaviour.
