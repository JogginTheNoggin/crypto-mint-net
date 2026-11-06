# Source Round 16: consensus review (Black, Part D)

## Summary

No change in this round adds or alters a validity, weight, selection or
byte-form rule, and no team proposed a consensus change, so **nothing is
passed to the owner for approval**. **Nothing is blocked.**

The round touches the wallet, the CLI, the local RPC, the data-directory
lock and how files are written to disk. No consensus package changed:
between Round 15's base and this one, and in each of this round's PRs,
the only Go packages touched are `cli`, `filter` (its flags), `fsutil`
(new), `nodepolicy`, `peers`, `rpc`, `tracking` and `wallet`. `chain`,
`engine`, `entry`, `ledger`, `layer`, `weight`, `state`, `p2p` and `wire`
are untouched.

The parts that come near what a node keeps are these. Black checked each
for anything that could stop a heavier ledger being taken (D115):
- The entry body's buffer (D147, ORANGE-026, RED-040). It is still a
  memory hint with no size bound.
- The peer book and tracker are now written with syncs (RED-041). Their
  contents and when they are saved are unchanged.

| Field | Value |
| --- | --- |
| Round | Source Round 16 |
| Consensus suggestions | None |
| Base reviewed | `main` @ `b502960` (PR #70, D147) |
| Previous base | `claude/m10-watch` @ `46bcb9e` (since merged to `main`) |
| Blue branches | PR #75 @ `7e4813d` (BLUE-017, D148: RED-040, RED-041, RED-042) |
| Proposals | GREEN-041 (PR #73 @ `8544408`); ORANGE-026 (PR #74 @ `5eb90f0`) |
| Result | 10 clear, 0 with conditions, 0 blocked |

## Round 15's items

| Item | Now | Checked how |
| --- | --- | --- |
| Black's preference: keep `entryHint` and lower its ceiling | Met (D147) | `entryHint` is 64 MiB. The buffer doubles as bytes arrive, up to the stated length, and there is still no bound on the size read |
| No size bound on the entry body | Still true | Neither D147, ORANGE-026 nor BLUE-017 adds a limit on `POST /v1/entry` |

## The entry body (D147, ORANGE-026, RED-040)

- **Same bytes, same checks.** `readEntryBody` reads the body whole and
  decodes it with the same JSON rules as before. The entry still goes
  through `HandleEntry(..., Pushed)` with every check. `poon submit entry`
  now sends the hex as it was read instead of encoding it again
  (`SubmitEntryHex`); the node gets the same JSON.
- **No size bound.**
  - ORANGE-026 only changes where the buffer starts when the stated size
    is past the hint: the stated size halved until it fits under the
    hint, so the doublings end at exactly the stated size. The start is
    never more than `entryHint`+1, and growth past it still follows the
    bytes that arrive.
  - RED-040's fix sets `Content-Length` on the client's request, so the
    node sizes its buffer by it. A false length cannot cut an entry
    short: Go's server reads exactly the stated length, and a wrong one
    is an error for that request only, the same as before.
- This is the full token's local route only. Entries from peers do not
  pass through it.

## Durable writes (RED-041)

`fsutil.WriteFile` replaces five copies of write-then-rename with one
that syncs the file and its directory. For the peer book and the tracker,
the data written and when it is written are unchanged: `Save` copies the
records under the lock and writes outside it, so a slower write never
holds up the host. A crash now leaves the old or the new file whole,
never an empty one. Neither file can refuse a ledger: no ban, bad list or
depth is added, and a lost or old file only forgets peers.

## Clear

| Item | Why it is clear |
| --- | --- |
| D147: GREEN-040, `poon submit` reads a `-bytes` reply | CLI input only |
| D147: ORANGE-025 with the 64 MiB hint | Memory only, with no bound (see above) |
| D147: GUI ORANGE-012, submit sends the hex as read | The node gets the same JSON and runs the same checks |
| D147: `nodepolicy.Lock` on Windows | The local data-directory lock only. It refuses a second process, as on Unix |
| D147: key-only wallet commands hold the wallet after their prompts | The wallet file only |
| BLUE-017 (RED-041): one durable writer | Storage only. The peer book and tracker keep the same contents (see above) |
| BLUE-017 (RED-040): submit sends its length | The client's request header. No size bound on the node |
| BLUE-017 (RED-042): status and receive ask the node unheld; waiters say why | Wallet and CLI only. The wait having no limit concerns local commands, not what the node accepts |
| GREEN-041: help lists only the flags a command takes | Flag registration and help text. `-lease` is now only on `grind start`, and `-kind` is not offered on views, where it selected nothing |
| ORANGE-026: the buffer ends at the stated size | Memory only, with no bound (see above) |

## Items for the owner

- **No consensus change is proposed this round.** Nothing needs your
  approval.
- **No hard-fork case is open.** Entries of any size are still taken
  from peers and from the node's owner alike, and nothing this round can
  make a node refuse a heavier ledger.

How this was checked:
- Read the code changes from `46bcb9e` to `b502960` (PR #70), the diffs
  of PRs #73, #74 and #75 against `main`, and decisions D147 and D148.
- `go test ./pkg/rpc ./pkg/cli ./pkg/wallet ./pkg/peers ./pkg/tracking
  ./pkg/nodepolicy ./pkg/filter ./pkg/vectors ./pkg/p2p` passes on
  `8544408`, `5eb90f0` and `7e4813d`, and `./pkg/fsutil` passes on
  `7e4813d`.
