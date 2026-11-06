# GUI-036: A seed typed in the terminal as several words after -restore or -seed shows in the Jobs list, except its first word

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed; verified by GUI/Red R41B @ 56c5e0a |
| Fixed by | GUI/Blue, Round 40C (gui/shell `secretWords`: after `-restore` or `-seed`, the next arg is always the value, as poon's flag parser takes it, and every arg after it is hidden too, up to the next flag; after `-restore=` or `--seed=`, the args after it are hidden the same way. No `LooksLikeKey` test, since the value is always a secret. `sim -seed` is unchanged. TestRedactSecretWords covers the three lines above, the value ending at `-wallet` or `-alg`, and `wallet create -restore - -wallet w.json` keeping `w.json`, and fails on the old code) |
| Found on | main @ a79be4b (#352, #354; GUI-035's fix in R39C) |
| Component | gui/shell/secret.go (`Redact`: for a secret flag, `more := 1`, so only the next word is hidden; GUI-035's word-by-word read is only in `keyFileFlag`) |
| Surface | local (the Jobs list and `/api/jobs`, for as long as the job is kept) |

## Summary

GUI-035's fix taught `Redact` to hide every word of a key typed after
`-key-file` or `-seed-file` as several words. The secret flags
themselves, `-restore` (`wallet create`) and `-seed` (`keygen`), still
hide one word.

So a seed typed in groups shows in clear after `<hidden>`:

- `wallet create -restore c0ffeec0 ffeec0ff … ffeebeef` shows 7 of the
  8 groups;
- `keygen -seed` in groups of 16 shows 3 of the 4;
- `-restore=` with two halves shows the second half.

The first group is hidden, and the job is marked `redacted: true`, which
reads as if it were all handled.

A wallet seed restores every key in the wallet, so this is the most
valuable secret the GUI handles. Typing a seed in groups is likely:
people copy a 64-digit seed to paper in groups, and type it back that
way. The terminal's saved history is fine. `secretLine` keeps any line
with `-restore` or `-seed` out of it, with any value but `-`. poon
refuses each of these lines ("-restore must be 64 hex digits"), and its
error doesn't repeat the value.

## Reproduction

poon-gui at a79be4b, a fresh `POON_HOME`, Playwright. The seed is 64 hex
digits, `c0ffee…beef`. In the terminal, type each of these and press
Enter:

1. `wallet create -restore` and the seed in groups of 8.
2. `keygen -seed` and the seed in groups of 16.
3. `wallet create -restore=` and the first 32 digits, a space, then the
   last 32.

`/api/jobs` `command`:

```
poon wallet create -restore '<hidden>' ffeec0ff eec0ffee c0ffeec0 ffeec0ff eec0ffee c0ffeec0 ffeebeef
poon keygen -seed '<hidden>' eec0ffeec0ffeec0 ffeec0ffeec0ffee c0ffeec0ffeebeef
poon wallet create '-restore=<hidden>' ffeec0ffeec0ffeec0ffeec0ffeebeef
```

Each has `redacted: true`, and none is in localStorage.

## Impact

The same as GUI-035, with a seed in place of a key:

- 7/8 of a wallet seed sits in poon-gui's job list, up to 64 jobs, until
  poon-gui restarts;
- anyone with the GUI session can read it (GUI-001's threat model);
- the Jobs view shows it on screen.

It takes a typed mistake, which poon refuses. Nothing is sent anywhere
or saved to disk. The forms never put a seed on the command line: they
use `-seed-file -` and `-restore -`. Rated **low**.

## Recommendation

- **GUI/Blue (gui/shell).** For a secret flag that isn't inline, hide
  every word after it up to the next flag, not only the first. For an
  inline one, hide the words after it the same way.
  - A secret flag's value is always a secret, so this needs no
    `LooksLikeKey` test. A line that has more words after a seed is a
    mistake poon refuses anyway.
  - `sim -seed` stays as it is, since `command(args) == "sim"` returns
    first.
  - Simplest: reuse `keyFileFlag`'s loop without the key test, and stop
    at a flag word.
- **Test (secret_test.go).** Add the three lines above, and expect no
  hex group in the result. Also check `wallet create -restore - -wallet
  w.json` still shows `w.json`.

## Notes — Round 41B verification (fixed)

I checked this with Playwright on 56c5e0a, using the same three lines.
`/api/jobs` now shows only `<hidden>` after `-restore` and `-seed`:

- eight words for the groups of 8;
- four for the groups of 16;
- `-restore=<hidden>` and one more for the halves.

None of the lines is saved to the history.
