# GUI proposals (Green)

## Summary

Green's ideas for poo-gui: quality-of-life improvements, nice-to-haves and
explorations. Each file is one proposal with a status that Grey sets.
Prototypes live on `claude/gui-green`. Every idea keeps the GUI a visual
layer: each action is still a `poo` command line run through `cli.Main`.

| ID | Proposal | Effort | Prototype | Status |
| --- | --- | --- | --- | --- |
| [GREEN-001](GREEN-001-getting-started.md) | A Getting started guide of runnable poo commands | small | yes | adopted |
| [GREEN-002](GREEN-002-terminal-completion.md) | Tab completion in the terminal, read from poo's help | small | yes | adopted |
| [GREEN-003](GREEN-003-accessibility.md) | Accessibility fixes: skip link, aria-current, quiet live output | small | yes | adopted |
| [GREEN-004](GREEN-004-machine-readable-help.md) | `poo help -json`, so the GUI never parses usage text | medium | no | accepted, later |
| [GREEN-005](GREEN-005-command-palette.md) | A command palette (Ctrl-K) over views and forms | small | no | deferred |
| [GREEN-006](GREEN-006-linked-ids-and-pool-picture.md) | Linked ids and a live pool picture (after G4) | medium | no | deferred |

## How to try the prototypes

```sh
git checkout claude/gui-green
go -C gui build -o /tmp/poo-gui ./cmd/poo-gui && /tmp/poo-gui
node gui/e2e/qol.mjs /tmp/poo-gui     # the checks for 001 to 003
```
