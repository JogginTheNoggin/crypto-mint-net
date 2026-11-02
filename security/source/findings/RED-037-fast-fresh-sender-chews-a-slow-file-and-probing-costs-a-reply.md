# RED-037: A fast fresh sender cuts a slow file at its own line rate, byte for byte, and each probe costs the sender up to a parts reply

| Field | Value |
| --- | --- |
| Severity | low |
| Status | partly fixed |
| Found on | `claude/m10-watch` @ 97456c1 (Source Round 13 Part B) |
| Component | pkg/p2p/chase.go (`takeFrom`, `takeRoom`, `mayTake`, `fetchParts`) |
| Surface | peers (many senders; the sender of the honest entry) |
| Fixed by | `claude/project-thread-ggo8dt` @ 8a64066 (BLUE-014) |

## Summary

Read from the code, not run as a test. Two properties of the paid cuts,
beyond what D139 and D140 list:

1. **Bytes for bytes, but not time for time.** A sender with a measure 1.5
   times a kept file's, delivering fast (after one 64 MiB reply or one
   slice of measuring), cuts that file from its end as the parts arrive.
   An honest slow link's file holds hours of its progress; a fresh Sybil
   at line speed removes the same number of bytes in seconds, and then holds
   the room as long as its measure stays above two thirds of the honest
   rate. The attacker's cost is the honest file's size in bytes plus the
   measuring reply, so the cost over time is about the honest link's rate, the
   bandwidth contest D140 names, and the sender can be a fresh id each
   time. The file cut is not lost to a claim, only to a faster liar.
2. **The probe's cost is the sender's.** A waiting fetch with no measure, or one
   older than `partsStale` (2 x max(backoff, slice), 4 minutes at the defaults),
   asks for the part at its offset and throws replies away until measured:
   up to a full parts reply (64 MiB) or one slice. When it then cannot
   cut, it does not ask until the measure is stale and then does it again. At
   the reserve, with a fresh fast holder, a node discards up to a reply from
   every sender it is chased with, every 4 minutes or so, and an honest
   sender serves it. The waste is the sender's upstream and the node's
   downstream; it is bounded by the chase slots and the backoff.

## Impact

Low. Neither is a hold for good, as D140 argues: a holder keeps the room only
by delivering at at least two thirds of the honest rate, and the cut file
resumes. The cost of the denial for a slow link is the link's rate, which for
a link under 35 KB/s is small.

## Recommendation

- Let a cut file keep a record of the offset it had (and the hash of the
  prefix kept, if that is cheap) so that a later sender of the same entry can
  go on from it after the take-over, rather than starting from zero, where
  the cutter turns out to be false.
- Remember a probe's result for the sender's whole backoff (not
  `partsStale`) when the probe found it could not cut, so a node at the
  reserve does not discard a reply from each sender every few minutes.

Test: not run.

## Resolution

Partly fixed (D142). The probe's cost: a waiting chase whose measured
chase wrote nothing asks again only once its own measure lapses,
`partsStale` at first and doubled for each such chase in a row, up to
eight times (`progress.partsMiss`, `measureLasts`); a holder that slows,
stalls or leaves is still cut at once by its own measure. Not taken: a
record of a cut file's old offset, since a cut file already resumes
where it was cut. Left: a fast fresh sender still cuts a slow file at
its own rate, the bandwidth contest D140 names. Test:
`TestMeasureLastsLongerAfterMisses`.
