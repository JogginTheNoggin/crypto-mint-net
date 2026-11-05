# RED-093: A node.json key that differs from a setting's name only in letter case is read as that setting but kept as "unknown", so a setting the owner turned off or changed comes back at the next load

| Field | Value |
| --- | --- |
| Severity | Medium |
| Status | fixed |
| Found | Source Round 32 Part B, on `main` @ `6260590` (D184, W-1) |
| Area | `pkg/nodepolicy/unknown.go` (`collectUnknown`, `jsonFields`, `withUnknown`, `insertSetting`) |
| For Black | Not a rule. |
| Fixed by | `claude/project-thread-ggo8dt` @ 290702d (BLUE-033) |

## What is wrong

`encoding/json` matches object keys to struct fields without regard to case
(and by Unicode simple folding: `ſ`, U+017F, matches `s`, tested; the Kelvin sign U+212A for `k` should too, not tested).
`jsonFields` / `collectUnknown` look keys up by exact name. So a key such as
`"ALLOW_REMOTE"` is **applied** by `Load` (it sets `rpc.allow_remote`) and is
also listed as an **unknown** setting. `Save` then writes the real field and
puts the "unknown" key back next to it, with its old value:

1. **A setting the file omits when off comes back.** Many settings are
   `omitempty` (`rpc.allow_remote`, `no_sync`, `no_snapshots`, `grind`,
   `p2p.peers`, `tracking.no_timeout_misses` and so on). The owner (or a
   command) turns one off; `Save` leaves the real key out, and `insertSetting`
   sees no exact key and writes `"ALLOW_REMOTE": true` back. The next `Load`
   reads it as `allow_remote: true`.
2. **A folded key sorts after the real one.** `Save` puts the keys in sorted
   order, so `"liſten"` (U+017F) comes after `"listen"`, and the decoder
   applies the later one. The owner's new `p2p.listen` is replaced by the old
   value of the lookalike at the next load, whatever was saved.
3. The start warning says "settings this release does not know … kept but
   not used". They are used.

The file has to be crafted (a node.json taken from a guide, a backup or a
helper), but that is the case W-1 was asked about, and the effect is that a
defence the owner switched off, or the interface's `allow_remote` (which D149
keeps out of the token holder's reach), is switched on again by a routine
`poo` command or a restart, with no warning that it happened.

## How I checked it

`pkg/nodepolicy` test (throwaway, removed; copy kept with Red's notes):

```
loaded AllowRemote=true unknown=[rpc.ALLOW_REMOTE]
(owner sets AllowRemote=false, Save)  → file still has "ALLOW_REMOTE": true
reloaded AllowRemote=true
```

```
loaded p2p.listen="127.0.0.1:1111" unknown=[p2p.liſten]
(owner sets p2p.listen=127.0.0.1:3333, Save)
  "listen": "127.0.0.1:3333", "liſten": "0.0.0.0:2222"
reloaded p2p.listen="0.0.0.0:2222"
```

## Suggested fix (Blue's call)

- Decide "unknown" the way the decoder decides "known": a key is known if
  `strings.EqualFold`-style folding (the same one `encoding/json` uses)
  matches a field. The simplest sure way: decode once into the struct, once
  with `DisallowUnknownFields`-style bookkeeping, or compare the keys of the
  re-marshalled `Policy` with the file's, folded.
- Better still, make `Load` refuse (or warn and drop) a key that matches a
  field only by folding, so a file has one spelling of each setting.
- In `withUnknown`, never write a key that folds to a key the encoder wrote
  or could write.

## Not tested

Every `omitempty` setting one by one; a real older release reading the file.

## Resolution

Fixed in BLUE-033 (D185). A key is known when `encoding/json` would fill
a field from it: the exact name, or one equal under Unicode case folding
(`fieldFor`). Such a key is never kept as unknown. `Load` applies it,
`Save` writes the setting under its own name in its place, and
`FoldedSettings`, the start warning and `poo doctor` name it. No kept
setting is written beside a key that folds to its name. Red's two cases
now hold the owner's value after a save. Tests:
`TestBlue033FoldedKeyIsTheSetting`, `TestBlue033UnicodeFoldedKey`.
