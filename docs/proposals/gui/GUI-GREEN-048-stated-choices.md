# GUI-GREEN-048: A field's fixed choices are the ones poon lists

| Field | Value |
| --- | --- |
| Status | adopted (Round 23 Part A) |
| Based on | main @ f4f7945 |
| Area | gui/e2e/parity.mjs, gui/web/static/views.js (a `choices` field), views/peers.js, views/watch.js, style.css |
| Effort | small |
| Decided by | (Grey) |

## Summary

Some poon flags take values from a fixed set:

- `poon peers -category` takes the book states.
- `poon peers export -category` takes the book states plus `blocked` and
  `timed-out`.
- `poon watch -kind` takes event kinds.
- `poon peer clear -what` takes `flags`, `measures` or `all`.

The GUI keeps its own copy of each of these lists. parity.mjs already
checks that every flag is sent (GUI-GREEN-042), every stated default
(043) and every subcommand (045). It did not check these lists, so the
GUI would quietly fall behind when Source adds a value. D157 just did
this when it added the `unlisted` state.

This proposal:

1. Adds the missing check. A field that offers a fixed set of values, a
   select or a group of boxes marked with `flag:`, must offer exactly the
   values that `poon CMD -h` lists for that flag.
2. Makes the Peers view match Watch. Find peers and Export peers took the
   states as typed text, with the list in the placeholder. They now take
   them as tick boxes, as Watch takes event kinds (GREEN-014).

## Problem

1. **The lists can drift.** The lists are copies in three places: the
   `CATEGORIES` string in peers.js, the export placeholder, and `KINDS` in
   watch.js. Nothing tied them to poon. If Source adds a state or an event
   kind, the GUI cannot filter by it, and every check stays green.
2. **The views took the same kind of value in two ways.** Watch offers
   kinds as boxes ("boxes to tick rather than words to remember").
   Find peers and Export peers made you type `candidate,unlisted` from a
   placeholder, with no help for a typo. poon would refuse a typo only
   when the form was run.

## Idea

- **views.js:** a field type `choices`, built by an exported helper
  `choices(name, options, legend, flag)`.
  - It draws a box for each option, in the fieldset style Watch already
    uses.
  - It sends the ticked options as one comma list. With nothing ticked it
    sends nothing, so poon's default ("all") holds.
  - A select or choices field with `flag:` set carries `data-flag`.
- **Watch:** its kind boxes are now this helper. **Peers:** Find peers and
  Export peers take Categories as boxes. The Clear select (`-what`) is
  marked with `flag: 'what'`.
- **parity.mjs:** for each `[data-flag]` element, the check reads its
  options and the flag's help line in `poon CMD -h`.
  - It drops a trailing parenthesis, such as `(default "all")` or
    `(repeatable, comma-separated)`.
  - It takes the list in either form poon uses: `...: a, b, c` or
    `a, b or c`.
  - The two sets must be equal. A failure names the command, the flag,
    and both lists.
- **style.css:** a fieldset with `hidden` is hidden. Without this,
  `display: flex` beats `[hidden]`, the lesson from GREEN-032.

This adds no consensus rule and changes nothing in `poon`.

## Cost and risk

- Small: about 20 lines in views.js, 25 in parity.mjs, and a few in
  peers.js and watch.js.
- The check reads poon's prose. If Source words a list another way, the
  check finds no list and fails loudly ("poon lists none"), rather than
  passing. Either the wording or the check's pattern is then updated.
- Grind's `-device` (`auto, cpu, gpu:0, ...`) is an open list, not a
  fixed one, so it stays a text field with a datalist and is not marked.

## Prototype

Built on this branch.

- parity.mjs finds four marked fields (Find peers, Export peers, Clear,
  Watch kinds), and all four match poon.
- To prove the check bites, I took `unlisted` out of the GUI's states
  and `grind` out of its kinds for one run. It failed with
  `poon peers -category: the GUI offers candidate, priority, backup,
  archived; poon lists candidate, priority, backup, archived, unlisted`,
  and the same for export and watch.
- views.mjs: ticking `unlisted` and `candidate` in Find peers previews
  `poon peers -category candidate,unlisted -label friend` and finds the
  peer added by hand. watch.mjs's kind checks pass unchanged.
- All nine e2e scripts and the gui Go tests pass.
