# Source Round 33: compatibility review (White, Part D)

## Summary

White's review of Grey's base `main` @ `3c4ae00` (941049d, #255: D186,
D187, D188; #256), Red's RED-097 to RED-099 (#257), GREEN-058 (#259),
ORANGE-043 (#265) and BLUE-034 (#266, D189, which carries #259 and #265).
**Nothing is breaking and nothing blocks the round.** D186 (version and
features in the hello) does what the owner chose. **Before the first tag,
widen two of its limits (W-14)**, because after the tag they freeze for
good, and a post-quantum key or signature would not fit in 1 KiB.
D188's ledger file is sound for compatibility: it has a kind, a format,
and a reader that accepts the fields in any order. Its kind should be
renamed `poo-ledger` before the tag, to match the other files (W-15). The
owner has since moved the default ports again, to 7732 and 7733 (W-16,
R34A). No consensus question arose. Nothing needs the owner.

## Verdicts

| Item | Verdict | Notes |
| --- | --- | --- |
| D186: release and features in the hello; extensions on hello, ping and pong; `p2p.announce_version` | compatible with conditions | Matches WHITE-002 as revised. Checked in the code: `Agent` stays `poo`, the dial-back probe keeps its own hello, the release is checked against `0-9 A-Z a-z . - +` on decode, features are sorted and unique, unknown extension tags are kept and nothing reads them, and nothing after the lists is accepted. `docs/protocol/wire.md` documents it. **Condition:** W-14 before the first tag |
| D186 vs builds from before it (RED-099) | compatible with conditions | They cannot connect, both ways. Acceptable: no release exists (the owner's choice, and Black's terms say to state it). BLUE-034 adds `wire.ErrOldHello` and a changelog note. `Protocol` stays 1, which is right: this is the first layout any release will speak, and raising it would not help an old build, which already accepts any value |
| D187: ports 7336 and 7337, written by `poo init`; `0.1.0-alpha.1-dev`; pre-release tags | compatible with conditions | A `node.json` with an empty `listen` moves to the new default on upgrade; the changelog says so. `release.yml` adds `--prerelease` for a tag with `-` (WHITE-001 R12, done). **Condition:** superseded by W-16, the owner's move to 7732 and 7733 |
| D188: ledger export and import, every item fully checked | compatible with conditions | The file has `kind` and `format` 1, is refused when either differs, and ignores fields it does not know, so a later release can add one without a format bump. The items are `wire.EncodeItem` bytes (the frozen entry byte form plus layer nonces), so the file is as stable as wire v1. **Condition:** W-15 |
| GREEN-058 (#259): export refuses `-from` past the tip; import says what it needs | compatible | Messages and a check only |
| ORANGE-043 (#265): import streamed | compatible | `readLedgerFile` streams when `kind`, `format`, `series` and `pool` come before `entries` (as `poo ledger export` writes them) and otherwise reads the whole file as before, so any valid format 1 file still imports. Checked on #266's head |
| BLUE-034 (#266, D189): exports end where their header says; import calls bounded (256 items, 8 MiB past the first); `-dry-run`; partial totals on a stop; `ErrOldHello` | compatible | The call bound only limits what one RPC call carries. The command batches to fit it, and a single large item still goes through. No file or wire change beyond the named error |
| RED-097 to RED-099 (#257) | n/a (findings) | White agrees with all three. BLUE-034 fixes them |

## New suggestions for Grey

| Id | What | Why | When |
| --- | --- | --- | --- |
| W-14 | Widen two D186 limits before the first tag: `MaxExtValue` from 1 KiB to 64 KiB, and `MaxFeatures` from 64 to 255 (the most a `U8` count can say). Keep `MaxExt` 32 and the rest. Optionally cap the whole hello, for example at 256 KiB | Every limit a decoder checks is frozen at the tag: an older node drops a hello, ping or pong that goes over it, whatever the extension means. A later extension is likely to carry a key or a signature, and this project's own keys are post-quantum: an ML-DSA-65 public key is 1,952 bytes and its signature 3,309, and SLH-DSA signatures run to tens of KiB. A 1 KiB value could never carry one. Raising the limits costs nothing in memory: the hello is already read under the 16 MiB frame limit (`wire.ReadFrame(r, wire.MaxFrame)`). Red may want to look at the hello cap | R34A, before the tag |
| W-15 | Rename the ledger file's kind from `poo ledger` to `poo-ledger` | The other files are `poo-node-policies` and `poo-peers`. A kind is frozen once a release writes it, so the tag is the last chance. Only pre-release builds have written `poo ledger`; reading both for a while is optional | R34A, before the tag |
| W-16 | Default ports 7732 (local commands) and 7733 (peers), replacing 7336 and 7337 (the owner, 2026-11-05 21:36) | Both are IANA-unassigned. `poo init` already writes the port into `node.json` (D187), so nodes made with 7336/7337 keep them. Only an empty `listen` moves, and the changelog says so as it did for D187 | R34A |
| W-17 | Document in `docs/protocol/wire.md` that every decoder limit on the hello, ping and pong (and the release character set) is part of wire version 1 and never changes after the tag; new needs go in a new extension tag or a new message type | So a later team does not raise or lower one and cut off released nodes | With W-14 |

## Release status (WHITE-001)

| Item | State |
| --- | --- |
| R1 license | Open, the owner's call ("no license just yet") |
| R2 ports | D187 built 7336/7337; the owner moved them to 7732/7733 (W-16, R34A) |
| R3 first registry | On the owner's website; waiting for its URL |
| R4 W-1, R5 W-10 | Done (D184, D185) |
| R6 version in the hello | Done (D186), with W-14 before the tag |
| R7 local release run | GUI/White ran the GUI set on `d23b888`. The Source run is done at tag time |
| R8 signed checksums | Open (the owner holds the key) |
| R9 README status, SECURITY contact | Open |
| R10 changelog | Grey keeps it per round; finalised at tag time |
| R11 `poo-gui` install path docs | Open (GUI Grey) |
| R12 pre-release flag | Done (D187) |

No *release blocker* came from this round. W-14 and W-15 must be done
before the tag, because both freeze there.
