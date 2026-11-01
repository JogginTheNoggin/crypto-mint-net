# Source Round 10: consensus review (Black, Part D)

## Summary

No change in this round adds or alters a validity, weight, selection or
byte-form rule, and no team proposed a consensus change, so **nothing is
passed to the owner for approval**.

Round 9's C-6 is met (D129). Kept progress now outlasts the sender's
backoff. A stall that took something in no longer doubles the backoff.
The grace period drops only a departed sender's parts file.

BLUE-011 is clear, and Blue was right to leave out Red's host-wide byte
budget. Such a budget would let cheap progress make an honest chase
forget its count again and again, which D115 forbids.

**One hard-fork case remains, B4, found by reading.** It is an open
case of the disk-reserve wait that RED-027 began to fix. A connected
sender can keep a large entry's parts file growing a little every
chase. It lists the honest heavier ledger's own hashes and layers, so
it has shown as much as any honest sender. Its parts file then ranks at
or above every honest one, so it is never reclaimed. Once the disk is
at its reserve, the honest fetch of that entry waits for good.
BLUE-011 narrows the case and should still be merged. The rest of the
fix is for Round 11 Part A.

ORANGE-020, GREEN-035 and the two GUI asks are clear. One more
condition: `TestSlowSenderIsReachedOverManyChases` guards C-6, so its
flakiness must be fixed, not skipped.

| Field | Value |
| --- | --- |
| Round | Source Round 10 |
| Consensus suggestions | None |
| Base reviewed | `claude/m10-watch` @ `0a6cc94` (code `72d6f25`; D129, D130) |
| Previous base | `claude/m10-watch` @ `1ae4d46` |
| Blue branches | `claude/project-thread-ggo8dt` @ `03e3205` (BLUE-011, D131) |
| Proposals | ORANGE-020 (`claude/source-orange-r10` @ `79d4160`); GREEN-035 (`claude/source-green` @ `aab3a07`); GUI asks: doctor `fix_args` for a lowered `layer_rate`, GUI-017 (LOCK_NB for offline schedule edits) |
| Result | 11 clear, 1 clear with conditions (the flaky guard test), 1 blocked: hard fork (B4, by reading) |

## Round 9's item

| Item | Now | Checked how |
| --- | --- | --- |
| C-6, kept progress against the backoff | Met (D129) | `ProgressTTL` runs from the later of the save and the end of the sender's backoff. `chase.Slowed` (took something in, then stalled) waits the first backoff step and restarts the doubling. `ProgressGrace` drops only the parts file. `TestSlowSenderIsReachedOverManyChases` and `TestProgressOutlastsTheBackoff` cover it |

## Blocked

### B4. A trickling sender can hold the disk reserve against an honest large entry (blocked: hard fork, by reading)

- **Where.** `reclaimParts` and `partsRank` (BLUE-011, D131), on top of
  RED-022's disk reserve (D123).
- **The order of reclaim.** A fetch waiting for disk takes room only from
  kept parts files that rank strictly below its own. The order is:
  1. a departed sender's;
  2. then an idle one, which asked for parts and took none;
  3. then less shown (`lessShown`);
  4. then fewer bytes.
- **The case.**
  1. A heavier valid ledger holds a large entry E. The attacker copies
     the ledger's hash listing and layers, which are public, and serves
     them. Its chase passes step 4 with the same work any honest
     sender's does.
  2. When the fetch reaches E, it answers with a large reply naming E's
     hash and a huge size. It sends E's real bytes, slowly. The first
     part starts with E's parent as required. Nothing is checked
     against E's hash until the item is whole, and it never will be.
  3. It sends something in every chase. So it is never idle, `Slowed`
     keeps its backoff at the first step, and its progress is saved
     every chase, so it never reaches its time to live.
  4. Its file grows until the parts disk is at its reserve.
  5. An honest sender's chase of E then waits for room. Its rank is
     equal on what it has shown and lower on bytes, so it cannot reclaim
     the attacker's file. The attacker's own chase can reclaim the
     honest file, because that one ranks below it.
  6. The honest node waits for good for the one entry it needs. That
     breaks D115.
- **Cost to the attacker.** It must first send as many bytes as the disk
  has free above the reserve, which can be hundreds of gigabytes. After
  that, holding the disk costs a trickle. It is expensive but possible
  against a chosen node, and nothing in the protocol bounds how long it
  lasts.
