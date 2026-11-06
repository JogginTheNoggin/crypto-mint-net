# GUI-034: A key typed after -key-file in the terminal is saved to the browser's history, though the Jobs list now hides it

| Field | Value |
| --- | --- |
| Severity | low |
| Status | open |
| Found on | main @ 7eb16ab (#310, #315: `shell.LooksLikeKey` in `shell.Redact`, GUI-032 R36B) |
| Component | gui/web/static/api.js (`secretLine`: only the secret flags, seed and restore, keep a line out of saved history); gui/web/static/terminal.js (`saveable`, `saveHistory`: localStorage `poo-gui-history`) |
| Surface | local (the browser profile's storage, kept across sessions) |

## Summary

GUI-006 keeps a terminal line that may hold a secret out of the history
the browser saves. `saveable` asks `api.secretLine`, which looks only
for `-seed` and `-restore` with a value other than `-`.

Round 37A taught poo-gui that a key pasted where a key file's path goes
is a secret too. `shell.Redact` now hides a `-key-file` or `-seed-file`
value that `LooksLikeKey`, so the Jobs list shows `<hidden>`. The
terminal's saved history wasn't given the same test. A line typed as
`poo sign pay.json -key-file c0ffee…beef` (or the colon-separated form)
is:
- shown in Jobs as `-key-file <hidden>`, redacted, which is right;
- saved, whole, to localStorage `poo-gui-history`. That history is kept
  across poo-gui restarts and browser sessions (up to 200 lines), and
  Up-arrow recalls it.

Typing a key on a command line is a mistake. poo's own help says "a
key is never taken on the command line". But it is the mistake GUI-032
guards against in the form, and GUI-006 guards against for seeds in the
terminal. Here the one copy that outlives poo-gui is the one left
unguarded.

## Reproduction

poo-gui at 7eb16ab, a fresh `POO_HOME`, Playwright. pay.json is
`{"hash":"sha256","legs":[{"serial":1}]}`.

1. Terminal: type `sign pay.json -key-file <64 hex digits>`, then Enter.
   Then the same with the key colon-separated (`c0:ff:ee:…`).
2. `/api/jobs`: both sign jobs show `<hidden>` for the key, with
   `redacted: true`.
3. `localStorage.getItem('poo-gui-history')` holds both lines, keys and
   all.

## Impact

The key ends up in the browser profile on disk, on the online machine,
for as long as the history keeps it, and Up-arrow shows it to anyone at
the screen. Getting there takes a typed mistake, which poo then refuses
("the key file does not exist; … never the key itself"). Nothing is
sent anywhere. Forget history clears it. Rated **low**.

## Recommendation

- **GUI/Blue (api.js).** Make `secretLine` also true when a `-key-file`
  or `-seed-file` value (as the next word, or after `=`) passes the
  Wallet view's `looksLikeKey`, the same test as `shell.LooksLikeKey`.
  Move `looksLikeKey` to a shared module (api.js or render.js) so the
  two front-end uses can't drift. The line stays in this session's
  Up-arrow and is never saved, as GUI-006 does for seeds.
- **Optional.** While loading, drop any saved line that now counts as
  secret. `loadHistory` already filters with `saveable`, so this comes
  free once `secretLine` knows the new case: lines saved before the fix
  are dropped on the next load.
- e2e (terminal.mjs or qol.mjs): type `sign x.json -key-file` followed
  by 64 hex digits, and also the colon form. Expect neither in
  `localStorage['poo-gui-history']`, and expect a path such as
  `-key-file cold.key` to still be saved.
