# Source Round 8: consensus review (Black, Part D)

## Summary

No change in this round adds or alters a validity, weight, selection or
byte-form rule, and no team proposed a consensus change, so **nothing is
passed to the owner for approval**.

Round 7's B1 is fixed. Its test (`TestHeldBranchWithoutLayers`) now
passes, and so do the new tests for C-1 to C-3. Conditions C-2 and C-4
are met, and the G4 gap is closed.

Two hard-fork cases remain on the new base. Both must be fixed in Round
9 Part A:

- **B2, measured by Orange (ORANGE-018).** Under the default request
  budgets, a long heavier branch is never reached. The chase overruns
  the sender's budget, times out, drops its progress and starts again,
  every time. ORANGE-018 fixes this for the default budget. A timeout
  must also keep the chase's progress, so a sender with a lower budget,
  or a slow one, only delays the chase.
- **B3, by reading.** Paging one entry's many layers (`layersFrom`, from
  C-3) never checks the time slice or the pace. A sender can keep a
  chase paging there forever and hold its chase slot, so honest chases
  for that pool never run.

BLUE-009 is clear on one condition, C-5. Its early stop in step 4 (from
RED-021) skips the rest of a reply without counting it. That brings back
Round 7's C-1 in the most common shape, a branch that fits in one page.
GREEN-033, ORANGE-017 and the queued GUI-014 source fix are clear.

| Field | Value |
| --- | --- |
| Round | Source Round 8 |
| Consensus suggestions | None |
| Base reviewed | `claude/m10-watch` @ `ae8a103` (code `afd779f`; D121, D122) |
| Previous base | `claude/m10-watch` @ `0fd59fa` |
| Blue branches | `claude/project-thread-ggo8dt` @ `7608fdb` (BLUE-009, D123) |
| Proposals | GREEN-033 (`claude/source-green` @ `512026c`); ORANGE-017, ORANGE-018 (`claude/source-orange-r8` @ `d328a01`); GUI-014 source fix (queued, no code yet) |
| Result | 13 clear, 2 clear with conditions, 2 blocked: hard fork (B2, B3) |

## Round 7's items

| Item | Now | Checked how |
| --- | --- | --- |
| B1, held branch without its layers | Fixed (D121) | `findCommon` walks back through held entries to the selected ledger, with no request and no limit. Ancestors of a held entry are always held, since pruning removes leaves first. `topUpLayers` now asks about the sender's hashes. `TestHeldBranchWithoutLayers`, `TestDeepLayersAreToppedUp` and Black's appendix case pass on `7608fdb` |
| C-1, recheck ended a heavier chase | Fixed on the base. BLUE-009 brings it back (C-5 below) | `fetchBranch` goes back to step 4. Only a branch counted to its end can end as not heavier |
| C-2, progress forgotten | Met | A worked chase is never forgotten for one that has not passed its work check. Then the less work, then the oldest, is forgotten first |
| C-3, two million layers per entry | Met, except B3 | Wire types 27, 52 and 53 are allowed by D76. Paging strongest first misses no layer: a layer arriving meanwhile can only push ones already read later, so at worst one is re-read |
| C-4, ORANGE-014's set key | Met | The key's coverage is recorded at `setKey`, and the equivalence test is kept |
| G4, the listed hash | Closed | Nothing is bad-listed when an entry does not match its listing. Only the sender is flagged |

## Blocked

### B2. Long chases overrun the sender's request budget and start over forever (blocked: hard fork)

- **Where.** `pkg/p2p/chase.go`, every chase loop. The sender's
  `tracking` request budget is 600 a minute by default.
- **What happens.** Orange measured this on `ae8a103` with the defaults:
  1. Checking a long run takes two requests per 256 hashes.
  2. Past 600 requests in a minute, the sender drops requests
     unanswered.
  3. The chase waits out `RequestTimeout`, ends as Nowhere and drops its
     progress.
  4. The next chase starts again from the common entry and fails at the
     same point.
- **Cases that are never reached:**
  - a heavier branch ending in about 65,000 or more entries without
    layers;
  - 200,000 such entries that the node already holds without layers;
  - layers more than about 75,000 entries below the tip (the B1
    top-up).
- **The rule it breaks.** D115. Two default settings, the sender's
  budget and the asker's chase, together keep a heavier valid ledger
  out of reach for good.
