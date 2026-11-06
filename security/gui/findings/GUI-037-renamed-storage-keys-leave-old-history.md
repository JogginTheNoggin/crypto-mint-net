# GUI-037: The poon rename moved the browser's storage keys, so the old saved history is never filtered or forgotten, and the chosen node policy and wallet are dropped

| Field | Value |
| --- | --- |
| Severity | low |
| Status | open |
| Fixed by | |
| Found on | main @ a79be4b (#353, #354: the poo → poon rename, D204) |
| Component | gui/web/static/terminal.js (`HISTORY_KEY` `poo-gui-history` → `poon-gui-history`), where.js (`poo-gui-where`, `poo-gui-theme` → `poon-gui-…`), views/wallet.js (`poo-gui-wallet` → `poon-gui-wallet`, and `-series`) |
| Surface | local (the browser profile's storage for the GUI's origin) |

## Summary

The rename changed every localStorage key the GUI uses, and nothing reads
the old ones. A browser that ran an earlier build on the same origin
(127.0.0.1 and the port poon-gui reuses) keeps them.

- **`poo-gui-history`.** This is the saved terminal history, up to 200
  lines.
  - It may hold lines saved before the fixes that now keep secrets out.
    GUI-034 kept keys after `-key-file` out only from Round 37C, and
    other spellings later. GUI-034's cleanup worked because
    `loadHistory` filters with `saveable` on every load, which drops old
    secret lines.
  - That only works on the key it reads. The old key is never loaded, so
    it is never filtered. "Forget history" removes only
    `poon-gui-history`, so it can't clear the old key either. The
    leftover lines stay in the profile on disk, and the GUI gives no way
    to remove them.
- **`poo-gui-where`.** This holds Settings' `-root` and `-policy`. It
  is dropped quietly, so after the upgrade every view runs against
  poon's default root (`$POON_HOME` or `~/.poon`), not the node policy
  the user chose. Settings shows it empty, which is the only sign.
- **`poo-gui-wallet`** (and `-series`). The Wallet view's wallet file is
  dropped, and it falls back to `wallet.json`. **`poo-gui-theme`** is
  dropped too.

## Reproduction

poon-gui at a79be4b, Playwright, one origin.

1. Set these in localStorage, as an earlier build left them:
   - `poo-gui-history` = `["status", "sign pay.json -key-file <64 hex digits>"]`
   - `poo-gui-where` = `{"root":"/data/mynode","policy":""}`
   - `poo-gui-wallet` = `cold-wallet.json`
2. Reload.
   - Up-arrow in the terminal recalls nothing.
   - Settings' root is empty, and views add no `-root`.
   - The Wallet view uses `wallet.json`.
3. Terminal → Forget history. Afterwards all three old keys are still
   in localStorage, the key line included.

## Impact

- **No release yet.** No alpha has been tagged, so this reaches only
  people who ran a build from before the rename: jerry and the teams'
  test profiles. That is why it is **low**. But the rename is the last
  chance to deal with it cheaply, and any later rename of the
  placeholder names (the memo says more may come) would do the same.
- **History.** A key typed before GUI-034's fix stays in the browser
  profile, and nothing in the GUI clears it.
- **Settings.** Commands quietly act on a different node policy than the
  one chosen. A user may then run, say, Make a key, or `init`, against
  the default root.

## Recommendation

- **GUI/Blue (terminal.js, where.js, wallet.js).** Once on load, for each
  old key:
  - `poo-gui-history`: remove it. Or move its lines into the new key
    through `saveable`, so secret lines are dropped, and then remove it.
  - `poo-gui-where`, `poo-gui-theme`, `poo-gui-wallet` and
    `poo-gui-wallet-series`: copy each to its new key when the new key is
    unset, then remove the old one.
- **Forget history.** Also remove `poo-gui-history`, for a browser that
  never ran the migration.
- **For the next rename.** Keep the storage key names in one place, with
  a list of old names that the migration reads. The site domain and
  network name will change, and the GUI's own name may change too.
- **e2e (qol.mjs).** Set the old keys, reload, and expect:
  - the history's key line is gone;
  - `status` is recalled;
  - Settings shows `/data/mynode`;
  - the Wallet view shows `cold-wallet.json`;
  - no `poo-gui-*` key is left.
