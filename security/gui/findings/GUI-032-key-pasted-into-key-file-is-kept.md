# GUI-032: A key pasted into "Key file" on Sign with a key kept elsewhere goes on the command line, into poo's error, and into the Jobs list

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed for a plain hex key; verified by GUI/Red R36B @ d25f777, with a residual for keys written with separators (see R36B notes) |
| Fixed by | GUI/Blue, Round 35C (wallet.js: the key is asked for masked unless "The key is in a file" is ticked; a Key file of 32 or more hex digits with no / \\ or . shows as `…` on the command line, and Sign refuses it, runs nothing and empties the field; qol.mjs checks it). Source #291 (D194): key and seed file errors no longer carry the path or OS error text, and the masked prompt says `key:` for `poo sign` and `keygen -raw` |
| Found on | main @ 58273f7 (#283, #285: Wallet → "Sign with a key kept elsewhere", D193) |
| Component | gui/web/static/views/wallet.js (the `keyFile` field: plain text, passed as `-key-file VALUE`); pkg/cli/node.go (`seedFlags.read`: `os.ReadFile(*f.file)`, whose error names the path); gui/jobs, gui/server (`/api/jobs` shows args redacted only for `shell.SecretFlags`, seed and restore) |
| Surface | local (the GUI page, the Jobs list, a job's kept output) |

## Summary

"Sign with a key kept elsewhere" is the first GUI form that takes key
material through a free text field. Every other form that needs a seed
(Make a key, Build claim, Sign a series, Release a part, Restore a
wallet) has no path field at all. It always runs `-seed-file -`, and poo
asks for the seed in a masked field.

The new form's "Key file" field is a plain text input. Its value goes
on the command line as `-key-file VALUE`. Left empty, it runs
`-key-file -`, and the masked prompt works well (checked below). But
the form is about a cold key, and a user holding that key on the
clipboard can easily paste it into the field named "Key". When they do:

1. The command line under the form shows the key while it is typed, and
   Copy puts it on the clipboard with the rest of the line.
2. poo tries `os.ReadFile(KEY)` and prints `poo: open KEY: no such file
   or directory`. The key is in the result on the page.
3. `/api/jobs` lists the job's args with the key in them. `-key-file`
   isn't a secret flag, so nothing is redacted. The job's output, the
   same error, can be replayed from Jobs (`/api/jobs/ID/events?from=0`)
   for as long as the job is kept (up to 64 jobs).
4. "Run in terminal" types the same line, key and all, into the
   terminal. It isn't saved to the browser's history (`term.run` doesn't
   push it), but it is on screen.

None of this reaches an OS process list. poo-gui runs poo in-process
(`cli.Main`), so the argument is never in `/proc/PID/cmdline`. It stays
inside poo-gui and the page.

## Reproduction

poo-gui at 58273f7, a fresh `POO_HOME`, Playwright. pay.json is
`{"hash":"sha256","legs":[{"serial":1}]}`, which is enough to reach the
key read: `poo sign` decodes the legs only after the key.

1. Wallet → Sign with a key kept elsewhere. Co-signing file pay.json,
   Key file a 64-hex-digit key.
   - The field's type is `text`. The `.cmd code` line holds the key
     before anything is run.
2. Sign. The run ends `exit 1`, `poo: open <KEY>: no such file or
   directory`.
3. `/api/jobs`: the sign job's `args` hold the key, and it isn't marked
   redacted. Its event stream from 0 replays the error with the key.

The masked path, for contrast. With Key file empty, the form asks
`seed:` in a `password` field, for a seed, an Ed25519 raw key, and an
ML-DSA-87-sized input. Afterwards the key is in neither the page nor
`/api/jobs`.

## Impact

It takes a user mistake, but the form invites it: a field labelled "Key
file" next to a cold key in the clipboard. The cost is a private key,
kept elsewhere so that it would never be on an online machine, now
sitting in poo-gui's job list and output on that machine, readable by
anyone with the session. GUI-001's threat model still applies: a leaked
GUI session reads `/api/jobs`. Nothing is sent to the network or the
node, and the key goes away when poo-gui restarts or the job is
evicted. Rated **low**.

## Recommendation

- **GUI/Blue (wallet.js), the main fix.** Refuse a Key file value that
  looks like a key rather than a path, before anything runs. For
  example: only hex digits, 32 or more of them, with no `/`, `\` or `.`.
  Say: "This looks like the key itself. Leave Key file empty, and poo
  asks for the key in a masked field." Then clear the field, so the
  `.cmd` line no longer shows it. `form()` already has `ask` and the
  required-field check to hang this on. Better still, make the empty,
  masked way the plain one: a "The key is in a file" box, like Restore
  and Use my own seed, with the path field shown only when it is ticked.
- **Source (pkg/cli), a small help.** In `seedFlags.read`, when
  `-key-file` or `-seed-file` names no file and the value is all hex,
  return "the -key-file value is not a file, and it looks like a key;
  never put a key on the command line: use -key-file - and poo asks for
  it", without echoing the value. A poo user in a shell gets the same
  help, and the GUI's error stops carrying the key.
- **Optional (gui/shell).** Redact a `-key-file` / `-seed-file` value
  that looks like hex in `shell.Redact`, as `-seed` is. That limits
  steps 1 and 3 if the other checks are skipped.
- **Wording nit.** The masked prompt reads `seed:` for a `-raw` key too
  (`seedFlags.read`). "key:" for `poo sign`, and for `keygen -raw`,
  would match what the form asks for.
- e2e (qol.mjs): fill Key file with 64 hex digits and press Sign. Expect
  the refusal, no `sign` job in `/api/jobs`, and a `.cmd code` line
  without the value.

## Notes — Round 36B verification (held up, with a residual)

I checked this with Playwright on d25f777.

- **A plain hex key** (64 digits) in Key file, with the box ticked: the
  line shows `-key-file …`, and Sign refuses with "Nothing was run: Key
  file holds what looks like a key …". The field is emptied, no `sign`
  job starts, and the key isn't in `/api/jobs`.
- **poo's error** (Source #291) no longer names the path: "the key file
  does not exist; give the path of a file holding the key in hex, never
  the key itself". The masked prompt now reads `key:` for `poo sign`
  and `keygen -raw`.
- **Residual: a key written with separators gets past `looksLikeKey`.**
  The check needs 32 hex digits in a row, with no `/ \ .`. Three
  spellings each go on the line, start a job, and sit in
  `/api/jobs`'s args, though poo's error no longer repeats them:
  - colon-separated hex, the way `openssl pkey -text` prints a private
    key;
  - hex in groups of 8 separated by spaces;
  - 64 hex digits with a trailing `.`, as when copied from a sentence.

  Suggested: count hex digits after taking out `:`, spaces and `-`, and
  allow one trailing `.`. Or redact the `-key-file` value in
  `shell.Redact` whenever it holds 32 or more hex digits, which covers
  every spelling in the Jobs list. This isn't filed separately, as it is
  the same exposure as GUI-032 for a less likely paste, and the error
  part is fixed.
- **The refusal itself leaves the form broken** for its next run. Filed
  as GUI-033.

