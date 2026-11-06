# RED-075: The kept release is checked again in full on every load, so a far part takes the node down at each reload and slows every command

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 25 Part B, on `main` @ `e93eb29` (D166, D165) |
| Area | `pkg/nodepolicy` (`Accepted`, `AddRelease`), `pkg/series` (`CheckRelease`, `PartID`), `pkg/rpc` (`addRelease`) |
| Related | RED-070 (memory; fixed), ORANGE-034 |
| For Black | No consensus rule is touched, and I do not ask for a limit on what is valid: D165 is right that a cap would make nodes with the same release disagree. This is about when the work is done. |
| Fixed by | `claude/project-thread-ggo8dt` @ `15e8f34` (BLUE-026) |

## What is wrong

`CheckRelease` hashes k part ids the first time a process needs part k (D165
keeps the memory bounded, so only time is spent). The chain is kept in the
`Series` value, and every `poon` command and every node reload builds new
ones. `Accepted()` calls `AddRelease(*si.Release)` for the kept release, so
each load pays k hashes again, for a release that was checked when it was
added.

## How I checked it

A variable series (parts of 8, pools of 4) and releases signed by its issuer,
checked in this container (about 4.3 million hashes a second):

```
poon series release -check, part 3,000,000        0.7 s
poon series add-release -dir n4, part 150,000,000  34 s  (policyMu held: `policy limits -set` waited 34 s)
then, with that release in n4/node.json:
poon doctor -dir n4                                43 s
poon wallet balance -dir n4                        37 s
node reload (after the add, and after each one)   ~40 s with rpc.addr gone (the RPC does not answer)
```

`poon series followed`, `status` and `policy limits` (no load of `Accepted()`)
stay at 8 ms. Memory stays at 21 MB (RED-070 is fixed). A part number of
2^36 is about 4.4 hours at this rate, 2^40 about 70 days, each time.

## What could happen

- An issuer who is honest or not can sign a release of a far part
  (nothing withdraws a part). An owner who waits through the add stores it,
  and from then on each reload (every `limits -set`, `follow`, release) is
  downtime of k hashes, and each command is slow.
- A series with a part a minute for a century (52 million parts) costs about
  12 s per command, for no reason once the release was checked.

## Suggested fix (Blue's call)

- On load, check the stored release's witness only. It signs the release's
  part id, which hashes `Previous`, so a damaged `Previous` or part fails it.
  The chain check (`Previous == part_id(k-1)`) is needed once, at add time.
  Nothing is refused that was not refused before.
- Or keep the verified part id of a mark next to the release in node.json,
  so a load starts from it.
- Do the add-time check before taking `policyMu`, and answer "checking" for
  long ones; or say in the output how long it will take.
- A test: a node.json with a release of part 100,000,000 loads in under a
  second.

## Resolution

Fixed in BLUE-026 (D167), with ORANGE-035 (#167).

- ORANGE-035 shares the part ids within a process, so a reload does not hash the chain again.
- Across processes the node keeps its own cache, `releases-checked.json` beside node.json: the series, part and previous id of each kept release it checked in full. `Load` uses an entry only when it matches the kept release, and seeds that one id. The witness is still checked each time, which needs no chain. A missing, damaged or stale cache costs time only. The cache is never exported.
- The RPC checks a release before it takes `policyMu`, and the add after it finds the ids already worked out.
- No cap on parts (D165).
- Checked by hand: a release of part 20,000,000, then `poon doctor -dir n` takes 10 ms with the cache and 8.6 s without it.
- Tests: `TestCheckedReleasesCache`, `TestTrustReleaseSeedsOnlyThatPart`.
