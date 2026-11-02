# GUI-023: Import's Apply can import a different file than the one previewed

| Field | Value |
| --- | --- |
| Severity | low |
| Status | open |
| Found on | main @ 3804c55 (PR #81, the Policies view's "Import node policies" form, D149) |
| Component | gui/web/static/views.js (`importForm`: `feed` sends whatever `picked` holds when a job starts; the Apply button made by a preview stays after the file changes) |
| Surface | node (node.json of one or every saved node policy, from a file someone else made) |
| Fixed by | _(pending — GUI/Blue)_ |

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
