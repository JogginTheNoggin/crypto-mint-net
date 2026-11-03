# GUI-GREEN-047: Make a key asks for its seed, as D98 says a seed is given

| Field | Value |
| --- | --- |
| Status | proposed (Round 21 Part B) |
| Based on | main @ 4dc7d3d |
| Area | gui/web/static/views/crypto.js (Make a key), gui/web/static/api.js (secretLine), gui/e2e/qol.mjs, gui/e2e/parity.mjs |
| Effort | small |
| Decided by | (Grey) |

## Summary

The Series and keys view's **Make a key** form had a free-text Seed
field that it sent as `poo keygen -seed HEX`. Source's own rule (D98) is
that a seed is never a command-line argument: `poo keygen -h` calls
`-seed` "for test vectors only: other users can read the command line",
and the way to give a seed is `-seed-file` (`-` for standard input).
The form now has a **Use my own seed** box instead. Ticked, the form
runs `poo keygen -seed-file -`, and poo asks for the seed in the GUI's
masked field, as Restore from a seed in the Wallet view already does.

A second, smaller fix rides with it: a secret flag whose value is `-`
holds no secret, so `poo wallet create -restore -` and
`poo keygen -seed-file -` are no longer treated as secret lines. Copy
(GUI-GREEN-046) now works on them, and the terminal saves them in its
history.

## Problem

1. **The GUI did not follow D98.** Every other place where the GUI takes a
   seed uses poo's masked prompt: wallet create's restore box, and
   `-seed-file` in claim and pay, which parity.mjs leaves out with the
   reason "a seed is typed into poo's masked prompt, never named as a file
   (D98)". Make a key was the one exception. It put the seed on the line,
   where it showed in clear text in the form's preview, the terminal's
   echo and "Run in terminal".
2. **The GUI taught the wrong habit.** The GUI exists to show the
   headless command for each action. A person who copied the previewed
   `poo keygen -seed …` into a shell would put a real seed into the
   process list and the shell's history, which is the risk RED-005 and
   D98 closed.
3. **Copy was off on lines that hold no secret.** `api.secretLine`
   matched a secret flag by name only, so `wallet create -restore -`, a
   line whose seed is typed in later, counted as a secret line. Copy was
   disabled on it, and the terminal would not save it.

## Idea

- Make a key: replace the Seed text field with a **Use my own seed**
  check. Ticked, the form adds `-seed-file -`. poo reads the seed through
  `cli.Stdio.Secret` (D101), so the GUI masks it exactly as it masks any
  other seed. Unticked, poo makes a random seed, as before.
- `api.secretLine`: a secret flag counts only when its value is not `-`,
  whether the value is written `-seed -` or `-seed=-`. A flag with no
  value after it still counts, so the check still errs toward a secret.
  `sim -seed` stays exempt.
- parity.mjs: `keygen -seed` joins LEFT_OUT, with D98 as the reason. The
  terminal keeps `keygen -seed HEX` for test vectors, as D98 does.

This adds no consensus rule and changes nothing in `poo`. The GUI only
runs a flag that poo already has.

## Cost and risk

- Small: one field and one args line in crypto.js, and about ten lines
  in api.js.
- The Go side (`shell.SecretFlags`, which redacts values in Jobs) is not
  changed. Redacting `-` as `<hidden>` there is harmless.
- A test vector made in the GUI now means ticking the box and pasting the
  seed into the masked field. The terminal still takes `-seed HEX`.

## Prototype

Built on this branch.

- qol.mjs:
  - Ticking Use my own seed previews exactly
    `poo keygen -alg ed25519 -seed-file -`, and Copy stays on.
  - Make key opens a masked (password) field. A seed typed there gives
    the same address as `poo keygen -seed` with that seed.
  - `secretLine` is checked on `-seed HEX`, `-seed=HEX`, `-restore -`,
    `-restore=-`, `-seed-file -`, `sim -seed 3` and a bare trailing
    `-seed`.
- parity.mjs passes with `keygen -seed` left out.
- All nine e2e scripts pass.
