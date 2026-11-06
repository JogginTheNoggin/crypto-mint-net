# GUI-035: A key typed in the terminal as several words after -key-file shows in the Jobs list, because `shell.Redact` reads only the next word

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed; verified by GUI/Red R40B @ a79be4b |
| Fixed by | GUI/Blue, Round 39C (gui/shell `keyFileFlag`: the value after `-key-file` or `-seed-file` is read word by word, up to the next flag, a word with `/ \\ .` (tested with and without it), or past 512 characters, and every word of a value that `LooksLikeKey` is hidden; TestRedactKeyFileWords covers groups of 8, openssl's 15-byte lines, two halves, `=` with more words and a dot at the end, and fails on the old code). Also from the R39B notes: `secretLine` tests the first word before the 512-character cap, so one hex word longer than that stays out of history (qol.mjs) |
| Found on | main @ 21c34c8 (#338, #341); the code is from Round 36C (`shell.Redact` with `LooksLikeKey`, GUI-032) |
| Component | gui/shell/secret.go (`keyFileFlag`: tests only `args[i+1]`; `Redact` hides only that word); compare gui/web/static/api.js (`secretLine`: reads the words after the flag as one value) |
| Surface | local (the Jobs list and `/api/jobs`, for as long as the job is kept) |

## Summary

poon-gui has two checks for a key typed where a key file's path goes:

- **The terminal's saved history** (GUI-034). `api.secretLine` joins the
  words after `-key-file` or `-seed-file` into one value, up to a word
  with `/ \ .`, and asks `looksLikeKey`. So a key in groups (`c0ffeec0
  ffeec0ff …`) or as openssl prints it is kept out of the history.
- **The Jobs list** (GUI-032, R36C). `shell.Redact` asks
  `LooksLikeKey(args[i+1])`, the one word after the flag, and hides only
  that word.

The terminal splits a line on spaces, so a key typed with spaces, and
not quoted, is several words. Redact then sees only the first group,
which is too short to look like a key. Nothing is hidden, and every
group shows in `/api/jobs` `args` and `command`. When the first group
is long enough, Redact hides that group only, and the rest of the key
shows after `<hidden>`.

The likeliest way to get here is openssl's own layout. `openssl pkey
-text` prints a private key 15 bytes to a line, as colon hex with a
four-space indent. Pasted into the terminal's one-line input, that is
one word per 15 bytes, which is 30 hex digits. That is under the 32
that `LooksLikeKey` needs, so no word passes.

poon itself refuses the line before reading any file ("poon: usage: poon
sign FILE|- -key-file KEYFILE …"), and its error doesn't repeat the
words. The history check works. Only the Jobs list keeps the key.

## Reproduction

poon-gui at 21c34c8, a fresh `POON_HOME`, Playwright. pay.json is
`{"hash":"sha256","legs":[{"serial":1}]}`. The key is 64 hex digits,
`c0ffee…beef`.

In the terminal, type each line below and press Enter:

1. `sign pay.json -key-file c0ffeec0 ffeec0ff … ffeebeef` (groups of 8).
2. `sign pay.json -key-file c0:ff:…:ee:    c0:ff:…:ee:    be:ef`
   (openssl's 15-byte lines run together).
3. `sign pay.json -key-file <first 32 digits> <last 32 digits>`.

Then read `/api/jobs`:

| Line | args after `-key-file` | redacted |
| --- | --- | --- |
| 1 | all 8 groups, in clear | no |
| 2 | all 3 openssl lines, in clear | no |
| 3 | `<hidden>`, then the last 32 digits in clear | yes |

None of the three lines is in localStorage `poon-gui-history`, which is
right. Each job's output is only poon's usage line.

## Impact

The same as GUI-032 step 3 and GUI-034, for a key typed with spaces:

- a key kept off the online machine sits in poon-gui's job list on it, up
  to 64 jobs, until poon-gui restarts;
- anyone with the GUI session can read it (GUI-001's threat model);
- the Jobs view shows it on screen.

It takes a typed mistake. Nothing is sent to a node or the network, and
it isn't saved to disk. The Wallet form's Key file field refuses all
these spellings before anything runs (GUI-032), so this is
terminal-only. Rated **low**.

## Recommendation

- **GUI/Blue (gui/shell).** Make `keyFileFlag` read the value the way
  `secretLine` does:
  - take the words after the flag that aren't flags (don't start with
    `-`);
  - stop at the first word holding `/`, `\` or `.`, or past 512
    characters;
  - when the run joined with spaces, with and without that last word,
    passes `LooksLikeKey`, hide every word in it.

  `Redact` would then return the flag followed by one `<hidden>`, or
  keep the count of words, whichever reads better in Jobs. This keeps
  the front-end and Go checks the same, as GUI-032 asked for.
- **Optional, simpler.** On any key file flag whose joined run looks
  like a key, hide every argument after the flag. Such a line is a
  mistake poon refuses anyway, so nothing useful is lost from the Jobs
  list.
- **Test (TestRedactKeyFile).** Add the three lines above: groups of 8,
  openssl's 15-byte lines, and two halves of 32. Expect no hex group in
  the result. Also check that `-key-file cold.key extra` still shows
  `cold.key`.

## Notes — Round 40B verification (fixed)

I checked this with Playwright on a79be4b (poon-gui). The same three
terminal lines now show only `<hidden>` words in `/api/jobs` after
`-key-file`:

- groups of 8: eight `<hidden>`;
- openssl's 15-byte lines: three;
- two halves: two.

None of the lines is saved to the history. One hex word of 600 digits
is kept out of the history again, as is `--key-file=` with the same.

The same gap is still open for the secret flags, `-restore` and
`-seed`. `Redact` still hides one word after them, so a seed typed in
groups shows in Jobs. Filed as GUI-036.
