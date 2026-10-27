# GUI-002 — Terminal line-splitting diverges from a real shell (breaks "identical to headless")

- **ID:** GUI-002
- **Severity:** Medium (correctness / trust-boundary; not directly RCE)
- **Status:** fix-proposed
- **Fix branch:** `claude/gui-blue`
- **Component:** `gui/shell/shell.go` (`Split`), `gui/web/static/terminal.js` (client-side `clear`)

## What

jerry's HARD rule is that the GUI terminal "works identically to headless
`poo`": the same line a user types must produce the same command. The GUI
honors the *execution* half of this (every command runs through `cli.Main`,
the same entry point as the `poo` binary — confirmed in `gui/jobs/jobs.go`).
But the *parsing* half is a bespoke reimplementation in `shell.Split`, and it
deliberately and accidentally diverges from a POSIX shell, so the **same typed
line can yield different arguments in the GUI than in a real shell**.

Concrete divergences:

1. **Stray backslashes are kept.** The doc says: "any other backslash is kept,
   so Windows paths such as `C:\Users\a` work unquoted." A POSIX shell does the
   opposite — outside quotes `\x` collapses to `x`. So the line
   `poo foo C:\new` gives:
   - GUI (`shell.Split`): arg `C:\new` (backslash kept)
   - bash: arg `C:new` (backslash removed; and `\t`, `\n` etc. would drop the
     slash too)
   A path or value containing backslashes is parsed differently depending on
   where you typed it.

2. **CR / LF treated as word separators.** `Split` splits on `' '`, `'\t'`,
   `'\n'`, `'\r'`. A POSIX shell treats a newline as a *command terminator*,
   not an in-word separator. Reachable through the API (`POST /api/jobs`
   `{line}` and `GET /api/split`) with an embedded newline: a shell would run
   two commands (or error), the GUI silently folds both lines into one
   argument vector.

3. **Leading `poo` is stripped.** `Split` drops a leading `poo` word so
   `poo status` and `status` are equal. Harmless for normal use, but it means
   a genuine first argument whose value is `poo` is swallowed, and it is a
   rule a real shell does not have.

4. **`clear` is intercepted client-side.** `terminal.js` short-circuits
   `text.trim() === 'clear'` and never sends it to `cli.Main`. In headless,
   `poo clear` would reach the command dispatcher (and presumably error as an
   unknown command). So `clear` behaves differently in the two "identical"
   terminals, and it is the one command the GUI silently computes itself —
   exactly the "GUI decides something" the architecture forbids.

## Why it matters

This is a divergence finding, not a direct exploit, but it undermines the
property the whole GUI design leans on: that the terminal is a faithful
pass-through. Any divergence is a place where a user's mental model ("this is
just poo") is wrong, which is where mistakes and, later, security-relevant
surprises live — e.g. a path/argument that looks safe when tested in a shell
but lands differently through the GUI, or a copy-pasted multi-line snippet
that collapses into one unexpected invocation. It also quietly violates a
stated hard rule, which Grey's mitigation posture depends on.

## Reproduce

- `GET /api/split?line=foo%20C:%5Cnew` → `args: ["foo","C:\\new"]`; compare
  `printf '%s\n' foo C:\new` under bash → `foo` `C:new`.
- `POST /api/jobs {"line":"a\nb c"}` → one job with args `["a","b","c"]`.
- Type `clear` in the GUI terminal → screen clears, no job runs
  (`section.dataset.runs` still increments but `cli.Main` is never called).

## Suggested fix (for GUI/Blue — do not implement here)

- Decide and **document the exact grammar** the terminal implements, and state
  plainly where it is *not* POSIX (the Windows-path backslash rule is a
  deliberate choice — keep it, but call it out, and ideally gate it to
  Windows). Add table-driven tests pinning the grammar.
- Reject or normalize embedded CR/LF in `{line}` instead of treating them as
  separators (a single terminal line can't contain them anyway; the API
  should not accept a meaning a shell wouldn't give).
- Reconsider the client-side `clear` special case: either route it through a
  real `poo` builtin so both terminals agree, or document it as a pure
  display-only convenience of the GUI terminal (not a command).
- Keep the single-source-of-truth property: splitting already happens
  server-side in `shell.Split` (the terminal sends `{line}`), which is good —
  the fix is about matching the reference semantics, not about moving code.

## Notes / what is already right

No OS shell is involved: args go straight to `cli.Main`, so there is **no**
classic shell metacharacter injection (`;`, `|`, `$()`, globbing, redirection
are all inert). That is the correct design and should be preserved. This
finding is strictly about the splitter's fidelity to the reference shell.

## Fix (GUI/Blue, branch `claude/gui-blue`)

- `gui/shell` now has two pinned grammars, and `shell.Native` picks the one for the platform:
  - **POSIX** splits exactly as `sh` does. An unquoted backslash escapes any character (`C:\new` → `C:new`). In `"..."`, a backslash escapes only `` $ ` " \ ``. A word-initial `#` begins a comment.
  - **Windows** splits as cmd.exe plus Go's poo.exe argv parsing. Backslashes are literal except before `"`, and single quotes are ordinary characters. The Windows-path rule now applies only on Windows, where it is what the real shell does.
- Syntax that a shell would turn into something other than one poo command, or expand, is **refused** with an error that tells the user to quote it. On POSIX that is `| & ; < > ( ) $` backtick `* ? [`, a leading `~`, and `$` or backtick inside `"..."`. On Windows it is `& | < > ^`. Before this fix, these characters were passed through silently.
- CR/LF anywhere in a line is refused (`ErrNewline`) in both `/api/jobs {line}` and `/api/split`.
- Leading `poo` stripping is kept as a documented convenience (the prompt already reads `poo`; `poo poo` is an unknown command in a shell anyway).
- `clear` is no longer intercepted. It goes to `cli.Main` like any other line. Clearing the screen is now a Clear button plus Ctrl-L, which are display controls rather than commands.
- `shell.Quote` and the page's `api.quote` now quote per grammar, so a view's "Run in terminal" splits back to the same argv. `GET /api/grammar` tells the page which grammar to use, and `TestQuoteMatchesShell` (gui/web) runs the JS through node and compares it with `shell.Quote`.
- The grammar is documented in docs/gui.md, under "The terminal's grammar", including what is not modeled (bash `!` history, brace expansion, PowerShell quoting).
- Regression tests: `TestSplitPOSIX`, `TestSplitWindows`, `TestQuoteRoundTrip`, `FuzzSplitQuote` (gui/shell); `TestSplitEndpoint` (gui/server); e2e checks that `clear` reaches poo, the Clear button clears, and `status | cat` is refused (gui/e2e/terminal.mjs).
- Not verified on a real Windows host: the Windows grammar follows Go's documented rules, and its tests run on Linux.
