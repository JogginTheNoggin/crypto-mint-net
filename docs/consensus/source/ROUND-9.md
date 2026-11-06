# Source Round 9: consensus review (Black, Part D)

## Summary

No change in this round adds or alters a validity, weight, selection or
byte-form rule, and no team proposed a consensus change, so **nothing is
passed to the owner for approval**.

Round 8's blocked items are fixed on the new base:
- **B2.** Pacing is adopted, and a timeout now stalls the chase, keeping
  its progress.
- **B3.** Paging an entry's layers yields per page, and the order is
  checked.
- **C-5.** Step 4 moves past counted entries only.
- **RED-022's condition.** The disk wait is visible in `poon chases`,
  `poon doctor` and a metric.

Nothing is blocked this round.

BLUE-010 is clear on one condition, C-6. As it stands, its time limit on
kept progress (`ProgressTTL`, 1 hour) meets the stall backoff, which
doubles up to an hour (`MaxBackoff`). A chase that stalls every time,
which is what a slow link does to a long fetch, loses its progress just
when it would go on. That brings back B2 for slow senders.

GREEN-034 and ORANGE-019 are clear. ORANGE-019's open question is
whether D120's pause counts pages or hashes. Either answer only paces,
so it is not a consensus matter, and Black prefers keeping pages.

| Field | Value |
| --- | --- |
| Round | Source Round 9 |
| Consensus suggestions | None |
| Base reviewed | `claude/m10-watch` @ `1ae4d46` (code `86d3611`; D124 to D127) |
| Previous base | `claude/m10-watch` @ `ae8a103` |
| Blue branches | `claude/project-thread-ggo8dt` @ `0e58e57` (BLUE-010, D128) |
| Proposals | ORANGE-019 (`claude/source-orange-r9` @ `2f9ab47`); GREEN-034 (`claude/source-green` @ `cbe2262`) |
| Result | 12 clear, 1 clear with conditions (C-6), 0 blocked |

## Round 8's items

| Item | Now | Checked how |
| --- | --- | --- |
| B2, long chases restart for ever | Fixed (D124) | ORANGE-018's pace (480 a minute) is adopted. A timeout ends the chase as `Stalled`, which backs the sender off but keeps it in the group and keeps the progress (`Outcome.Keeps`). `TestHeavierLedgerLongBareRun` (100,000 bare entries, default budgets) and `TestChaseBelowSenderBudgetIsReached` cover it. See C-6 for how BLUE-010 interacts with it |
| B3, `layersFrom` never yields | Fixed | It checks the slice and the pace before each page after the first, and saves the entry and offset in the progress. Every honest sequence passes the order check: within a page, stronger first and lower nonce among equals. Re-reads at a page start come only from layers that arrived meanwhile, so there are never more of them than the total grew by. A shrinking total, or a page past the total, can only come from a liar, since layers are never removed one at a time and an entry that leaves the selected ledger is answered "not held". A false sender is flagged and nothing is bad-listed |
| C-5, early stop skipped uncounted entries | Fixed | `askLayers` reports how many hashes, from the first, it counted whole, and step 4 moves only past those. A stopped entry keeps its offset |
| RED-022's disk wait must be visible | Met | A warning log, `waiting` and `waiting_since` in `poon chases`, a `poon doctor` warning and the `poon_chases_waiting_disk` metric |

## Clear with conditions

### C-6. BLUE-010 (RED-023): kept progress must outlast the stall backoff

- **What BLUE-010 does.** Kept progress, its parts file included, is
  forgotten once its sender has been away for `ProgressGrace` (2
  minutes), or once no chase has taken it up for `ProgressTTL` (1
  hour).
- **The interaction.** D124 backs a stalled sender off as after
  Nowhere: 30 s, doubling with each failure in a row, up to `MaxBackoff`
  of 1 hour. Nothing but a switch resets the doubling, not even a chase
  that made progress before it stalled.
  - Picture a sender on a slow link. Every chase moves the fetch on and
    then times out on a long reply. `ask`'s timer covers the whole
    reply, so a 64 MiB parts reply needs more than about 2 MB/s.
  - After seven stalls the backoff is an hour, the same as
    `ProgressTTL`.
  - The sweep, which runs every `CheckEvery`, then forgets the progress
    at the moment the sender is due again. The next chase starts from
    nothing, gets about one chase's worth further, and loses it again an
    hour later.
  - A branch or large entry that needs more than one chase's worth of
    transfer is then never reached from that sender. That is B2 again,
    for slow senders.
  - Before BLUE-010 the same case was only slow, because the progress
    was kept until used.
