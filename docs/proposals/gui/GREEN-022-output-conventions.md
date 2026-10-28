# GREEN-022: Output conventions the GUI relies on

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | claude/gui-4-watch @ 50a0edd |
| Area | pkg/cli (Source/Grey); gui render and hints |
| Effort | small |
| Decided by | (Grey) |

## Summary

Three small mismatches in how commands print, found while wiring
`poo doctor` into the GUI. Each one makes the GUI either parse text or
special-case a command:

1. `poo doctor` takes `-json`, while the M10 commands take
   `-o json|table`.
2. Doctor's `fix` is free text that mixes a command with prose, such as
   "poo init (or poo init -example to try the worked example), or pick
   one with -policy or -dir".
3. Errors carry no stable code, so the GUI's next-step hints (GREEN-007)
   and node banner (GREEN-011) match message text.

## Idea

For Source/Grey to decide, smallest first:
1. Give `doctor` the shared `-o json|table` flag, and keep `-json` as an
   alias so scripts still work.
2. Add `fix_args` (an argv, for example `["init", "-example"]`) next to
   the prose `fix` when the fix is a single command. The GUI then offers
   a button for exactly that argv and shows the prose as it is.
3. Have the common errors print a short stable code, for example
   `poo: E_NO_NODE: rpc: is the node running? …`, or include it in a
   JSON error object under `-o json`. The GUI matches the code, and the
   wording can change freely.

## Cost and risk

Item 1 is a few lines. Items 2 and 3 add fields and prefixes, which is
additive for JSON and a visible change for text output. That is why
this is Source/Grey's call. Nothing in the GUI breaks if none of it
happens: the GUI keeps parsing as it does now.

## Prototype

None. These are Source changes.
