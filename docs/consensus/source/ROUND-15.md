# Source Round 15: consensus review (Black, Part D)

## Summary

No change in this round adds or alters a validity, weight, selection or
byte-form rule, and no team proposed a consensus change, so **nothing is
passed to the owner for approval**. **Nothing is blocked.**

Most of the round is the wallet and the CLI, which decide nothing a node
accepts:
- D145 adds the wallet's compare-and-write lock and confirms new
  passphrases.
- BLUE-016 holds the wallet lock until a command saves.
- GREEN-040 lets `poo submit` read a `-bytes` reply.

The parts that touch entries are the RPC's entry body (D145's streamed
hex, ORANGE-025's single read). Black checked them for a size bound and
found none. The bytes handed to `entry.Decode` are the same as before,
and the entry goes through every check a pushed entry gets.

| Field | Value |
| --- | --- |
| Round | Source Round 15 |
| Consensus suggestions | None |
| Base reviewed | `claude/m10-watch` @ `46bcb9e` (code `615159c`; D145) |
| Previous base | `claude/m10-watch` @ `184bc54` |
| Blue branches | `claude/project-thread-ggo8dt` @ `26dae4a` (BLUE-016, D146: RED-039, PR #65) |
| Proposals | GREEN-040 (`claude/source-green` @ `9be787c`); ORANGE-025 (`claude/source-orange-r15` @ `7d5a04c`) |
| Result | 7 clear, 0 with conditions, 0 blocked |

## Round 14's items

| Item | Now | Checked how |
| --- | --- | --- |
| BLUE-015, the lifted body cap | Merged as cleared (D144) | Full-token `POST /v1/entry` is unbounded. Every other route and the view token are bounded |
| Black's memory note | Met (D145) | The hex is decoded straight from the body (`hexBytes`), and the JSON is unchanged |
| Black's optional staleness note | Not taken (D145) | Grey's reason is sound. Tying a waiting file's staleness to `measureLasts` would favour the file that is not moving. Either way it is a delay paid for in bytes, so it is no D115 matter |

## The entry body (D145, ORANGE-025)

- **Same bytes.** `hexBytes` decodes the same hex as `decodeHex` did:
  either case, even length, or "not hex". An escaped or non-string value
  falls back to the usual JSON decode. So `entry.Decode` gets the same
  bytes it did before, and `HandleEntry(..., Pushed)` runs every check.
  Nothing is skipped or added.
- **No size bound.**
  - D145 adds no limit.
  - ORANGE-025's `entryHint` (1 GiB) is only how much the buffer
    reserves up front from the stated `Content-Length`. Past it, the
    buffer grows as the bytes arrive, so an entry of any size is still
    read.
  - It is the full token's route only, as in Round 14.
- **ORANGE-025's open question: should the buffer be sized from the
  stated length?** Either answer is fine for consensus, since the size
  only changes when memory is reserved, never whether an entry is read.
  - Black's preference is to keep the hint and lower its ceiling, for
    example to 64 MiB. A wrong or hostile `Content-Length` then reserves
    little up front, and a real large entry grows the buffer as it
    arrives.
  - The only client is the full token, so this is tidiness, not a fix.
- **`encoding/json/v2` with `DefaultOptionsV1` and unknown members
  refused.** This is how the local request is parsed. It has no bearing
  on any byte form or on what the node accepts from peers. The tests
  pass on `7d5a04c`.

## Clear

| Item | Why it is clear |
| --- | --- |
| D145: GUI-019, the wallet compare-and-write lock | The wallet file only |
| D145: GUI-020, confirming a new passphrase | The CLI and GUI only |
| D145: GREEN-039 adopted, and the streamed hex | Output, hints and the local RPC's parsing. The same bytes reach the same checks (see above) |
| BLUE-016 (RED-039): pay, claim, submit and resend hold the wallet lock until they save | Wallet and CLI only. What the node is sent, and what it accepts, are unchanged |
| GREEN-040: `poo submit` reads a `-bytes` reply | CLI input only. The hex it finds is decoded and submitted as before |
| ORANGE-025: the entry body read once | Memory only, with no bound (see above) |
| Red PR #65 (RED-039) | The finding. Its fix is BLUE-016 |

## Items for the owner

- **No consensus change is proposed this round.** Nothing needs your
  approval.
- **No hard-fork case is open.** This round's changes are to the wallet,
  the CLI and the local RPC. Entries of any size are still taken from
  peers and from the node's owner alike.

How this was checked:
- Read the code changes from `184bc54` to `46bcb9e`, BLUE-016's,
  GREEN-040's and ORANGE-025's diffs, and decision D145.
- `go test ./pkg/rpc ./pkg/cli ./pkg/wallet ./pkg/vectors` passes on
  `26dae4a`, `9be787c` and `7d5a04c`.
