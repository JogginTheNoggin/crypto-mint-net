# GUI-026: "Build a rival claim" with no Parent submits the claim to the node, and peers, instead of building it

| Field | Value |
| --- | --- |
| Severity | low |
| Status | open |
| Found on | main @ 2cacdd5 (PR #146, the Pools view's "Build a rival claim" form, D161) |
| Component | gui/web/static/views.js (`form`: a field's `required` is listed in the field spec but never applied, so an empty required field is sent as no flag); gui/web/static/views/pools.js (the rival claim's `args` drop `-parent` when it is empty); pkg/cli/node.go (`claimCmd`: without `-parent` it submits the claim) |
| Surface | node, then the network (a signed claim handed to the node, which keeps it pending and forwards it) |
| Fixed by | (GUI/Blue) |

## Summary

The new form says it "Builds a claim of a mint into an entry on the
parent you choose … and prints its hash and the command to grind it". Its
Parent field is marked `required: true`. It runs:

```
poo claim -seed-file - -serial N [-parent P] [-series S] [-work W] [-max-tries T]
```

`poo claim` does two different things, depending on `-parent`:
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
2. Press "Build claim". poo asks for the seed in the masked field, as
   intended.
3. The claim is **submitted**. The result shows its tid and
   `forward: yes`. That claim can't be taken back.

The command line shown above the button does read
`poo claim -seed-file - -serial 5`, without `-parent`. But nothing else
on the form says that this line claims the mint outright rather than
building a rival entry. The button still reads "Build claim".

## Reproduction

poo-gui at 2cacdd5, `init -example -listen 127.0.0.1:0 -p2p
127.0.0.1:0`, a node run as a GUI job, and a test seed. The steps were
run with Playwright, using the script kept outside the repository.

1. `poo pending -serial 5` returns `[]`.
2. In Pools → "Build a rival claim", fill in Mint serial `5` and leave
   Parent empty. The line reads `poo claim -seed-file - -serial 5`.
3. Press "Build claim", type the seed into the masked field, and press
   Enter. The status reads "done", and the result shows a tid and
   `forward: yes`.
4. `poo pending -serial 5` now lists the claim, tid `bdf62103…`,
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
  send `-parent`, even when it is empty (`['-parent', v.parent]`). poo
  then refuses an empty value, instead of the GUI falling back to the
  plain claim. Or make the form refuse in its own args when Parent is
  empty.
- Optional (Source): `poo claim -parent ""` given explicitly could be
  refused with "-parent needs a value", as D152 did for `-if-digest`, so
  a script passing an unset variable doesn't submit by mistake. That's
  CLI behaviour and goes to Source/Grey, not here.
- e2e (views.mjs): leave Parent empty on "Build a rival claim" and press
  the button. Expect no job, or a refusal, and `poo pending -serial N`
  still `[]`.
