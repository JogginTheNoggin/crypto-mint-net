# Source Round 35: compatibility review (White, Part D)

## Summary

White's review of D192 (#284), D193 (cold keys, #284), Green's GREEN-060
(#289), Orange's ORANGE-045 (#286) and Blue's BLUE-036 (#291, D194,
which carries #289 and #286), at `pr291` head `f7bd892` on `main` @
`58273f7`. **Nothing is breaking and nothing blocks the round.** D192
did every White item queued for it: W-18, W-19, W-20, W-21, README
alpha, neutral default-series wording, the "poo" name in `pkg/brand`,
and the registry URL rule. The new commands and flags add to what was
there and change no byte form, wire layout or setting.

The cold-key work adds **two files that cross machines and releases**:
the unsigned payment (`poo wallet pay -out`, `wallet claim -to-policy
-out`, signed by `poo sign`) and the watch-only wallet. An offline
signer is the machine least likely to be upgraded, so these files will
meet older and newer releases more often than any other. White asks for
three small changes before the tag, while the first release's readers
can still be shaped, and records one rule for the rename:

- **W-23.** Give the unsigned payment file a `kind` and `format`, and
  have `poo sign` keep the keys it does not know when it writes the file
  back.
- **W-24.** Have the wallet file keep the fields it does not know when
  it saves, as `node.json` does (D184). Today an older release that
  opens a newer wallet drops the newer fields on its next save.
- **W-25 (rule).** A field that older readers must not ignore, such as
  `watch_only`, comes with a new `format`. It doesn't apply to
  `watch_only` itself, since every release knows it from the first tag.
- **W-26 (rule, for the rename).** If the "poo" rename comes after a
  public release, readers must still accept the old file kinds, the old
  data folder and the old `$POO_HOME`. Before the first release, nothing
  is needed.

Tests: `go build ./...` and `go test ./pkg/wallet ./pkg/policy ./pkg/tx
./pkg/cli` pass at `f7bd892`. No consensus question arose; Black owns
the D193 confirmation (same transaction bytes, ORANGE-045 shared
signature). Nothing needs the owner.

## Verdicts

| Item | Verdict | Notes |
| --- | --- | --- |
| D192 W-18: ping/pong extensions 64 KiB in all (`wire.MaxSmallExt`), frozen | compatible | Done as asked; tests at and over the total |
| D192 W-19: `wire.MaxHelloFrame` 3 MiB written as frozen in wire.md | compatible | Done |
| D192 W-20: ledger reader skips unknown header keys (4 KiB each, at most 64, none twice) | compatible | Done. A file from a later release imports |
| D192 W-21: release signing steps, `docs/allowed_signers`, principal `poo-release`, no key tied to one person | compatible | Done. The file holds no key until a maintainer adds one; signing stays optional for the alpha (WHITE-001 R8) |
| D192: README alpha | compatible | Done |
| D192 RED-W-021: RPC client refuses an empty token | compatible | Client side only; a node set up normally has a non-empty token, so only a misconfigured client is refused, with a clear error |
| D192: `poo init -default-series` (was `-reserve`), `"builtin": "default"` | compatible | A renamed flag and output value, both before any release, both in the changelog. After the tag, a rename like this keeps the old name working for a release (W-8). One stale comment still says `"reserve"` (`pkg/cli/node.go:412`) |
| D192: `pkg/brand` | compatible | The frozen strings are right: `PoO-wallet-` and `poo-rpc-hello` must never follow a rename. **Note:** `KindPrefix`, `DataDir` and `$POO_HOME` *do* follow it, which is right before the first release and needs W-26 after it |
| D192: no built-in registry URL yet; one default in code, never written to node.json | compatible | Matches what White asked for |
| D193: `poo keygen -raw -seed-file F` | compatible | Reads each algorithm's standard raw private key (RFC 8032 seed, FIPS 204 seed, FIPS 205 key, SEC1 scalar), so keys from other tools keep working across releases. New flag only |
| D193 + GREEN-060: `poo keygen -out` | compatible | Writes a seed in hex, the same form `-seed-file` already reads |
| D193: `poo wallet create -watch-only` (`"watch_only": true`, `format` 1) | compatible with conditions | Every release from the first tag knows the field, so `format` 1 is fine. A pre-D193 dev build would ignore the field and drop it on save, which is W-24's problem in general. **Condition:** W-24 before the tag; W-25 for the next such field |
| D193: `poo sign FILE -key-file K [-raw] [-hash] [-out] [-show]` | compatible with conditions | `-hash` reads files from before D193. **Condition:** W-23 |
| D193: the unsigned payment file gains `hash` | compatible | Optional; older files still sign with `-hash` |
| D193: `poo wallet pay -change ADDR` | compatible | New flag. The default change address is unchanged for seeded wallets |
| GREEN-060: `poo wallet claim -to-policy NAME -out F` | compatible | Writes the same unsigned payment file; a claim leg has one partial witness per output (`witnessCount`), which `poo sign` from this release reads. Covered by W-23 for later releases |
| ORANGE-045: one signature per key per transaction | compatible | No file or byte form changes: each input still carries its own signature entry, now the same bytes. Black confirms the consensus side |
| BLUE-036 RED-104: `poo sign` shows `series`, `hash`, `spends`; refuses a leg of another series | compatible | Output fields only added |
| BLUE-036 RED-105: `"variable": true` in the unsigned file; `tx.DecodeVariable` | compatible | A file without the mark still reads for a series the process has built, and a clear error names the mark otherwise. A signer older than D194 ignores the mark and fails to decode a variable leg with a plain error, which fails safe. This is the case W-23 makes explicit |
| BLUE-036 RED-106: raw keys self-tested | compatible | Refuses only a key that could never spend |
| BLUE-036 RED-107: ledger file read twice when entries come first; a pipe is refused for such a file | compatible | Files written by `poo ledger export` put the header first, so they still import from a pipe. Only a hand-ordered file needs a path. Changelog says so |
| BLUE-036 RED-108: dry run checks header keys after the entries | compatible | Reads the same files |
| BLUE-036 GUI-032: key file errors never echo path or contents | compatible | Messages only |
| BLUE-036 GUI-GREEN-061: `wallet pay` and `wallet passwd` refuse early on a watch-only wallet | compatible | Refuses before anything is recorded, so no wallet file is left half-changed |
| Docs: CHANGELOG, FEATURES, modules/poo.md, modules/wallet.md, running-a-node.md | compatible | Each new command and flag is listed. Release-notes-ready |

## W-23: the unsigned payment file needs a kind and format

The file `poo wallet pay -out` writes and `poo sign` reads is plain JSON
with no `kind` and no `format`. Every other file the tool writes for sharing
has both (`poo-ledger`, `poo-peers`, `poo-node-policies`). This file is
the one most likely to cross releases. The signer is an offline machine
that may stay on one release for years, while the wallet machine
upgrades.

Today two things go wrong across releases:

1. **A newer file read by an older signer.** Unknown keys are ignored
   silently. RED-105 shows the risk: a signer from before D194 ignores
   `"variable": true`. It fails safe there, because the decode errors,
   but a later field that changes meaning might not fail at all. The
   signature covers the tid, so nothing can be stolen. What can go wrong
   is that `-show` shows the wrong thing, which is the check RED-104
   added.
2. **An older signer writing the file back.** `poo sign` unmarshals into
   `wallet.Unsigned` and writes that struct back, so any key it does not
   know is lost. `poo wallet submit` on the newer wallet then sees a
   file missing what it wrote.

**Ask (before the tag):**

- Write `"kind": "poo-payment"` (from `brand.KindPrefix`) and
  `"format": 1` in every unsigned payment file.
- Readers accept a file with no kind and no format as format 1, so files
  from dev builds and from before this change still sign.
- Readers refuse a `format` above the one they know, with a message
  that names the release needed. They refuse a wrong `kind`.
- Within a format, unknown keys are kept on write-back: hold the top
  level and each leg as `map[string]json.RawMessage` alongside the
  struct, the way `node.json` keeps unknown settings (D184). Only
  fields an older signer can safely ignore go into an existing format.
  Anything else bumps it (W-25).
- Test: a file with an unknown top-level key and an unknown leg key
  signs, and both keys come back unchanged. A file with `format` 2 is
  refused.

## W-24: the wallet file should keep fields it does not know

`wallet.Save` marshals `wallet.File` and writes it. Any field a later
release added is gone after one save by an earlier release: a watch
command, a policy add or a payment is enough. W-1 fixed the same problem
for `node.json` (D184). The wallet matters more, because it holds the
only record of payments sent (RED-041).

The first release's reader is the one every later release must survive
a rollback to. So this has to be in `v0.1.0-alpha.1`, or the first
rollback that matters will lose data.

**Ask (before the tag):** have `wallet.Open` keep the top-level keys
`File` does not know, and have `Save` write them back unchanged. Use the
same limits as `node.json` (256 keys, 64 KiB each, 64 levels deep). As
with W-11, a new field goes at the top level or in a new object, not
inside a list entry, whose unknown fields are not kept. Test: a wallet
with an unknown key survives `Save`.

## W-25 (rule): a field old readers must not ignore needs a new format

`watch_only` changes what the file means: a wallet with no seed. A
reader that ignores it would treat the wallet as sealed with an empty
seal. That's harmless here, because every release from the first tag
knows the field. After the tag, any field like it (a new seal type, a
new key kind, a spending rule) comes with `format` + 1, and the older
release refuses the file with a message instead of misreading it.
Fields an older reader can safely skip stay in the current format and
are kept by W-23 and W-24. White checks this on every new field in a
shared file. Added to the per-round checklist.

## W-26 (rule): the "poo" rename after a release

`pkg/brand` makes the rename one edit, and that is right for code. If
the rename lands **before** the first public release, nothing else is
needed. If it lands **after**, three things would move with it and
strand existing users:

| What moves | What breaks | What the rename must add |
| --- | --- | --- |
| `KindPrefix`: `poo-ledger`, `poo-peers`, `poo-node-policies` (and `poo-payment`, W-23) | Exports from older releases are refused as the wrong kind | Readers accept the old prefix too (a `brand.OldKindPrefixes` list); writers use the new one |
| `DataDir`: `~/.poo` | A node or wallet starts empty beside the old folder | If the new folder is missing and `~/.poo` exists, use it and say so (or offer `poo doctor` to move it) |
| `$POO_HOME`, `$POO_VIEW_ONLY` | Scripts and services stop finding the data | Accept the old names for at least one release, with a warning |

The hello `Agent` can follow the rename: it is shown, never matched.
Metric names can follow with a changelog line. No action now. White
checks it when the owner decides the name.

## What Grey should take into Round 36 Part A

1. Merge #291 (with #289 and #286) and Black's Part D PR.
2. **W-23** unsigned payment `kind` and `format`, unknown keys kept on
   write-back. Before the tag.
3. **W-24** wallet file keeps unknown top-level keys. Before the tag.
4. One stale comment that still says `"reserve"` (`pkg/cli/node.go:412`).
5. W-25 and W-26 are rules only. White has added W-25 to its checklist.

## Release readiness after this round

Nothing new blocks `v0.1.0-alpha.1` apart from W-23 and W-24, which are
small. Still open from WHITE-001: license (owner), at least one registry
node and its URL (owner), optional signing keys in `docs/allowed_signers`,
the repo going public (owner), and the two matching local release builds
at tag time (W-9). The owner's planned major change comes before the
final review round.