- **Fix (node policy), in Round 9 Part A:**
  - **Adopt ORANGE-018.** The asker counts its requests to each sender
    per minute and yields with its progress kept at `RequestPace` (480).
    Black clears it: it decides only how fast a node asks.
  - **A timeout yields too, keeping its progress** (Orange's own open
    point). The sender is backed off as now, but a later chase goes on
    from where this one stopped. Without this, a sender with a budget
    below 480 or a slow link still causes B2.
  - **Testing.** Add the 100,000-entry bare run with default budgets,
    and one with a sender budget below the pace, to the P0 tests (they
    can run in CI's long step).

### B3. Paging one entry's layers never yields (blocked: hard fork, by reading)

- **Where.** `layersFrom` in `pkg/p2p/chase.go`, on `ae8a103` and on
  BLUE-009 alike. It is called from `askLayers` in both step 4 and the
  top-up.
- **What happens.**
  - The loop pages while the sender says "more" and sends at least one
    nonce. It never checks the slice deadline or the pace, and it keeps
    no offset in the progress.
  - In step 4, a repeated layer adds no work (`st.counted`), so a sender
    can resend the same valid layers, which are cheap to copy, with
    "more" set, forever. The top-up behaves the same way.
- **Effect.**
  - The chase never yields, so its slot is held for as long as the
    sender wants (`Chases` 4, `PerPool` 2, `Unlisted` 1).
  - Two such senders in one pool keep every chase of that pool from
    running, including the chase of an honest heavier ledger. That
    lasts as long as they keep it up.
  - Before BLUE-009 this also grew memory without limit, which was
    RED-021. BLUE-009 fixed the memory, not the loop.
- **Fix (node policy), in Round 9 Part A:**
  - Check the deadline and the pace before each layers-from page. Save
    the entry and the offset in the progress, yield, and go on from
    there.
  - Check the strongest-first order across pages with O(1) memory: no
    layer may be stronger than the last one read. Equal strength must
    rise in nonce, except for one re-read at a page start. A sender
    that breaks the order is false.
  - Red should try the repeating sender in Round 9 Part B.

## Clear with conditions

### C-5. BLUE-009 (RED-021): the early stop must not skip what it did not count

- **What it does.** In step 4, `askLayers` now stops counting once the
  work beats the node's. That part is fine. It still counts every
  remaining entry of the reply as answered, and `checkWork` moves
  `pending` past them, so their layers are never counted. Likewise, an
  entry whose many layers are paged counts as answered after the first
  page that is enough.
- **Why it matters.** When the node's own ledger outgrows the counted
  work during the fetch, C-1 goes back to step 4 to count more. The
  skipped items are gone by then. If the listing is complete, the chase
  ends as "provisional work does not beat" while the uncounted rest of
  the branch is heavier.
  - The commonest case is a short fork whose whole branch fits in one
    hash page. Step 4 stops at the entry that just beats and skips the
    rest of the page.
  - The node then retries from scratch after the sender's backoff, so
    this is the narrow C-1 case again: a chase on a growing minority
    side can repeat it without end.
- **Not reproduced.** It was found by reading, and the code path is
  plain.
- **Condition for merging BLUE-009.** Move `pending` only past the
  entries actually counted. For an entry stopped in the middle of its
  many layers, keep the entry and its offset, so a return to step 4
  goes on from there. B3 needs the same offset, so one change covers
  both. Add a test: a one-page branch, and the node's own ledger
  growing between steps 4 and 5.

### RED-022's disk reserve (BLUE-009)

Clear, with one condition. Waiting for disk space never refuses an
entry, and an entry larger than a node's disk is a physical limit, not a
coded one. The condition: the wait must be visible to the operator,
beyond a chase log line, for example in `poon node` status or a warning
event. A heavier ledger waiting on disk space should not look like a
healthy node.

## Clear

| Item | Why it is clear |
| --- | --- |
| D121: `findCommon` walk-back and the new `topUpLayers` | Local walk only. The top-up pages back from the sender's tip until the node is as heavy as the sender stated, with no depth limit, and it yields |
| D121: C-1 hand-back to step 4 | Ends as not heavier only after counting the whole listing (subject to C-5 on BLUE-009) |
| D121: C-2 eviction order | A worked chase is never evicted for one that has not passed its work check |
| D121: C-3 wire types 27, 52, 53 | New types, as D76 allows. Byte forms are unchanged. B3 is about pacing, not about the types |
| D121: `tracking.Spend` counts only what it lets through | A steady honest sender gets the budget's rate. This closes ORANGE-016's starved sender |
| D121: 32-bit `Reader.Count` guard | Decoding only. A count past `int` cannot be held anyway |
| D122: GREEN-032, ORANGE-014, 015, 016 | Cleared in Round 7. ORANGE-014's conditions are met |
| BLUE-009: RED-020 | How replies sort and cache. The order key (the entry and its layer count) changes whenever a layer is added, since layers are never removed one at a time |
| BLUE-009: RED-021 counting as layers arrive | Counting as it arrives is fine. Only C-5 needs fixing |
| GREEN-033 | Parts move to `<data dir>/parts`, 0700, with leftovers removed on start. Where bytes are written, not what is accepted. With RED-022, the reserve now guards the store's own disk, which is what it is for |
| ORANGE-017 | `Size` runs the same code as `Encode`, so the bytes are identical (the `pkg/vectors` and round-trip tests pass). `onKept` checks the size before encoding, with the same threshold as before |
| ORANGE-018 | It decides how fast a node asks a sender, never what it accepts. It is half of B2's fix |
| GUI-014's source fix (queued) | Server-side schedule operations under `policyMu` or If-Match, and a whole-job remove guard. Grinding schedule only, nothing that decides validity or selection. Black will look at the code when it lands |

## Items for the owner

- **No consensus change is proposed this round.** Nothing needs your
  approval.
- **B2 and B3 are node-policy fixes needed before Round 9 Part B.**
  Neither changes a rule. B2 is Orange's measured case: with today's
  defaults, a long heavier branch is never reached. B3 lets a cheap
  sender hold a chase slot indefinitely.

How this was checked:
- Every code change between `0fd59fa` and `ae8a103` in `pkg/p2p`,
  `pkg/tracking`, `pkg/encoding`, `pkg/policy`, `pkg/tx` and `pkg/wire`
  was read.
- BLUE-009's diff and the diffs of the three proposals were read.
- Decisions D121 to D123 were read.
- `TestHeavierLedgerAlwaysReached`, `TestHeldBranchWithoutLayers`,
  `TestDeepLayersAreToppedUp` and `TestManyLayersOnOneEntryAreCounted`
  were run on `7608fdb`, and they pass.
- B2's numbers are Orange's. B3 and C-5 are from reading and were not
  reproduced.
