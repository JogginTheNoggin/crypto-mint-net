# GUI-026: "Build a rival claim" with no Parent submits the claim to the node, and peers, instead of building it

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed (GUI/Blue, Round 23C), merged; verified by GUI/Red R24B @ 2aceb1a |
| Found on | main @ 2cacdd5 (PR #146, the Pools view's "Build a rival claim" form, D161) |
| Component | gui/web/static/views.js (`form`: a field's `required` is listed in the field spec but never applied, so an empty required field is sent as no flag); gui/web/static/views/pools.js (the rival claim's `args` drop `-parent` when it is empty); pkg/cli/node.go (`claimCmd`: without `-parent` it submits the claim) |
| Surface | node, then the network (a signed claim handed to the node, which keeps it pending and forwards it) |
| Fixed by | GUI/Blue, Round 23C (`form()` applies and checks `required`; e2e in gui/e2e/views.mjs) |

## Summary

The new form says it "Builds a claim of a mint into an entry on the
parent you choose … and prints its hash and the command to grind it". Its
Parent field is marked `required: true`. It runs:

```
poon claim -seed-file - -serial N [-parent P] [-series S] [-work W] [-max-tries T]
```

`poon claim` does two different things, depending on `-parent`:
- **With `-parent`**, it builds the claim into an entry on that parent
  and keeps it as a branch. Nothing is sent to peers.
- **Without it**, it is the plain claim. It signs the claim and calls
  `SubmitTx`. The node puts it in the pending pool and forwards it to
  its peers.

`required` does nothing. `form()` never reads `fd.required`: no input
gets the `required` attribute, and nothing checks the field on submit.
The same is true of the import forms' File field and of Mint serial. So:

1. Fill in Mint serial and leave Parent empty. The args drop `-parent`,
   since `flag('parent', '')` is nothing.
2. Press "Build claim". poon asks for the seed in the masked field, as
   intended.
3. The claim is **submitted**. The result shows its tid and
   `forward: yes`. That claim can't be taken back.

The command line shown above the button does read
`poon claim -seed-file - -serial 5`, without `-parent`. But nothing else
on the form says that this line claims the mint outright rather than
building a rival entry. The button still reads "Build claim".

## Reproduction

poon-gui at 2cacdd5, `init -example -listen 127.0.0.1:0 -p2p
127.0.0.1:0`, a node run as a GUI job, and a test seed. The steps were
run with Playwright, using the script kept outside the repository.

1. `poon pending -serial 5` returns `[]`.
2. In Pools → "Build a rival claim", fill in Mint serial `5` and leave
   Parent empty. The line reads `poon claim -seed-file - -serial 5`.
3. Press "Build claim", type the seed into the masked field, and press
   Enter. The status reads "done", and the result shows a tid and
   `forward: yes`.
4. `poon pending -serial 5` now lists the claim, tid `bdf62103…`,
   serial 5.

The form's seed handling held up. The seed went only through the masked
prompt (`State.Secret`). It was not in the page, in any job's output, or
on the command line, and the masked field was gone once the job ended.

## Impact

A user who means to prepare a rival claim gets an actual claim instead.
The claim is signed with the seed they typed, put in their node's pending
pool, and forwarded to peers. That is not undone, and it is public at
once:
- the claiming address, and the fact that this key contests the mint,
  are on the wire before the user chose to show them;
- the next entry on the selected tip can take the claim, so the result
  isn't the one the user was setting up.

It needs the user to leave the required field empty and to type the
seed. No one else can do it, and nothing is lost beyond the user's
intent. Rated **low**.

## Recommendation

GUI/Blue:

- **Make `required` real in `form()`.** Set the `required` attribute on
  the input, so the browser refuses an empty submit. Also check it in
  the submit handler (an empty required field is refused, with its
  label named), because `run()` calls `requestSubmit()` and a
  programmatic submit may skip constraint checks in some paths. That
  also fixes the import forms' File field and Mint serial.
- **Belt and braces for this form.** Have the rival claim's `args` always
  send `-parent`, even when it is empty (`['-parent', v.parent]`). poon
  then refuses an empty value, instead of the GUI falling back to the
  plain claim. Or make the form refuse in its own args when Parent is
  empty.
- Optional (Source): `poon claim -parent ""` given explicitly could be
  refused with "-parent needs a value", as D152 did for `-if-digest`, so
  a script passing an unset variable doesn't submit by mistake. That's
  CLI behaviour and goes to Source/Grey, not here.
- e2e (views.mjs): leave Parent empty on "Build a rival claim" and press
  the button. Expect no job, or a refusal, and `poon pending -serial N`
  still `[]`.

## Fix (GUI/Blue, Round 23C)

`form()` in gui/web/static/views.js now applies `required`:

- A required field's input gets the `required` attribute while it is
  shown. A field hidden by `only` is not sent, so it can't be missing.
  The browser then holds an empty one, and no submit event fires.
- The submit handler checks again, so the form holds even when the
  browser's check is off or skipped. If a shown required field is empty,
  nothing runs, and the area says "Nothing was run: fill in Parent."
  (`.missing`, an alert) and focuses that field.

This applies to every required field in the GUI: Mint serial on
"Holdings of a mint" and "Build a rival claim", Parent on the rival
claim, and File on the policy, settings and peer import forms. "Where a
payment stands" reused the serial field spec with its `required: true`,
but its In mint serial is optional (`-in-serial`). It is now marked
`required: false`, so enforcing `required` doesn't break that form.

Not done: always sending `-parent ''`. poon's `claim` reads an empty
`-parent` as no `-parent` (`*parent == ""`), so it would still submit.
If an explicit empty `-parent` should be refused, that is a
pkg/cli change for Source. The GUI never sends one.

Regression test: gui/e2e/views.mjs, on "Build a rival claim". With Mint
serial 5 and Parent cleared, it checks the following:
- Parent has `required`.
- Pressing "Build claim" leaves Parent `:invalid`.
- With `noValidate` set, `requestSubmit()` shows "Nothing was run: fill
  in Parent."
- `/api/jobs` lists no `claim` job, and no seed prompt opened.
- In mint serial on "Where a payment stands" is not required.

On the old code (2cacdd5 + Red's R23B docs) it fails at "Parent is
required".

The same PR takes Red's GUI-025 nit. The peer plan's notes are poon's
phrases (`once applied, … takes them all out`, `2 blocks, 1 of them
network range`), and the GUI ran them together. Each is now made a
sentence, with a capital letter and a full stop. The GUI adds its own
block count only when poon's notes have none, so the count isn't given
twice. The GUI-025 e2e check now asserts both.

## Notes — Round 24B verification (held up)

I checked this with Playwright on 2aceb1a, with a node running as a GUI
job and Mint serial 5 filled in.

- **Parent empty, or only spaces:** pressing "Build claim" starts no job
  and asks for no seed. The form shows "Nothing was run: fill in Parent."
- **The handler check stands on its own.** A raw `submit` event sent to
  the form, which skips the browser's own `required` check, is also
  refused with the same message.
- **Nothing was submitted.** No `claim` job appears in `/api/jobs`, and
  `poon pending -serial 5` stays `[]`.
- **Hidden fields.** `required` follows `only`, so a hidden field can't
  block a form, and an empty hidden field is never sent.
