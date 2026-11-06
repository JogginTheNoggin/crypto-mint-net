# Source Round 35: consensus review (Black, Part D)

## Summary

This round adds cold keys: keys the node never holds (D193, jerry's
request). It covers:
- `poon keygen -raw` for a key made elsewhere;
- watch-only wallets;
- `poon sign` on any machine, with no node;
- `pay -change`;
- `wallet claim -to-policy`.

D192 settles White's last frozen wire limits (a 64 KiB extension total
for ping and pong, and `MaxHelloFrame` written into wire.md), the empty
token, neutral wording for the default series, and `pkg/brand`.

Blue's D194 fixes RED-104 to RED-108, GUI-032 and GUI-GREEN-061. It
carries Green's GREEN-060 and Orange's ORANGE-045, which signs each
transaction once per key.

**Result: 10 clear, 0 with conditions, 0 blocked.**

- **No consensus rule or depth limit is added.**
  - Transactions and witnesses are the same bytes. Only the machine that
    makes them changes.
  - `TestByteFormV1Frozen` passes on `main` and on PR #291, and the
    Round 24 and Round 25 cross-version tests give the same bytes on
    both.
- **Signing once per key gives signatures nodes accept.** Every
  witness of a transaction signs the same message, `policy.Message(tid)`.
  So one signature from a key is valid for every input that key owns. No
  rule asks for different signatures across inputs.
  - Black checked this through the pool's full path. A spend of two
    inputs owned by one key, with the second input given the first
    input's witness byte for byte, is accepted by `AddEntry`.
  - The control is refused: the same witness on a transaction with
    another tid fails "signature for key 0".
- **Claims made through `poon sign` follow the claim rules.**
  `ClaimUnsigned` builds the same claim as `Wallet.Claim`:
  - `CheckMint` first;
  - one output of the whole supply to the policy's address;
  - `s.Mint(serial)`, so a variable series' part is included;
  - one witness per output.

  The node checks the witness against the output's address, as for any
  claim.
- **`tx.DecodeVariable` is tool-side only.** Its one caller is
  `Unsigned.DecodeLeg` in the wallet. Node decoding (`DecodeFrom`,
  `IsVariableID`) is unchanged.

