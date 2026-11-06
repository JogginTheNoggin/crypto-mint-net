# GUI-003 — Passphrase masking is an output-regex guess and can fail open

- **ID:** GUI-003
- **Severity:** Low
- **Status:** fixed (merged by GUI/Grey into claude/gui-5-release at fca4369, verified). Since D89 (GUI Round 2 Part A) the GUI masks exactly the lines the core reads through cli.Stdio.Secret (D101) instead of every input line; for GUI/Red to re-check next round.
- **Fix branch:** `claude/gui-blue`
- **Component:** `gui/web/static/terminal.js` (`setSecret`, the `state` handler), interacts with `pkg/cli/wallet.go` prompts

## What

When a poon command reads a passphrase from stdin, the terminal is supposed to
switch the input field to masked (`type=password`) and not echo it. The trigger
is a pure client-side heuristic:

```js
state: s => { if (job === cur) setSecret(s.waiting && /passphrase: ?$/i.test(cur.tail)); }
```

where `cur.tail` is the **last 64 bytes of stdout and stderr concatenated**
(`cur.tail = (cur.tail + c.data).slice(-64)` with no stream distinction). So
masking happens only if, at the moment the job blocks on input, the tail of the
combined output ends in `passphrase:`. The core prompts are `"passphrase: "`
and `"new passphrase: "` (`pkg/cli/wallet.go`), which match today — but the GUI
is guessing at the node's intent from its printed bytes.

It fails open (secret typed in cleartext, visible on screen) whenever:

- a command writes anything to **stderr** after printing the prompt (the
  interleaved stderr bytes push `passphrase:` off the end of `tail`);
- a prompt wording changes or a new secret-reading command uses different text
  (e.g. "enter PIN:", "unlock key:");
- more than 64 bytes are emitted between the prompt and the block;
- the `state:waiting` event is delivered before the prompt chunk (ordering
  race) — masking then lags by one round-trip, during which a fast typist can
  submit in cleartext.

When it fails, the submitted line is shown via `write(text + '\n', 'echo')` —
the passphrase appears in the on-screen scrollback (shoulder-surf, screen
share, screenshot). It is *not* written to localStorage history (input during
a running job skips the history push — that part is correct), and the server
drops the input buffer on job end (`in.buf = nil`), so this is a display /
exposure issue, not a persistence one.

## Why it matters

The GUI promises a masked field for secrets, so users will type real wallet
passphrases into it. A masking rule that depends on guessing the node's state
from its output bytes is brittle: the one time it guesses wrong, a wallet
passphrase is rendered in clear text on a screen the user may be sharing.

## Reproduce

- Run `poon wallet passwd` (or any passphrase-reading command) and have it emit
  a stderr warning line *after* `passphrase: ` but before the read — the field
  stays `type=text`.
- Alternatively, simulate a command that prints `passphrase: ` followed by >64
  bytes before blocking → no mask.

## Suggested fix (for GUI/Blue — do not implement here)

- Make the node **declare** when it is reading a secret instead of the GUI
  sniffing output. Add a signal on the job protocol (e.g. a `state` field like
  `{secret:true}`) that `cli.Main` sets when a command calls its
  passphrase-reading path (wire it through `cli.Stdio`, so headless is
  unaffected). The terminal masks on that flag, not on a regex.
- Until then, harden the heuristic: match on **stdout only**, anchor on the
  known prompt set, and mask eagerly (default to masked while `waiting` on any
  command known to read secrets).
- Consider never echoing a submitted input line verbatim while a job is
  waiting unless it is known-non-secret.

## Notes / what is already right

Input lines during a running job are correctly kept out of localStorage
history, and the server zeroes the unread input buffer on job completion
(`input.close` sets `in.buf = nil`). Keep both.

## Fix (GUI/Blue, branch `claude/gui-blue`)

- Masking no longer depends on the output. The terminal masks its input field (`type=password`, with the prompt reading "input (hidden)") for the whole time a command is running, and does not echo what is sent. A "Show typing" button reveals the input for that command only. Views' input fields (`runJob` in views.js) are masked by default the same way, with a Show button. Because it fails closed, a missed or reworded prompt, interleaved stderr, a >64-byte gap, or an event-ordering race can no longer expose a passphrase.
- The core signal suggested in this finding, where `cli.Main` declares that it is reading a secret, would be a `pkg/cli` change, so it belongs to the Source teams. Passed to GUI/Grey to raise if wanted. Since the GUI now fails closed, the signal would only improve the UX (no masking on non-secret prompts).
- Related, for Source: headless `poon` reads passphrases from stdin with echo on (`pkg/cli/wallet.go`), so in a real terminal the passphrase is visible as it is typed. Passed to GUI/Grey for the Source teams.
- Kept: input during a job stays out of localStorage history, and unread input is dropped when a job ends.
- Regression tests: the e2e checks that the field is masked while `wallet create` waits and that the passphrase is not echoed (gui/e2e/terminal.mjs, gui/e2e/views.mjs).