- **Conditions for merging BLUE-010:**
  - Progress kept by a stall must last at least until its sender's
    backoff ends, plus a margin. For example, measure `ProgressTTL` from
    the end of the sender's backoff, or keep it above `MaxBackoff`.
  - A chase that took something in before it stalled (entries, layers,
    part bytes or counted work) should not double the backoff. Back it
    off by the first step, as a yield does. Otherwise a steady slow
    sender ends up at an hour between slices for nothing.
  - **`ProgressGrace`.** Dropping the parts file of a sender that left is
    fine, and BLUE-010 already does it whenever disk is short. Dropping
    the rest of a worked chase's progress after 2 minutes away costs
    little memory to keep. Prefer to drop only the parts file at the
    grace, and the rest at the time to live.
  - **Test.** A sender whose replies always time out partway, with a
    branch that needs more than eight chases, is reached in the end.
- **Why only a condition.** Forgetting progress is node policy and only
  delays, provided a chase can still move forward between losses. The
  conditions keep that true. Black found this by reading and did not
  reproduce it.

RED-024 (parts kept on any stall) and RED-025 (a `layer_rate` over the
maximum is lowered at load) are clear.

## Clear

| Item | Why it is clear |
| --- | --- |
| D124: `chase.Stalled` | Keeps the sender in the group and keeps the progress. It delays, and refuses nothing (subject to C-6 once BLUE-010 is merged) |
| D124: pace (`RequestPace`, `RequestPaceWindow`) | How fast a node asks, never what it accepts |
| D124: the `layersFrom` order check | Only a sender that breaks the strongest-first order is false. An honest sender cannot break it, as shown above |
| D125: grind schedule edits (GUI-014) | The node's own grinding schedule and its RPC. Nothing about validity or selection |
| D126: ORANGE-017, GREEN-033 | Cleared in Round 8. The bytes are unchanged and `pkg/vectors` passes |
| D127: `MaxLayerRate` 50 | It bounds only the extra layers a node's own grinder sends. What it accepts and fetches is unchanged, and a chase still fetches every layer it needs (C-3) |
| BLUE-010: RED-023's disk sweep (`dropAwayParts`) | It frees disk held for departed senders before waiting. The rest of their progress stays (the grace and time to live are C-6) |
| BLUE-010: RED-024 | Parts are kept whenever the progress is kept, so a stall resumes from its offset |
| BLUE-010: RED-025 | Load lowers an out-of-range `layer_rate` to the maximum and warns. Node policy only |
| ORANGE-019: asking for 4,096 hashes a page | The same size peers already serve. It changes only how many hashes a node asks for at once |
| ORANGE-019's question: D120's pause in pages or hashes | Either way it is a pause with progress kept, never a stop, so D115 holds. Black prefers keeping 64 pages: the request pace already bounds a liar's listing to about a million hashes a minute, and a heavier ledger is reached sooner |
| GREEN-034 | The CLI shows the schedule's version and takes `-if-version`. Grinding schedule only |

## Items for the owner

- **No consensus change is proposed this round.** Nothing needs your
  approval.
- **No hard-fork case is open on the new base.** BLUE-010 must meet C-6
  when Grey merges it. Otherwise a slow sender of a long branch could
  again never be reached.

How this was checked:
- The code changes from `ae8a103` to `1ae4d46`, BLUE-010's diff and the
  two proposals were read, along with decisions D124 to D128.
- `pkg/chase`'s backoff (`backoffLocked`, `Finish`) was read against
  BLUE-010's sweep.
- `go test ./pkg/p2p ./pkg/chase` passes on `0e58e57` (BLUE-010 on the
  new base; 180 s, including `TestHeavierLedgerLongBareRun`).
- C-6 is from reading and was not reproduced.
