# Source Round 32: compatibility review (White, Part D)

## Summary

White's first round review. It reads Grey's base `main` @ `6260590`
(D184), Blue's BLUE-033 (#251, D185, with ORANGE-042 and GREEN-057
carried), Red's RED-093 to RED-096 (#246), GREEN-057 (#247) and ORANGE-042
(#249), plus WHITE-002 step 1, which Grey deferred to White and Black.
**Nothing is breaking and nothing blocks the round.** Everything is
*compatible* or *compatible with conditions*. The conditions are: merge
BLUE-033 together with D184; put the list-entry gap and the keeping limits
in the release notes; and add one rule to White's checklist (new settings
go in objects, not inside list entries). WHITE-002 step 1 is *compatible
with conditions* and can be built in R33A once Black clears it. No
consensus question arose. Nothing needs the owner.

## Verdicts

| Item | Verdict | Notes |
| --- | --- | --- |
| D184, W-1: node.json keeps settings it does not know (main) | compatible with conditions | Does what W-1 asked for: unknown keys in objects survive Load and Save, and the start warns. Red found four ways it fell short (RED-093 to 096). **Condition:** merge BLUE-033 in R33A so they ship together; no release should have D184 without it |
| D184, W-10: `release` in `GET /v1/status` and `poon status` (main) | compatible | Added field only. Checked: `pkg/rpc` client decodes replies without `DisallowUnknownFields`, so an older `poon` reading a newer node, or the other way round, still works |
| D184, ORANGE-023: unused snapshots shown as a count (main) | compatible with conditions | A JSON field changed shape (a list became a count). Nothing outside `poon` reads it except the GUI, which ships in the same binary. **Condition:** a line in the changelog for anyone who scripts `poon status -o json` |
| BLUE-032 (merged in R32A): the dial-back hello carries `poon-self-check/TOKEN` in `Agent` | compatible | Older peers take any agent of up to 256 bytes. It matters for WHITE-002 step 1 below |
| BLUE-033 (#251, D185): folded keys are the setting; kept settings limited (256, 64 KiB each, 64 deep); compact writing; status caches node.json; imports keep them | compatible with conditions | Fixes RED-093 to 096. Tests pass here: `go test ./pkg/nodepolicy ./pkg/rpc ./pkg/series ./pkg/cli` on `378d4cb`. A folded key (`ALLOW_REMOTE`) is now saved under its real name: same meaning, so it is compatible. **Conditions:** (1) the release notes say a rollback-save loses a setting inside a list entry, or one over the limits, and that the start warning and `poon doctor` name it; (2) see W-12 on the 64 KiB per-value limit |
| ORANGE-042 (#249): one-pass save of kept settings | compatible | Carried in BLUE-033. Output is the same settings, compacted |
| GREEN-057 (#247): doctor names unknown and dropped settings, and a mismatch between the `poon` binary and the running node's release | compatible | Carried in BLUE-033. White welcomes it: it closes the "silent loss" gap White's README listed only as a rule. The release check is what W-10 was for |
| GUI ORANGE-024 (carried in BLUE-033): variable series set grown in place (`pkg/series`) | compatible | In-memory only; no file or message changes |
| RED-093 to RED-096 (#246) | n/a (findings) | All four are about W-1. White agrees with each; BLUE-033 fixes them |
| WHITE-002 step 1: `Agent` becomes `poon/VERSION` | compatible with conditions | See below. Ready for R33A once Black clears it |

## WHITE-002 step 1, reviewed with the code

> **Superseded (2026-11-05, after this review).** The owner asked to put
> the version in the protocol itself before the first release, and chose
> "hello only, both hello and ping extensible". WHITE-002 now has a
> `Release` field and a features list in the hello, and an extensions list
> at the end of hello, ping and pong; `Agent` stays `poon`. Conditions 2
> to 5 below still apply to the new `Release` field. Condition 1 holds as
> the dial-back hello not carrying the release.

What older nodes do: `DecodeHello` reads `Agent` as up to 256 bytes of
any text, and nothing in `pkg/p2p` or `gui/` compares it to `"poon"`
(checked with `grep`). So changing the text changes nothing for an older
peer. The wire and hello layout stay as they are.

Conditions for Grey when building it:

1. **Two agent forms, kept apart.** A normal hello sends `poon/VERSION`.
   The dial-back hello keeps `poon-self-check/TOKEN` (BLUE-032) and never
   carries the version. The prefixes differ (`poon/` and `poon-`), so
   `sawSelfProbe` stays as it is.
2. **Parsing.** `poon peers -o json` gets `version` only when the agent is
   `poon/` followed by 1 to 64 bytes of `0-9 A-Z a-z . - +`. Anything else
   (a hidden release, another program, a probe) leaves `version` out:
   never a guess (agreed with GUI/White).
3. **Text from peers.** The agent stays peer-chosen text. `poon peers`
   table output strips control and invisible characters from it, using the
   same check as `nodepolicy` (`plainText`). Red may want to check this.
4. **Opt-out.** `p2p.announce_version` (`release` by default, or `name`
   to send just `poon`) is machine-local, so a shared settings file never
   changes it, and an import plan lists it like other machine-local
   settings.
5. **Never a gate.** Nothing may choose, rank, refuse or stop asking a
   peer because of its agent. For Black: it is a claim shown to the
   owner, and nothing else reads it.

## New gaps and rules (suggestions for Grey)

| Id | Gap | Suggestion | When |
| --- | --- | --- | --- |
| W-11 | A setting a newer release adds **inside a list entry** (a series, a follow rule, a grind job) is dropped when an older release saves. D184 says so, GREEN-057 now names it, but it remains a loss | Rule for every team, added to White's checklist: a new setting goes in an object (a section, or a map keyed by name), never as a new field in a list entry. If one must go in a list entry, the change needs White's verdict and a release note. No code needed now | Rule now |
| W-12 | BLUE-033 keeps each unknown value only up to 64 KiB. A later release could add a setting that holds a list (peer addresses, jobs) and goes over that, and a rollback-save would drop it. Once kept values are written compacted, a large value no longer grows the file out of proportion (RED-094's cause was depth with indenting) | Optional: replace the per-value limit with a total for all kept settings (for example 1 MiB), keeping the depth and count limits. Grey's call; fine to merge BLUE-033 as it is | R33A or later |
| W-13 | The start warning and doctor list the first ten kept or dropped settings (`NameSome`) | Fine. Note for the release notes: `poon doctor -o json` should list them all, so a runner can see every one | When convenient |

## Release status

Open items before the first alpha, from WHITE-001: R1 license, R2 new
ports, R3 a first public registry and R8 signing, all waiting on the
owner's four questions. R4 (W-1) and R5 (W-10) are done in D184, once
BLUE-033 lands. R6 is WHITE-002 step 1, ready for R33A. R12 (pre-release
flag in `release.yml`) is not done. No *release blocker* came from this
round's changes.
