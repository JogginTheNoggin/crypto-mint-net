# Source Round 40: compatibility review (White, Part D)

## Summary

White's review of Grey's base #353 (D204: the tool renamed `poon`, W-36,
GUI-GREEN-065), #358 (D205: every old name, the RPC hello label
included), #363 (D206: the protocol's labels `PoO-` to `PoON-`, a label
change jerry approved), Green's GREEN-065 (#356), Orange's ORANGE-050
(#360) and Blue's BLUE-041 (#365, D207, which carries #356 and #360), at
`pr365` head `2343bc5`. **Nothing is breaking and nothing blocks the
round.** No new asks before the tag.

The rename is the largest change since WHITE-001, and it lands at the
right time. Nothing has been released, so new labels, names and ids cost
nothing beyond a fresh start for anyone running a dev build. After the
tag, the same change would have meant a migration and a new network.

- **Byte form v1 and wire version 1 are now frozen with the `PoON-`
  labels at the first tag** (D76, D206). The layouts and limits White
  froze in Rounds 32 to 39 are unchanged.
- **W-26 (rename after a release) is closed.** The rename landed
  before the first release, so no reader needs the old kinds, folder or
  variables.
- **The fresh start (D206, D207, GUI Green #359) is the right call.** A
  node pointed at an earlier build's files refuses to start, so nothing
  is half-read. `docs/running-a-node.md` says how to start fresh.

White checked `main` @ `b94d861` and `pr365`:
- No `PoO-` label, standalone `poo` word, `.poo` or `POO_` is left in any
  `.go`, `.sh`, `.yml` or `.js` file.
- The old default series id `fe52c1ac…` appears nowhere.
- `scripts/release.sh` builds `poon-*` and `poon-gui-*`.
- `docs/allowed_signers` and `docs/releasing.md` use `poon-release`.
- `go test ./pkg/...` passes on `main`, vectors included, and the bounty,
  store, node and BLUE-041 tests pass at `2343bc5`.

Nothing needs the owner.

## Verdicts

| Item | Verdict | Notes |
| --- | --- | --- |
| D204: binary, GUI, data folder, `POON_*` variables, file kinds, temp prefix, metric prefix, hello agent, signers' principal renamed via `pkg/brand` | compatible | Before any release. The `pkg/brand` design (one edit plus the frozen list) did its job |
| D204 W-36: memo id frozen at 16 bytes, never shorter | compatible | Done |
| D204 GUI-GREEN-065: `asked`, held `filled`, limits' `zero` | compatible | Added output fields |
| D205: RPC hello label `poon-rpc-hello`; records renamed with `git mv` | compatible | A client and node from before D205 no longer prove each other, which is correct, since both are dev builds. From the tag this label is frozen (brand.go says so) |
| D206: every `PoO-` label is now `PoON-` (consensus hash, key, snapshot and log labels); vectors and fixed ids regenerated; new default id `45772e75…66f4c5` | compatible | Approved by jerry, and Black's to confirm on the consensus side. For compatibility: layouts, field orders and limits are unchanged, so every White freeze still holds. An earlier build is a separate network, not a fork, and the snapshot and log tags make a node refuse its files instead of misreading them |
| GREEN-065 (#356): usage spacing, zsh completion `_poon` | compatible | Shell completion is named after the binary |
| ORANGE-050 (#360): no regression; benchmark fix | compatible | Test only |
| BLUE-041 RED-122: `POON_VIEW_ONLY` fails closed (on for any value but empty, `0`, `false`, `no`, `off`) | compatible | It widens what turns the safety switch on. After the tag, narrowing it again would be a behaviour change for W-8. Widening is the safe direction, and it's right before the tag |
| BLUE-041 RED-122: "Data from earlier builds: start fresh" | compatible | Answers GUI Green #359 |
| BLUE-041 RED-123: held offers past 16 layers read filled; 8-char memos and old temp files not handled | compatible | Display only, plus leftovers from dev builds, which start fresh |

## The freeze list after the rename

These are fixed from `v0.1.0-alpha.1` on. Each is listed in
`pkg/brand`, wire.md or the module docs:

- Consensus and key labels `PoON-*`, including `PoON-wallet-<name>-v1`
  and `PoON-wallet-seed-v1`. A change gives every wallet new keys.
- The RPC hello label `poon-rpc-hello`.
- The snapshot magic `PoON-snap` and the store tag `PoON-log`.
- Byte form v1 and wire version 1, with the limits in wire.md (W-14,
  W-18, W-19), the bounty v1 layouts, the 1 KiB offer bound and the max
  strength byte (W-29, W-32, D202).
- The file kinds `poon-*` with format 1, the done file's line start
  (W-31), the bounty memo start `bounty ` and its 16-byte id (W-34,
  W-36), and the wallet leg state names (W-34).

Display names stay free to change: the network's name, the default
series' display name and the site domain (jerry, 2026-11-06). None of
them is in a byte form, a label or a file kind.

## What Grey should take into Round 41 Part A

1. Merge #365 (with #356 and #360) and Black's Part D PR.
2. Optional: a CHANGELOG line under Unreleased, "builds from before D206
   share no ids or files with this one; set up fresh".

## Release readiness after this round

From White's side, nothing is left before the freeze. What remains is the
owner's:
- license;
- at least one registry node and its URL;
- optional maintainer keys in `docs/allowed_signers`;
- making the repo public, which also allows public downloads from
  GitHub Releases and gives Actions its runners.

Then the final review round, the freeze and the tag steps in WHITE-001.
The release archives will be named `poon-<version>-<os>-<arch>` and
`poon-gui-…`. Outside this repo, the website's download names and any
shown series id will need updating at the tag (the Website team's job,
noted to the coordinator).
