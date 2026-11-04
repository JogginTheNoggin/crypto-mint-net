# RED-076: A signed series file prints its `series_id` text raw, so it can write terminal codes

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | open |
| Found | Source Round 25 Part B, on `main` @ `e93eb29` (D164 and D166 tooling) |
| Area | `pkg/cli` (`readSeriesInfo`, used by `init -series`, `series info`, `series parts`, `series release`) |
| Related | RED-053 (node text on the terminal) |
| For Black | No consensus rule is touched. |
| Fixed by | |

## What is wrong

`readSeriesInfo` (`pkg/cli/node.go:198`) refuses a signed series file whose
`series_id` is not the id of its params with
`fmt.Errorf("series_id %s is not the id of its params (%s)", ss.ID, s.ID())`.
`ss.ID` is the file's text, printed as it is. The other file text in these
tools is quoted (`unknown field "\x1b[2JFAKE"`).

A signed series file is what an issuer hands to everyone who follows the
series: the default series' distribution channel. The text is read before
the id check, so anyone who can change the file can choose what the terminal
shows.

## How I checked it

```
$ python3 -c '... "series_id": "\x1b[2J\x1b[31mFAKE\x07" ...'  > s_esc.json
$ poo init -dir nx -series s_esc.json
poo: s_esc.json: series_id <ESC>[2J<ESC>[31mFAKE<BEL> is not the id of its params (8dfdf9a3...)
```

The bytes `033 [ 2 J` and `\a` reach the terminal: the screen is cleared and
the next text can claim anything, such as a check that passed.

## What could happen

A tampered series file that fails the check can clear the screen and print
a line that looks like a pass or a different id. The file is refused in any
case, so the harm is a misleading screen.

## Suggested fix (Blue's call)

Print `%q` of `ss.ID`, or the escaped form `rpc` already uses for node text.
A test next to the RED-053 ones: no byte below 0x20 other than newline in
the error.
