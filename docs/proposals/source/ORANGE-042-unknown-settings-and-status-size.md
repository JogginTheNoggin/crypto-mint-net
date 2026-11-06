# ORANGE-042: Round 32 measured; saving unknown settings was quadratic, and the status reply after an upgrade

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `6260590` |
| Area | `pkg/nodepolicy` (W-1, D184), `pkg/rpc` and `pkg/node` (status, D184) |
| Effort | one small change and two tests; one proposal |
| Decided by | Grey; White and Black in Part D |

## Summary

D184 keeps the settings in `node.json` that this release does not know
(W-1). It also caps the per-pool reasons in the status reply at 8 and
adds the release to it (GUI ORANGE-023, W-10). I measured both against
the build before them (`5f80582`).

**1. Saving `node.json` took time that grew with the square of the
unknown settings: 5 s for 3,000. Fixed here.**

`Save` put each kept setting back by decoding and re-encoding the whole
policy once per setting (`withUnknown`, `insertSetting`). So the work was
the number of settings times the size of the file.

| unknown settings | file | Load, before → D184 | Save, before | Save, D184 | Save, this fix |
| --- | --- | --- | --- | --- | --- |
| 0 | 0.7 KB | 133 → 118 µs | 0.78 ms | 0.68 ms | 0.62 ms |
| 10 | 1.6 KB | 131 → 174 µs | 0.88 ms | 0.99 ms | 0.69 ms |
| 100 | 10 KB | 108 → 318 µs | 0.82 ms | 8.0 ms | 1.2 ms |
| 1,000 | 99 KB | 125 → 2,026 µs | 0.88 ms | 527 ms | 2.9 ms |
| 3,000 | 301 KB | about 4 ms | | 5,071 ms | 7.0 ms |
| 10,000 | 1.0 MB | 250 µs → 26 ms | 0.75 ms | about 56 s (estimated) | 31 ms |

The "before" build drops the unknown settings, so its Save writes only
the 0.7 KB it knows.

- A release adds a few settings at a time, so a real file holds tens of
  unknown settings at most, and the old code was fine there.
- But `Save` runs under the policy lock on every `-set`, import and
  release change.
- A file edited by hand or by a script, or carried through several newer
  releases, has no bound.
- The fix costs nothing at small counts.

**The fix.** `withUnknown` now groups the kept settings by their path
into a tree, and decodes and encodes each object once, putting all of
its settings in together. The result is byte for byte what the old code
gave:
- a setting whose key this release now writes is still left out;
- so is one whose parent is no longer an object;
- a missing or null parent is still made an empty object;
- a policy with no setting put back is returned as it was.

`TestWithUnknownMatchesOneByOne` compares the two on 300 random sets of
kept settings. The sets use random depths, keys this release writes,
lists, nulls and missing parents. I checked that the test fails when the
"key already written" rule is taken out.
`TestWithUnknownManyIsLinear` puts back 10,000 settings and needs well
under 5 s. It takes about 20 ms.

`Load` grows with the file, as it must: it decodes each object a second
time to find the unknown keys. That is 2 ms at 1,000 settings and 26 ms
at 10,000.

**2. After an upgrade, the status reply no longer grows. It is still
about 255 bytes a pool.**

| pools | build | status, same release | first start after an upgrade | second start |
| --- | --- | --- | --- | --- |
| 1,024 | before | 261 KB, 4.9 ms | 356 KB, 5.6 ms | 261 KB, 4.8 ms |
| 1,024 | D184 | 261 KB, 5.4–6.1 ms | 262 KB, 5.0 ms | 261 KB, 5.4 ms |
| 8,192 | before | 2.09 MB, 43 ms | 2.86 MB, 62 ms | 2.09 MB, 34 ms |
| 8,192 | D184 | 2.09 MB, 52 ms | 2.10 MB, 43 ms | 2.09 MB, 53 ms |

- The cap removes the 769 KB the reasons added at 8,192 pools. The
  upgrade now adds 0.7 KB at any size.
- The release adds 40 bytes.
- A start after the upgrade takes 1.4–1.5 s at 8,192 pools, against
  1.0 s, because it replays every pool once. That is the same in both
  builds.

What is left is the `pools` list: one row of about 255 bytes for every
followed pool. That is 2.1 MB and about 50 ms of the node's time a reply
at 8,192 pools. `poon status -counts`, which its help describes as "for a
view that refreshes often", fetches the whole reply and sums it in the
client. Each status reply also loads `node.json` again to set
`policy_changed`. With the fix above, that costs at most a few
milliseconds.

## Proposals

**Proposal 1: let a status request leave out the pool rows.** Add a
query, for example `GET /v1/status?pools=series`. It would return one
row of totals per followed series, the same totals `poon status -counts`
computes now. `-counts` and a front end that refreshes often would then
use it. At 8,192 pools of one series the reply would go from 2.1 MB to
about 1 KB, and from about 50 ms to well under 1 ms.

The default reply stays as it is, so older clients see no change. White
should check how a newer client asking an older node falls back: an older
node ignores the query and sends the full reply, which `-counts` already
handles.

## Measurements

The machine has 4 vCPUs (Xeon with SHA-NI, AVX2 and AVX-512) and runs Go
1.27.3. The harnesses were test files outside the commit, run on this
base and on `5f80582` in a worktree.

- **Unknown settings:**
  - `nodepolicy.Default()` with the example series, saved, then K unknown
    settings added, each a small object. Half are at the top level and
    half inside `p2p`.
  - `Load` then `Save` on the same file: 20 times for K up to 100, twice
    for K of 1,000 and more.
  - The D184 figure for 10,000 is extrapolated from 1,000 and 3,000,
    which grew 9.6 times for 3 times the settings.
- **Status:**
  - The example series widened to N pools, followed whole. Each pool
    holds one claim and one entry with a 16-try layer, written with
    release "1".
  - `GET /v1/status` through `rpc.NewHandler` on an `httptest` server,
    five times each, after each of three starts:
    - with release "1";
    - with release "2", where every snapshot is from another build, so
      unused;
    - with release "2" again.

## What changed

- `pkg/nodepolicy/unknown.go`: `withUnknown` builds a tree of the kept
  settings (`extraTree`) and puts them back in one pass per object.
  `insertSetting` is removed.
- Tests: `pkg/nodepolicy/orange042_internal_test.go`:
  - `TestWithUnknownMatchesOneByOne`, against the D184 code kept in the
    test as `insertOneByOne`;
  - `TestWithUnknownManyIsLinear`.

## Consensus impact

None.
- `node.json` holds node policy only.
- The fix writes the same bytes as before.
- The status reply is information for the owner.
- The proposal adds a smaller reply and changes no verdict.
