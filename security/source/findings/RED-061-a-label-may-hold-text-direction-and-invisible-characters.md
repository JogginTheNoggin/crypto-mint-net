# RED-061: A label may hold text-direction and invisible characters

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | Open |
| Found | Source Round 21 Part B, on `main` @ `8662766` (D157) |
| Area | `pkg/peers/peers.go` (`CheckLabel`), `pkg/cli` (`-o table`) |
| For Black | No consensus rule is touched. |

## What is wrong

`CheckLabel` refuses what `unicode.IsControl` names (category Cc). The
text that BLUE-021 added to `Validate` refuses control characters,
bidirectional overrides and bytes that are not UTF-8. A label gets only
the first of those, so these are accepted: U+202E (right-to-left
override), U+2028 (line separator), U+200B (zero width space), U+061C
(Arabic letter mark). The plan prints a label from a file escaped, but
`poo peers -o table` and `poo tracking` print the stored label as it is.

## How I checked it

`CheckLabel` on `"a‮b"`, `"a b"`, `"a​b"`, `"؜"`:
all accepted. The table renderer for a book record with such a label
writes the raw bytes (my test logged them quoted).

## What could happen

A label from a shared peer file reorders or hides text on the line that
shows it, for example makes `friend` look like another label, or two
addresses' labels look the same in the table. The address column is
plain text (`plainText` refuses the same characters there), so the
effect stays inside the label.

## Suggested fix (Blue's call)

Use one text check for labels and for `Validate` (the same function),
and make `termSafe` apply to the table output of anything that came from
a file.

## Not tested

The GUI's peer table, which shows labels in a web view.
