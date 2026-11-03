# GUI-024: On Windows, a copied command line can run other commands in cmd or PowerShell

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | fixed (GUI/Blue, Round 21C); awaiting GUI/Red's verification |
| Found on | main @ 4dc7d3d (GUI-GREEN-046, "Copy" next to every command line) |
| Component | gui/web/static/views.js (`cmdLine`: Copy copies `api.quote(row.args)`), gui/web/static/api.js (`quoteWindows`), gui/shell/shell.go (`Quote` for the Windows grammar); any form whose field a link can fill, such as Explore's "An address" (`#/explore?address=…`) and "A transaction" (`?tid=…`) |
| Surface | local (the user's own shell, as the user) |
| Fixed by | GUI/Blue, Round 21C, on claude/gui-blue: Copy uses its own `copyLine` (api.js); on Windows a word is copied only from a set both PowerShell and cmd.exe take literally, double-quoted for a space, and any other word turns Copy off with the reason |

## Summary

GUI-GREEN-046 added **Copy** next to the `poo` line each form and panel
shows. Its aim is that "the line can go into a shell, a script, a
service file or a note, and run the same command there". It copies
`api.quote(args)`.

On POSIX that holds. `quotePOSIX` puts every word that isn't plainly
safe in single quotes, so `sh`, `bash` and `zsh` read each word back as
one argument and run nothing else.

On Windows, `api.quote` uses `quoteWindows`. That is the grammar the GUI
terminal splits with (`shell.Windows`, the rules of
`CommandLineToArgvW`). It is not the grammar of any shell a user would
paste into:

- It leaves a word bare unless it holds a space, a tab, `"`, `&`, `|`,
  `<`, `>` or `^`. So `$`, `;`, `(`, `)`, `` ` ``, `%` and `@` pass as
  they are.
  - **PowerShell**, Windows Terminal's default shell, runs `$(…)` in a
    bare argument, and `;` ends a statement there.
  - **cmd.exe** expands `%VAR%`.
- Inside a quoted word it escapes `"` as `\"`. cmd.exe doesn't know that
  escape. Its quote state flips at every `"`, so text after a `\"` is
  outside quotes, and an `&` there starts a new command.

So a field value that the GUI shows as one argument can become extra
commands when the copied line is pasted into a Windows shell.

A field can hold text the user never typed. Explore reads its query
string and fills the form: `#/explore?address=X` and `#/explore?tid=X`
fill and submit "An address" or "A transaction". The session is a
cookie, and a link with a hash sends no token, so a link from anywhere
(a chat, a web page, a mail) to `http://127.0.0.1:PORT/#/explore?…`
opens the user's signed-in GUI with that text in the field. The user
doesn't need to follow a link at all. Pasting an address someone sent
"to look up" puts that sender's text in the line just the same.

## Reproduction

poo-gui at 4dc7d3d, with the Playwright script kept outside the
repository. The GUI was run on Linux, and `/api/info` was answered with
`grammar: "windows"` (as a Windows server answers), so the page quotes
as it does on Windows.

1. Sign in, then open `/#/explore?address=<payload>` as a new page load,
   as a link from outside would.
2. Read "An address"'s command line, and press its **Copy**. Copy is on,
   since no secret flag is in the line.

| payload | line shown and copied (Windows) | in a Windows shell |
| --- | --- | --- |
| `x$(calc)` | `poo address x$(calc)` | PowerShell runs `calc` |
| `a;calc` | `poo address a;calc` | PowerShell runs `poo address a`, then `calc` |
| `x"&calc&"y` | `poo address "x\"&calc&\"y"` | cmd.exe runs `poo address "x\"`, then `calc`, then `\"y"` |

With the POSIX grammar, the same three lines are copied as
`poo address 'x$(calc)'`, `poo address 'a;calc'` and
`poo address 'x"&calc&"y'`. Each is one argument, and nothing else runs.

The shells weren't run here, since this machine has no Windows. The
rows follow PowerShell's argument-mode expansion and cmd.exe's quote
toggling. `calc` stands for any command.

"Run in terminal" is not affected. The GUI terminal splits the line
with its own grammar and runs poo in-process, with no shell.

## Impact

A user on Windows who copies a line from the GUI and pastes it into
PowerShell or cmd.exe can run commands that someone else wrote. Those
commands run as the user, with their files, their wallet and their node.
That is what the feature invites the user to do: "build a command with a
form, copy it, and run it". The line on show looks like one `poo`
command, and only careful reading of the quoting shows otherwise.

It needs a Windows user, a field filled with someone else's text (a link,
or a pasted value), Copy, and a paste into a shell. It gives code
execution, so it is rated **medium**. On Linux and macOS the copy is
safe.

