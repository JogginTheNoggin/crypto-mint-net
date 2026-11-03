# GUI-023: Import's Apply can import a different file than the one previewed

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed (GUI/Blue Round 18C; Source -if-digest D151, Round 19A), merged; verified by GUI/Red R19B @ e71e317 |
| Found on | main @ 3804c55 (PR #81, the Policies view's "Import node policies" form, D149) |
| Component | gui/web/static/views.js (`importForm`: `feed` sends whatever `picked` holds when a job starts; the Apply button made by a preview stays after the file changes) |
| Surface | node (node.json of one or every saved node policy, from a file someone else made) |
| Fixed by | GUI/Blue, Round 18C, on claude/gui-blue: Apply keeps the preview's args and the very text sent to it; any change to the form after a preview removes Apply; the dialog names the previewed file |

## Summary

The import form is built as a review gate for a file someone else wrote.
**Preview** runs `poo policy import -json FILE`, which changes nothing,
and draws the plan. **Apply** then runs the same command with `-apply`.
The form says "Nothing changes until you press Apply". The user is
meant to apply exactly what they just reviewed.

A file picked in the browser is sent to poo as `FILE = -`. The page
keeps its text in one variable for the whole form, `picked`, and
`feed` sends `picked` to whichever job starts. The Apply button that a
preview adds keeps the arguments of that preview (`last`, ending in `-`),
but **not** the text. So:

1. Pick file A and press Preview. The plan for A is shown, with Apply.
2. Pick file B, in the same field or by mistake. The page sets `picked`
   to B's text. The preview of A and its Apply button stay on the page.
3. Press Apply. The confirm dialog shows only
   `poo policy import … -json -apply -`, so the two files can't be told
   apart. Accepting it imports **B**, which was never previewed.

Two related gaps come from the same cause:

- Typing a path after the preview sets `picked` to null. The old Apply
  then starts `… -apply -` and sends nothing, so the job waits for input
  until it is stopped.
- With a path rather than a picked file, Apply reads the file again. A
  file changed between Preview and Apply, for example in a shared
  download directory, is applied without a new preview. This is the same
  as running the two commands in a shell. A digest check (below) would
  close it for both.

## Reproduction

poo-gui at 3804c55, using `init -example` in a fresh home. A.json is
`poo policy export` of an `init -example` root, so it changes nothing.
B.json is A.json plus `p2p.peers: ["203.0.113.7:7000"]`.

1. In Policies → Import node policies, pick A.json and press Preview.
   The plan reads "No settings change. 3 already the same", and Apply is
   shown.
2. Pick B.json. The Apply button is still shown.
3. Press Apply and accept the dialog. The result reads "added
   p2p.peers.[203.0.113.7:7000] … Saved", and
   `policies/default/node.json` now lists `203.0.113.7`.

The steps were run with Playwright against the real page.

## Impact

A node policy change the user never reviewed is saved, and by default
the running node reloads it. That could be peers, followed series,
accepted issuers or registries from someone else's file. The user did
press Apply on a file they picked, so this is a broken review gate, not
an attack from outside. Merge never overrides a local value without
`-prefer-file`, and machine-local settings need `-full`. It is node
policy, not consensus. Rated **low**.

## Recommendation

GUI/Blue:

- Tie Apply to what was previewed. At preview time, keep the text that
  was sent (`const text = await picked`) with the Apply button, and have
  Apply send that same text, not the current `picked`.
- Remove the Apply button, or disable it with "preview again", as soon
  as the file field, the picked file, or any checkbox changes.
- For a path, or as defence in depth (Source, optional): have
  `poo policy import -json` print a digest of the bundle it read, and add
  `-apply -if-digest D` that refuses a different file. The GUI then
  passes the previewed digest, and the terminal can do the same.
- e2e test: preview A, pick B, then Apply. Expect either no Apply button
  or A's content applied, and B's peer absent from node.json.

## Fix (GUI/Blue, Round 18C)

The fix is in `importForm` in gui/web/static/views.js. poo is unchanged.

- When Preview starts, the form keeps that run's args and the picked
  file's text, read once as `{name, text}`. Apply uses only those. It
  sends the same text and never reads what is picked at the time.
- Any `input` or `change` on the form after a preview removes its Apply
  button: picking another file, typing a path, or ticking an option. To
  apply something else, you preview it first.
- The confirm dialog names the previewed file:
  `Run poo policy import … -apply - with the previewed file NAME?`.
- Test: e2e `views.mjs` previews A, picks B and checks that Apply is
  gone. It then previews B and checks that the plan shows B's peer.
  Ticking an option removes Apply. Finally it previews A again, applies,
  and checks that the dialog names A and that node.json has no B peer.
  On the old code it fails at "picking another file takes Apply away".

Still open for Source, optional: when FILE is a path, poo reads the file
again at Apply, so the file could change on disk between Preview and
Apply. The GUI can't see that without reading the file itself. Red's
suggested bundle digest (`-expect DIGEST` on `-apply`) would close it in
poo, for the terminal too.

## Notes — Round 19B verification (held up)

- **The GUI, checked with Playwright on e71e317.**
  - Preview A, then pick B: the Apply button is gone.
  - Preview A again and press Apply. The dialog reads
    `Run poo policy import -settings -json -apply -if-digest 3da1d4eb… -
    with the previewed file A.json?`.
  - node.json gets nothing from B.
- **Apply sends the previewed file, pinned by digest.** It streams the
  same browser `File` and passes the digest that the preview's plan
  printed. A file that changed after Preview gives another digest and is
  refused.
- **Paths, checked in the CLI.** I previewed X.json with `-json`, which
  printed digest `edef3a18…`. Then I changed a peer in X.json on disk and
  ran `-apply -if-digest edef3a18…`. It exited 1 with "the file is not the
  one previewed … nothing was changed", and node.json is unchanged.
- **Any form change drops Apply.** That covers `input` or `change` on any
  field, checkbox or pick, so the options Apply runs with are the ones
  previewed.
- The digest is read from poo's own `-json` plan, not from the file.

## Hardening (GUI/Blue, Round 20C)

GUI/Red's R20B note was not a finding. Apply added `-if-digest` only when
the preview's JSON carried a digest, so a preview without one would have
offered an unpinned Apply. poo always prints a digest today. The Import
form now offers Apply only for a 64-hex digest. Otherwise it says "Apply
is not offered: the preview printed no digest…". Test: e2e `views.mjs`
blanks the digest in a preview's output on its way to the page and
expects no Apply. That test fails on the old code.
