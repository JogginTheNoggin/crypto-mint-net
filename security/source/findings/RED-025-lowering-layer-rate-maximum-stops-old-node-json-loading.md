# RED-025: A node.json with a `layer_rate` that was valid before D127 no longer loads, so the node does not start

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed |
| Found on | `claude/m10-watch` @ 1ae4d46 (Source Round 9 Part B) |
| Component | pkg/nodepolicy/nodepolicy.go (`MaxLayerRate`, `Grind.validate`, `Load`), pkg/node/node.go (`Open`) |
| Surface | node policy / upgrade |
| Fixed by | `claude/project-thread-ggo8dt` @ 5f993ee (BLUE-010) |

## Summary

D127 lowers `MaxLayerRate` from 1,000 to 50 and `Validate` refuses anything
above it. `Load` validates, and `node.Open`, the daemon's reload, `poo doctor`
and every command that reads node.json use `Load`. A node.json written under
Round 8 with a `layer_rate` of 51 to 1,000 (valid then, and shown in the
GUI's form) now fails to load, so the node does not start and a SIGHUP
reload fails, instead of using the new maximum.

## Reproduction

A temporary test in `pkg/nodepolicy` (not committed): `Default()` with a
grind section of `layer_floor` 8 and `layer_rate` 10, saved, the file edited
to `"layer_rate": 200`, then `Load`:

```
load of an R8-valid node.json with layer_rate 200:
  nodepolicy: invalid: grind: layer_rate must be 0 (the default, 10) to 50
```

## Impact

Low. No release has been tagged and the option is new in Round 8, so only
people running the stacked branches are affected. It matters as a rule:
tightening a bound in `Validate` makes every older file that was valid a
file the node refuses, which at the next release becomes a failure to start
after an upgrade.

## Recommendation

Validate strictly on write (RPC, CLI, GUI) and be tolerant on read: `Load`
clamps `layer_rate` to `MaxLayerRate` and logs a warning (the node's use is
already bounded by the rate), or `Validate` takes a mode. Keep a note in
`docs/modules/nodepolicy.md` that a bound may only be lowered this way.

Test: a node.json with `layer_rate` 200 opens, the grinder runs at 50, and a
warning is logged; the same value written through the RPC is refused.

## Resolution

Fixed by BLUE-010 (decision D128), as recommended. `nodepolicy.Load`
brings a `grind.layer_rate` over `MaxLayerRate` down to it and says so in
`Policy.Lowered` (not saved). `node.Open` logs each note as a warning and
`poo doctor` shows it as a warn line. `Save`, and with it the RPC, CLI
and GUI writes, still refuses the value. `docs/modules/nodepolicy.md`
notes that a bound may only be lowered this way. Test:
`TestLoadLowersAValueOverALoweredBound` (`pkg/nodepolicy`).