- **Before BLUE-011.** Any connected sender held its file, idle or not
  (RED-027). BLUE-011 removes the idle case, so it is a strict
  improvement and should be merged.
- **Fix shape, for Round 11 Part A (node policy, no rule changes).**
  Pick one, or combine:
  - **One file per entry.** Fetches of the same entry hash share one
    parts file. The bytes of an entry are the same from every honest
    sender, and the hash check at the end decides. An honest sender then
    continues the same file, not a second one. A size claim that
    disagrees with another sender's claim for the same hash means one
    of them lies. Keep the file whose bytes so far agree, and settle it
    by the hash check.
  - **Rank by recent rate, not total bytes.** Among chases that showed
    the same, the one that took in fewer bytes in its last slice ranks
    lower, so a trickler loses to an honest sender at full rate.
  - **Give back the reserve in turns.** A waiting fetch that has shown
    as much as the holder gets the room every other slice. Neither can
    hold the disk for good, and both still make progress.
- **Test to add to P0.** A sender that lists the honest heavier ledger
  and trickles a large entry it claims is huge, with the parts disk at
  its reserve (the `diskSpace` seam). The node must still reach the
  ledger from an honest sender.
- **For Red, Round 11 Part B.** Confirm the case on loopback.

## Clear with conditions

### The flaky guard test

`TestSlowSenderIsReachedOverManyChases` is the test for C-6, a D115
guard. It flakes under `-race` and load.
- Fix the cause, for example by giving its timeouts or slices room for
  `-race`, or by waiting on an event rather than the clock.
- Do not skip it, mark it short, or retry it until it passes. A guard
  that sometimes fails cannot tell a regression from noise.

## Clear

| Item | Why it is clear |
| --- | --- |
| D129: `chase.Slowed`, the time to live from the backoff's end, and the parts-only grace | Each only delays, and progress is kept until it can be used (C-6 met) |
| D130: ORANGE-019's `HashLimit` 4,096; `WeightlessPages` stays 64 | How many hashes a node asks for at once. The pause keeps its progress |
| D130: GREEN-034, and schedule `limits` in replies | The grinding schedule and its RPC and CLI only |
| BLUE-011: RED-026, counting by entry | An entry's layers are counted once, when its reply arrives. A reply item always carries every layer it has, or the entry is paged as many layers (C-3), so nothing is half counted. An honest listing never repeats an entry. Repeats within a reply are told apart by nonce. Re-reads in layer pages are those D124's order check already identifies, and they were counted when first read. Layers that arrive during paging and land before the read point are new work the check does not need. Weight is recomputed from what is fetched, as always |
| BLUE-011: compaction at the grace | A departed sender's progress is cut to its common entry and kept to the time to live. If it returns, it lists and counts again. Nothing is refused |
| BLUE-011: RED-027's reclaim order | Better than before: an idle or departed holder gives way. The case it leaves open is B4 |
| BLUE-011: no host-wide byte budget | Right under D115. Such a budget would let cheap worked progress repeatedly wipe an honest chase still counting |
| ORANGE-020 (`PageOwners`, `PreverifyPage`) | It only fills the verified-witness list ahead of time, on every core. `HandleEntryFrom` and `AddEntry` still check every entry in full. A preview that fails just stops early. ORANGE-014's C-4 conditions apply to the shared list as before |
| GREEN-035 | Grinding job slots, schedule restarts, `grind bench` and log wording. Nothing a node accepts |
| GUI ask: doctor `fix_args` when `layer_rate` is lowered | Advice in `poo doctor` |
| GUI-017: LOCK_NB lock for offline schedule edits | Local file locking for node.json |

## Items for the owner

- **No consensus change is proposed this round.** Nothing needs your
  approval.
- **B4 is a node-policy fix needed in Round 11.** It changes no rule.
  An attacker who first fills a node's spare disk can then keep it full
  with a trickle, and the node waits for good for a large entry of the
  heavier ledger. It is costly to set up but cheap to keep up.

How this was checked:
- The code changes from `1ae4d46` to `0a6cc94` were read, along with
  BLUE-011's diff, ORANGE-020's diff, the two proposals and decisions
  D129 to D131.
- B4 is from reading `reclaimParts`, `partsRank`, `fetchParts` and D129's
  `Slowed`, and was not reproduced.
