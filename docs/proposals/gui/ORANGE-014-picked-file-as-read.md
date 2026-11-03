# ORANGE-014: Send a picked policy file as it is read, and stop when `poo` stops

| Field | Value |
| --- | --- |
| Status | adopted (Round 19 Part A) |
| Based on | main @ 3804c55 |
| Area | gui/web/static/api.js (`inputFile`), gui/web/static/views.js (Import node policies) |
| Effort | small |
| Decided by | |

## Summary

Round 18's Import node policies form (D149) can take a file picked in the
browser. The page sends the file to `poo policy import -` on its
standard input. To do that, it read the whole file into one string the
moment the file was picked, then sent that string. `poo` reads at most
16 MiB of a bundle and refuses anything larger. A file picked by mistake
was therefore read whole into the tab first, whatever its size:

- a 500 MB file took 5.3 s before `poo` refused it;
- the browser's memory grew by about 490 MB.

With the change, the page reads the file 256 KiB at a time and sends
each part as it is read. When `poo` stops reading, because it refused
the file or finished, the next send fails, and nothing more of the file
is read. The page no longer decides how large a file may be: `poo`'s own
limit decides, as before. A normal policy file goes in one part and
takes the same time as before.

| Picked file | Before: until `poo` answered, browser memory | After |
| --- | --- | --- |
| A policy file (14 bytes) | 88 ms | 68 ms |
| 16 MiB | 0.92 s | 0.89 s |
| 100 MB | 1.0 to 1.8 s, +138 MB | 0.9 s, +87 MB |
| 500 MB | 4.9 to 5.3 s, +491 MB | 0.9 s, +47 MB |

Over 16 MiB, `poo` answered "not a node policy bundle: over 16 MiB" in
every case. About 66 parts of 256 KiB were sent before and after: the
sends always stopped once `poo` had refused the file. The server's
memory is the same before and after, at most about 70 MB, because the
job's input buffer holds what was sent until `poo` reads it. The
"before" times leave out the whole-file read at pick time, which came
before the button was pressed, so the real gap was larger than the table
shows.

## Change

- `api.inputFile(id, file)` reads `file.slice(off, off + PART)` as an
  `ArrayBuffer`, decodes it with one streaming `TextDecoder`, and sends
  each part with `more: true` (the parts from ORANGE-013). It then ends
  the input. A UTF-8 character split between two parts is joined again
  by the decoder. Checked: a 600,002-byte text of two-byte characters
  read in 256 KiB parts comes back identical.
- The import form keeps the picked `File` rather than its text, and
  hands it to `inputFile` for Preview and again for Apply.

## Checks

- `gui/e2e/views.mjs` (which imports a picked file), and the streams,
  terminal, qol, work and parity checks, all pass.
- The 5-minute soak and the usual scenarios are in the README re-check
  table.

## Modularity

Nothing to challenge: the change is one more small helper beside
`api.input`, and any view that hands a picked file to a command can use
it.

## Consensus

None. This only changes how the page hands a file's text to `poo`.