## Recommendation

GUI/Blue, in the page (and `shell.Quote` if Copy should match it):

- **Copy must quote for a shell, not for `CommandLineToArgvW`.** On the
  Windows grammar, give Copy its own quoting:
  - For PowerShell, put each word that isn't plainly safe
    (`^[A-Za-z0-9_\-.,\/:=+\\]+$`, and not starting with `@`) in single quotes, with `'` doubled. Single-quoted text is
    literal in PowerShell: no `$`, `;` or backtick is read. Start the line
    with `& poo` or plain `poo`, and label the button "Copy (PowerShell)".
  - cmd.exe can't safely quote every string (`%` and `!` expand even
    inside quotes). If cmd.exe is to be supported, turn Copy off when a
    word holds `"`, `%`, `!`, `^`, `&`, `|`, `<` or `>`, as it is turned
    off for a secret.
  - Or, simplest: on Windows, turn Copy off, with a tooltip, whenever a
    word is outside the safe set above. Ordinary lines (hex ids,
    addresses, paths without spaces, numbers) still copy.
- Keep `quoteWindows` as it is for the GUI terminal and the line on show.
  It is right for the GUI's own splitting.
- Optionally, have Explore's link parameters fill a field only with text
  that looks like what the field takes (64 hex for `tid`, an address for
  `address`). Then a link can't put arbitrary text in a line.
- e2e test (qol.mjs): with `/api/info` answering `grammar: "windows"`,
  load `#/explore?address=x$(calc)` and `…?address=x"%26calc%26"y`. Then
  either Copy is off, or the clipboard holds a line in which every word
  is literal (`'x$(calc)'`).

## Fix (GUI/Blue, Round 21C)

The fix is only in the page. The line on show, Run in terminal, and the
GUI terminal still use `quote`, which is `shell.Quote`.

- **`copyLine(args)`** (gui/web/static/api.js) gives Copy's line. On
  posix it is `quote`'s. On Windows the paste goes to PowerShell or
  cmd.exe, and no quoting means the same thing to both. So each word is
  checked:
  - A word made only of letters, digits and `_ - . : / \ = +` is copied
    bare. None of these is special to either shell outside quotes.
  - A word that also has spaces is copied in double quotes, unless it
    ends in `\`, because CommandLineToArgvW reads `\"` as an escaped
    quote. A word with a leading or trailing space is refused.
  - Any other word turns Copy off for the line: `$ ; & | < > ^ % ! " '`,
    a backtick, `( ) , @ # { } ~`, non-ASCII, a newline, or an empty word.
    The button's title says why and points to Run in terminal.
- **Tests.**
  - `gui/web/copyline_test.go` runs `copyLine` with the windows grammar
    over 20 cases. Red's three payloads, the other metacharacters, and the
    edge cases all turn Copy off. Plain words, paths and spaced paths are
    copied as expected.
  - e2e `qol.mjs` answers `/api/info` with `grammar: "windows"` and loads
    `#/explore?address=x$(calc)`. The line is shown bare, and Copy is off
    with the reason. `a;calc`, `x"&calc&"y`, `a|b` and `%PATH%` are off
    too. `abc123` copies as is, and `my addr` copies double-quoted. On the
    old code the test fails at the first check.
- docs/FEATURES.md: the "Shows its command" row.
