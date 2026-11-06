# GUI-GREEN-050: Review of the variable supply plan, and required fields that say so

| Field | Value |
| --- | --- |
| Status | adopted (Round 25 Part A) |
| Based on | main @ 2aceb1a |
| Area | docs/gui-variable-supply-plan.md (review), gui/web/static/style.css, gui/e2e/views.mjs |
| Effort | small |
| Decided by | (Grey) |

## Summary

This proposal has two parts:

1. **A review** of GUI/Grey's plan for variable supply series
   (docs/gui-variable-supply-plan.md), checked against jerry's "Mint
   Series: Fixed and Variable Supply" (Nov 3, 2026) and against the GUI
   rule that the GUI keeps no rule of its own (D84). The plan follows the
   design document closely. The notes below are about where a line of the
   plan could put a rule into the GUI, and what the forms should say
   because a release can never be withdrawn. Nothing here asks for a new
   consensus rule.
2. **A small build:** a required field now says "(required)" in its
   label before it is run. GUI-026 made a required field that is left
   empty run nothing, but the field was only marked after you pressed the
   button.

## Part 1: review of the plan

### Where the plan could put a rule in the GUI

The plan already says that "ranges, part ids and validity always come
from the headless output". Four lines of it could still be read as the GUI
working something out for itself. Each is a small rewording, so the build
keeps to D84.

| Plan line | Note | Suggested wording |
| --- | --- | --- |
| New series: "The form says that N and M must both be whole multiples of the pool size" | This is a consensus rule. Written in the GUI, it is a copy that can drift from Source. The headless command already refuses a bad N or M with its own message, and hints.js can add a next step under that message. | Leave the rule out of the form. Show poon's refusal, as every other form does. If a hint is wanted, take it from the flag's `-h` text. |
| Release a part: "The next part number by default" | If the GUI works out "next", it holds a rule, and the design lets a release of part k also release every part before it. | Leave the field empty, so poon picks its default. A placeholder that states poon's default is checked by GUI-GREEN-043. |
| Explore: "A transaction past the highest known release shown as waiting for a release" | Only poon knows the highest release it has seen. The GUI must not compare a serial with a range itself. | Show the state poon's JSON gives for the transaction (for example a status field), and ask Source for one if it is missing. |
| Series info: "the total supply so far" | This is N + (k − 1) × M, which is a rule if the GUI computes it. | Show it only as a field of poon's output, never computed in the page. |

### What the forms should say, because of the design

- **A release cannot be undone.** The design says "no message can
  withdraw it", and a leaked or lost signer key has lasting effects. The
  Release form's confirmation should say that the release is permanent
  and that it also releases every earlier part, not only
  "Run poon …?". A confirm text that a form can set (see Open points) is
  enough for this.
- **Max parts left empty means no cap.** Wherever the GUI shows max
  parts, it should say "no cap" when poon prints none, never 0.
- **The signer.** The design names a k of n signer policy as the way to
  reduce both key risks. The New series form's help could say so, in one
  line, beside the issuer field.
- **Naming a mint.** A mint of a variable series is named (series, part,
  serial). The lookup links in Explore (GREEN-013) should carry the part
  only where the command takes one. The plan already says this for forms;
  the links should follow the same rule.
- **Masked seed.** The issuer's seed goes in the masked field
  (`-seed-file -`), as in GUI-GREEN-047. The plan already says this.

### Tests the plan could add

The most important behaviour the design adds is that a transaction past
the known range waits and becomes valid once the release arrives. I
suggest adding to views.mjs:

1. Claim a mint of part 2 before part 2 is released, and check that
   pending shows the claim as waiting, not invalid.
2. Import the release, and check that the claim is then accepted.

This shows the GUI never calls a waiting transaction invalid, which is
the reason the design gives for there being no hard fork.

## Part 2: required fields that say so

GUI-026 made `required` mean something. A required field that is left
empty now runs nothing, because without it some commands do something
else. "Build a rival claim" without Parent would submit the claim
instead of building it. But you could not see which fields were
required until you pressed the button and the browser stopped it.
Release a part and Import a release, in the plan, will need required
fields too.

- **style.css:** a label whose input or select is required shows
  "(required)" after its name, in the muted colour. The mark follows the
  `required` attribute, which form() turns on only while the field is
  shown (GUI-026). So a field hidden by `only:` is never marked, and no
  view needs to change.
- **views.mjs:** checks that Parent in "Build a rival claim" is marked
  and that Layer tries, which is optional, is not.

This is a style rule only, and it changes nothing in `poon`.

## Open points for Grey

- A `confirm:` text that a form can set, shown in the danger dialog
  above the command line, for the Release form and for the irreversible
  forms that exist today (pay, peer delete, policy delete). This is
  small, but it is left for Grey to decide with the Release form.

## Prototype

Built on this branch.

- views.mjs:
  - Parent's label in "Build a rival claim" has the `::after` content
    "(required)", and Layer tries has none.
  - The GUI-026 checks pass unchanged.
- All nine e2e scripts and the gui Go tests pass locally.
