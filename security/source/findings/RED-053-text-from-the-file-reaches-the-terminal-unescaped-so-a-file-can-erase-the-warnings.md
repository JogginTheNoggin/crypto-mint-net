# RED-053: Text from the file reaches the terminal unescaped, so a file can erase the warnings

| Field | Value |
| --- | --- |
| Severity | Medium |
| Status | fixed |
| Found | Source Round 20 Part B, on `main` @ `522e891` (D155; the plan printing is from D149 on) |
| Area | `pkg/cli/policyport.go` (`printPlan`, `planLine`), `pkg/nodepolicy` (`Validate`, `Fit`) |
| For Black | No consensus rule is touched. |
| Fixed by | `claude/project-thread-ggo8dt` @ HASH (BLUE-021) |

## What is wrong

The preview is what the user trusts before `-apply`: it puts
"read first" lines up and lists what changes. The file it reads is
somebody else's, and the settings import exists to take files from
others. Several settings are free text that `Validate` only checks to be
non-empty, and the plan prints them as they are:

- `p2p.registries[]` (a URL or a file path): printed in the
  "read first" line, in the `+ p2p.registries:` line, in the
  "skipped, node settings" line, and in the new "may not fit this
  machine" line.
- The messages for an invalid `p2p.peers` or `p2p.advertise` entry quote
  the bad value with `%q` but then add Go's own text, which holds the
  raw bytes (an ESC shows in the error as `^[`).

A registry string with an escape sequence or a newline therefore
reaches the terminal as one. `-json` is not affected: it is escaped.

## How I checked it

A settings bundle that sets `max_peers` to 100000, `no_relay`,
`hop_limit` 1, two tracking limits to 1e9, and a registry
`https://reg.example/x\u001b[8A\u001b[J` (cursor up 8 lines, erase to the
end of the screen):

```
$ poo settings import hide.json -prefer-file > hide.out
```

`hide.out` holds, as bytes, the "read first" line, the lines for
`p2p.hop_limit`, `p2p.max_peers`, `p2p.no_relay` and the registry, and
then `^[[8A^[[J`. I fed it through a terminal emulator (pyte, 160 by 30):

```
import digest 97cd18d, tracking.block_lim
  + tracking.block_limit: 1000000000
  + tracking.timeout_limit: 1000000000
  ! issuers.any: took the file's true (was false)
dry run: nothing was changed; add -apply to make this change
```

The "read first" warning and the `max_peers`, `no_relay`, `hop_limit` and
registry lines are gone, and the digest line is cut. The user sees a
shorter and milder plan than the one the digest binds. A newline in a
registry string also lets the file print a line of its own that looks
like part of the plan (`note: fake line` printed on its own line in my
test).

Same for `poo policy import` (the strings are the same).

## What could happen

A shared "setup" file hides what it changes: the warning about taking
many pools or peers, and its own registry address, while `-if-digest`
and `-apply` are still honest about the bytes. The user previews, sees a
small plan, and applies it.

## Suggested fix (Blue's call)

- Print every value that came from a file through one function that
  escapes control characters (`strconv.Quote` style, or replace them
  with `?`), in `printPlan`, `planLine`, the Fit lines, the error
  messages, and `listSome`.
- In `Validate`, refuse control characters in registry locations (and
  other free text: template names, custom names), since none is valid.
- The node's own log lines that name a registry would need the same;
  I did not test those.

## Not tested

The GUI preview. It shows the plan in a web view, where escape
sequences do nothing, but a newline or a long string might still hide
a line; that is the GUI team's to look at.

## Also read, not run: the preview touches the file's paths

D155's fit check calls `os.Stat` on each local registry path in the file
(`grind.ThisMachine`, `Machine.Exists`) while the plan is built, so a
**dry run** now acts on text from the file. On Windows a path such as
`\\host\share\x` makes that call connect to `host`, which offers the
sign-in (NTLM) of the user to a machine the file names. It happens only
with `-full` (without it local registry files are dropped before the
check) and the node would open the same path once the registry is read,
but a preview is meant to change and reach nothing. I could not run
Windows here. Suggest: do not stat a path that is not a plain local
path (a UNC path, a device name), or say in the preview that it looked.

## Resolution

Fixed (D156).

- `Validate` refuses text with a control character, a bidirectional
  override, a line or paragraph separator, or bytes that are not UTF-8.
  It checks every string of a node policy, map keys too (`checkText`).
  None is valid in a setting. The error quotes the field and value with
  Go escapes, so it cannot carry the raw byte either. A file with Red's
  registry is refused before any plan is printed.
- The CLI's plan printing passes every value from the file through
  `termSafe`: `printPlan`, `planLine` and through it `listSome`, the
  "read first", skipped, fit and warning lines, and the node policy's
  name and directory. That still holds if a later field escapes the
  check.
- The fit check no longer opens a network or device path (`\\host\share`,
  `//host/share`, `\\?\...`). It names the path as not checked ("the
  preview does not open it, the node will when it reads the registry").
- Tests: `TestControlCharactersRefused` and
  `TestFitDoesNotOpenNetworkPaths` (pkg/nodepolicy);
  `TestImportRefusesTerminalEscapes` and `TestTermSafe` (pkg/cli).
- Not done here: the node's own log lines. They get only text that
  passed `Validate`, so the same check covers them. The GUI preview is
  the GUI team's to look at, as Red said.
