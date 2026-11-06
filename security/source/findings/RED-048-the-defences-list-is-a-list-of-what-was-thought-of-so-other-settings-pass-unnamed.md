# RED-048: `Defences`, the lines an import with `-settings` names first, is a list of what was thought of, so other settings that blind or starve a node go unnamed

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed |
| Found on | `main` @ 533dbcd (Source Round 18 Part B) |
| Component | pkg/nodepolicy/bundle.go (`Defences`, `isDefence`) |
| Surface | a bundle file from someone else; `poon policy import -settings`, `-full` |
| Fixed by | `claude/project-thread-ggo8dt` @ a133161 (BLUE-019) |

## Summary

The default import now takes only what to follow, and `Followed` is the right
kind of list: anything new is outside it until someone adds it. With
`-settings` (or `-full`) the import takes every other setting, and names the
dangerous ones first through `Plan.Defences`. That list is the other kind: a
list of what was thought of, so a setting not on it is taken and shown like
a harmless one. A file can set these under `-settings` and none is named:

`p2p.check_seconds` (-1: never ask priority peers for their stated weight),
`p2p.ping_seconds` and `p2p.review_seconds` (-1: never), `p2p.margin`,
`p2p.priority_slots`, `p2p.backup_slots`, `p2p.hop_limit`, `p2p.chunk_size`,
`p2p.hash_limit`, `p2p.layer_limit`, `max_pending`, `seen_size`, `ties`,
`log.level`, `grind.layer_floor`, `grind.layer_rate`.

## Reproduction

`TestRedDefencesCoverage` (scratchpad `red_r18b_nodepolicy_test.go.txt`
describes it; goes in `pkg/nodepolicy`): `Merge(Default(), file,
MergeOptions{Settings: true})` for a file that sets the settings above:

```
taken and NOT named in Defences: [log.level p2p.backup_slots p2p.hash_limit
p2p.ping_seconds ties grind.layer_floor grind.layer_rate p2p.hop_limit
p2p.layer_limit seen_size max_pending p2p.chunk_size p2p.priority_slots
p2p.review_seconds p2p.check_seconds p2p.margin]
named in Defences: []
```

## Impact

Low: `-settings` is asked for by name, and the plan still lists every line.
But it is the case D150 set out to make visible, and the settings that stop
a node asking its peers for their weights, or shrink the sizes it accepts,
are the ones whose effect is least obvious from the line.

## Recommendation

Invert it as `Followed` is: under `-settings`, put every changed setting
in the "read first" list except a short list of settings known to be only
cosmetic (`log.format`, labels), and keep `Defences` for the sharper wording.
Then a setting added later is flagged by default. A test that walks the
`Policy` struct and fails for a field in neither list keeps it so.

For Black: `p2p.check_seconds` -1 and `review_seconds` -1 are existing,
documented values; please confirm that a node with them (or with small
`hash_limit`, `layer_limit`, `chunk_size`) can still find and accept a
heavier ledger, since a bundle can now set them.

## Resolution

Fixed (D152). The list is now an allow-list, the same way `Followed` is.
Under `-settings`, `Plan.Defences` names every node-setting line the
import changes, except those under `nodepolicy.Cosmetic` (only
`log.format`). The thought-of list `Defences` is gone, so a setting added
later is named by default. Test: `TestSettingsNameEveryNodeSetting`
(every setting in this report). Black's question on `check_seconds` -1
and the small limits is for Part D.
