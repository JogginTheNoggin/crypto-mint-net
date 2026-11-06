# Source Round 34: compatibility review (White, Part D)

## Summary

White's review of Grey's base `main` @ `f1ac406` (#272, D190), Red's
RED-100 to RED-103 (#275), ORANGE-044 (#276), GREEN-059 (#277) and
BLUE-035 (#280, D191, which carries #277). **Nothing is breaking and
nothing blocks the round.** D190 did what Round 33 asked: ports 7732 and
7733, wider hello limits, the `poon-ledger` kind, and the limits written
down as frozen. White answers the two questions left open for it:

- **Ping and pong size (Blue's question, RED-103).** Keep the hello at
  the D190 limits, but cap the extensions of a **ping or pong at 64 KiB
  in total**, before the tag (W-18). A ping has no reason to carry more
  than one key or signature, and one SLH-DSA-256s signature (29,792
  bytes) fits. This is a frozen decoder limit, so it has to be decided
  now. Bigger data later goes in the hello or in a new message type,
  which older nodes ignore.
- **Hello frame (ORANGE-044 Proposal 1).** BLUE-035's 3 MiB hello frame
  is compatible: the largest hello the frozen limits allow is about
  2.1 MiB. Like the limits, the 3 MiB bound freezes at the tag (W-19).

One condition on BLUE-035: the ledger file reader now **refuses** header
keys it does not know. White asks that it **skip** them instead, within
the same 4 KiB limit, so a later release can add a header field without a
new format (W-20). The release signature (Red's note, WHITE-001 R8) belongs to
the project, not one person: maintainer keys listed in the repo, any one of
which signs (W-21). No consensus question arose. Nothing needs the
owner.

## Verdicts

| Item | Verdict | Notes |
| --- | --- | --- |
| D190: ports 7732 (local commands) and 7733 (peers) | compatible | `poon init` writes them (D187); only an empty `listen` moves, and the changelog says so |
| D190: `MaxExtValue` 64 KiB, `MaxFeatures` 255, frozen with wire v1; hello extensions not kept | compatible with conditions | W-14 done. **Condition:** W-18 for ping and pong before the tag |
| D190: ledger kind `poon-ledger` | compatible | W-15 done. Only pre-release builds wrote `poon ledger`. Source commit 0cc9428 changed `gui/` in the same step; GUI/White checked it and noted it only for the layer rule |
| RED-100 to RED-103 (#275) | n/a (findings) | White agrees. RED-103's second fix is answered by W-18 |
| ORANGE-044 (#276) | Proposal 1: compatible | Counting connections still in their hello against `MaxInbound`, and reading the hello under a bound near its largest size, change no layout. White's answer to "might a later release need a larger hello": no larger than the frozen limits allow, since a new field can only be an extension. So a bound just above the largest valid hello is safe (W-19). Proposals 2 to 4 change no file or message |
| GREEN-059 (#277): a dev build names its commit; the dry run says whether the pool is followed | compatible | A dev build's snapshot build id now includes its commit, so its first start after this change replays once. That's no change for release builds, which set `Commit` by `-X` |
| BLUE-035 (#280, D191): allocation follows bytes; 3 MiB hello frame; handshakes count against `MaxInbound`; inbox bounded by bytes (32 MiB a queue); import reads bounded; ledger header keys bounded | compatible with conditions | The inbox takes a message over its byte bound when the queue is empty, so no item is refused for its size alone. **Condition:** W-20 |

## Suggestions for Grey

| Id | What | Why | When |
| --- | --- | --- | --- |
| W-18 | Cap a ping's and a pong's extensions at **64 KiB in total** (the sum of the value lengths), as a decoder limit frozen with wire v1. Leave the hello at 32 × 64 KiB. Say so in `docs/protocol/wire.md`. Keep D190's test of the largest hello; change the 2 MiB ping test to the new cap | A ping is sent often and by every peer. Its extensions may one day carry one key or signature (SLH-DSA-256s, the largest, is 29,792 bytes), never 32 of them. 2 MiB pings are the cost RED-103 measured, and a node-policy frame limit cannot fix it later, because an older node would drop the larger ping. Decided now, it costs nothing | R35A, before the tag |
| W-19 | Treat `wire.MaxHelloFrame` (3 MiB) as frozen with wire v1, and list it next to the other limits in `wire.md` | An older node reads a hello under it forever, so a later hello must stay under it. It is above the largest hello the frozen limits allow, so this only writes down what is already true | With W-18 |
| W-20 | `readLedgerFile`: **skip** a header key format 1 does not define, reading it under the same 4 KiB bound and keeping nothing, instead of refusing the file. Keep refusing a repeated key, and a known key over the bound | Refusing means a later release can never add a header field (a note, a range, a checksum) without a new format, which older releases then refuse whole. Skipping bounds memory just as well (RED-102's concern), and the item checks are unchanged | R35A, before the tag (the reader's behaviour freezes with `format` 1) |
| W-21 | Signed checksums that belong to the **project, not one person** (WHITE-001 R8, Red's note; the owner, 2026-11-05: others may make releases later). **Recommended: maintainer keys listed in the repo.** `docs/allowed_signers` lists each maintainer's SSH public key, one line each, under the shared name `poon-release`, for example `poon-release valid-after="20261105" ssh-ed25519 AAAA... alice`. Any one maintainer signs a release: `ssh-keygen -Y sign -f ~/.ssh/their_key -n poon-release SHA256SUMS`, which writes `SHA256SUMS.sig`, uploaded beside it. Anyone checks with `ssh-keygen -Y verify -f allowed_signers -I poon-release -n poon-release -s SHA256SUMS.sig < SHA256SUMS`, then `sha256sum -c SHA256SUMS`. A maintainer is added by a reviewed change to `allowed_signers`, and retired by giving their line `valid-before="DATE"`, so their old signatures still check and new ones are refused. Each maintainer keeps their own key; none is shared, and none goes in the repo or a CI secret. Add the steps to `docs/releasing.md` ("Signing" and "Checking a release"). **Later option: keyless signing** (Sigstore `cosign sign-blob` from the release workflow, tied to the repository rather than a person). It needs a working Actions runner and trusts GitHub's identity service, so White suggests it only once CI runs, alongside the maintainer keys. **Optional for the alpha:** reproducible builds already let anyone check an archive against the source | No single person owns the signing. The list in the repo says who may sign, and it changes by the project's normal review. It needs only OpenSSH, which every platform has, and adds no code or dependency. One file covers the `poon` and `poon-gui` archives (GUI/White) | Docs in R35A; keys when maintainers are ready; optional before the alpha |

## Release status (WHITE-001)

| Item | State |
| --- | --- |
| R1 license | Open, the owner's call |
| R2 ports | Done: 7732/7733 (D190) |
| R3 first registry | On the owner's website; waiting for its URL |
| R4, R5, R6, R12 | Done (D184, D185, D186, D187) |
| R7 local release run | Tag-time. GUI part passes (GUI/White) |
| R8 signed checksums | Optional for the alpha. W-21: maintainer keys listed in the repo, any one signs; Sigstore later once CI runs |
| R9 README status, SECURITY contact | Open |
| R10 changelog | Tag-time |
| R11 `poon-gui` install docs | Done (#269) |

Before the tag, W-18 and W-20 must be done, because both freeze there.