| Field | Value |
| --- | --- |
| Round | Source Round 35 |
| Consensus | None (D192 to D194) |
| Other consensus suggestions | None |
| Base reviewed | `main` @ `58273f7` (PR #284: D192, D193; GUI #285 on top) |
| Previous base | `main` @ `8ceb7db` |
| Blue branches | PR #291 @ `f7bd892` (BLUE-036, D194: RED-104 to RED-108, GUI-032, GUI-GREEN-061; carries GREEN-060 #289 and ORANGE-045 #286) |
| Red | PR #287 @ `3873b79` (RED-104 to RED-108) |
| Proposals | ORANGE-045 (PR #286 @ `c20079c`, carried in #291); GREEN-060 (PR #289 @ `515a05c`, carried in #291) |
| Result | 10 clear, 0 with conditions, 0 blocked |

## One signature per key per transaction (ORANGE-045)

`Wallet.addSigs` and `SignWith` now sign `policy.Message(tid)` once for
each key in a leg, and give that signature to every input the key
appears in, at that input's own key index.

- **Why it is valid.** `CheckCached` satisfies each witness with
  `t.Satisfy(params, data, Message(tid))`. The message is the same for
  every input of a transaction. A witness is judged on its own policy,
  data and the tid, never against the other witnesses.
- **No rule forbids it.** The transaction rules refuse a repeated input
  (`ErrRepeatedInput`) and a repeated output address
  (`ErrRepeatedAddress`). Nothing compares witnesses or signatures across
  inputs. The verified-witness cache, keyed by witness and tid, simply
  finds the second input's witness already verified.
- **Randomised schemes too.** ML-DSA and SLH-DSA signatures may differ
  from one signing to the next, but any valid one verifies. Reusing one
  is as valid as making a new one.
- **Black's check.** Black ran a ledger test outside the repo on
  `f7bd892`:
  - claim 60 to alice and 40 to bob;
  - spend bob's 40 to alice;
  - spend both of alice's portions to carol, with input 1 given input 0's
    witness byte for byte.

  `AddEntry` takes all three entries. A control that moves the same
  witness onto a transaction with another tid is refused with "witness
  does not satisfy the policy: signature for key 0".
- **Orange's test agrees.** `TestSignWithOncePerLeg` signs three legs of
  four inputs each, Ed25519 and ML-DSA-65, including a shared 2-of-2,
  with one signing per leg. Every finished witness satisfies its policy.

## Cold keys (D193, D194)

### The same transactions, made elsewhere

- **`poon keygen -raw` and `NewRawSigner`.** These take a key in its
  algorithm's raw form. Since RED-106 every key is self-tested: it signs
  a fixed message, which must verify under the public key it gives. A key
  that couldn't spend its own address is refused before it is used.
  Addresses come from the same policy encoding as `poon keygen`
  (`TestRawSignerIsTheSameKey`).
- **Watch-only wallets.** These hold no seed. They prepare the same
  unsigned payment `poon wallet pay -out` always wrote. The change goes
  back to the first input's address, or to `-change ADDR`. Either way it
  is an ordinary output to an address the owner chose. It spends nothing
  beyond the inputs the payment already lists.
- **`poon sign`.** It adds signatures to an unsigned file. Since RED-104
  and RED-106:
  - each leg must be in the file's series;
  - its transaction must spend the mint the leg names;
  - each signature is verified before it is written.

  The signed file is submitted with `poon wallet submit` and checked by the
  node like any transaction.
- **A file that lies costs only its owner.** A wrong `hash` or a wrong
  `variable` mark gives a tid that nodes don't compute. The signatures
  then fail on every node alike. That is an invalid transaction, the same
  verdict everywhere, never a disagreement.

### `tx.DecodeVariable` (RED-105)

- **Only the signer calls it.** `Unsigned.DecodeLeg` calls it when the
  file says `"variable": true`, or when the process has built the series
  as variable. No node, engine, ledger or p2p path calls it.
- **The node still decides by the series it built.** `DecodeFrom` still
  reads a part only when `series.IsVariableID` knows the series, as in
  D166.
- **The tid covers the bytes either way.** `writeBody` writes the part
  only when it isn't 0, and a variable series' parts start at 1
  (`PartOf`). So a leg read with `DecodeVariable` re-encodes to the same
  body, and the signer signs the tid the node will compute.

### Claims through `poon sign` (GREEN-060, `wallet claim -to-policy`)

`ClaimUnsigned` builds `tx.Tx{Mint: s.Mint(serial), Outputs: [{Address:
s.Address(p), Amount: Supply}]}` with one witness slot for policy `p`,
after `tx.CheckMint` and `CheckPolicy`. That is the claim `Wallet.Claim`
builds, with only the address source different. `witnessCount` counts
outputs for a claim, as `tx` does. The node checks the claim exactly as
any other, including the competing-claim rules (D161).

## D192: frozen limits and names

- **The 64 KiB extension total for ping and pong (W-18).** A ping or pong
  over it fails inside the extension list, so it is `ErrBadExtras`: the
  connection ends with no offence (Round 34). It is frozen with wire
  version 1. This matches what Black asked of any discovery limit in
  Round 34. The largest valid ping is now about 64 KiB, and pings at and
  over the total are tested.
- **`MaxHelloFrame` in wire.md (W-19).** It is now part of the frozen
  set, as Round 34 noted it had to be.
- **The rest.** Unknown ledger-file header keys skipped (W-20), release
  signing steps (W-21), `rpc.ErrEmptyToken`, the neutral default-series
  wording and `pkg/brand`. The default series' id and file are
  unchanged. All of these are tooling and names.

## Clear

| Item | Why it is clear |
| --- | --- |
| D192 (W-18, W-19): 64 KiB extension total for ping and pong; `MaxHelloFrame` frozen in wire.md | Wire limits on optional fields. Over the total is `ErrBadExtras`, with no offence |
| D192: W-20, W-21, empty token, neutral wording, `pkg/brand` | Tooling and names. The default series is unchanged |
| D193: `poon keygen -raw`, `NewRawSigner` (self-tested since RED-106) | The same policies and addresses. A key that can't sign for its address is refused |
| D193: watch-only wallets, `pay -change` | The same unsigned payment. The change is an ordinary output |
| D193, D194 (RED-104): `poon sign` shows and checks series, mint and each signature | The same witnesses, made on another machine. Nodes check them as ever |
| ORANGE-045 (carried in #291): one signature per key per transaction | Every witness signs `Message(tid)`. Accepted by the pool's full checks (Black's test) |
| GREEN-060 (carried in #291): `wallet claim -to-policy`, `keygen -out` | The same claim `Wallet.Claim` builds. Checked by the node as any claim |
| D194 (RED-105): `Unsigned.Variable`, `tx.DecodeVariable` | Tool-side only. Node decoding is unchanged. The tid covers the same bytes |
| D194 (RED-107, RED-108, GUI-032, GUI-GREEN-061): ledger file read twice; header checks; key files never echoed; no unrecorded pay | Tooling. Every imported entry is still fully checked |
| Red (PR #287): RED-104 to RED-108 | Findings, fixed in #291. None touches a rule |

## Items for the owner

- **Nothing needs your approval this round.**
- **A payment signed on an offline machine is exactly the payment the
  wallet would make.** Nodes check it the same way, and a wrong or
  tampered file is refused by every node alike.
- **Signing once per key is valid.** Every input of a payment signs the
  same message, so one signature covers every input that key owns. Black
  confirmed a node accepts it.

How this was checked:
- Read D192 to D194, `wallet/pay.go` (`addSigs`, `SignWith`,
  `ClaimUnsigned`, `Claim`, `DecodeLeg`, `witnessCount`), `tx/tx.go`
  (`decodeFrom`, `DecodeVariable`, `writeBody`), `policy/raw.go`,
  `policy.CheckCached`, `policy.Message`, `series.PartOf`, `wire/hello.go`
  (`MaxSmallExt`, `ErrBadExtras`), and the diffs of PRs #286, #287, #289
  and #291.
- Black's ledger test of a shared signature across two inputs, with a
  control, on `f7bd892`.
- The Round 24 and Round 25 cross-version tests and `TestByteFormV1Frozen`:
  identical on `58273f7` and `f7bd892`.
- `go test ./...` passes on `58273f7`, #286, #287, #289 and #291. The
  wallet, policy, tx, cli and series packages pass under `-race` on
  `f7bd892`.
