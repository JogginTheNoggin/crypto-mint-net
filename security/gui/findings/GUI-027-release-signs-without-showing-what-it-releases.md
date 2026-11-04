# GUI-027: "Release a part" asks for the issuer's seed and signs before it shows which mints the release creates

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed (GUI/Blue, Round 25C), merged; verified by GUI/Red R26B @ eb74797 |
| Found on | main @ 88bd534 (PR #161, Series and keys → "Release a part", D163) |
| Component | gui/web/static/views/crypto.js (the Release form: one danger confirm naming only the command line, then `poo series release FILE -part K -seed-file -`); pkg/cli/seriesparts.go (`seriesRelease` prints the signed release only, without the range `-check` prints) |
| Surface | the series itself (a signed release, valid for anyone to publish, creating every mint up to the end of part K) |
| Fixed by | GUI/Blue, Round 25C (`releaseForm` in crypto.js: Preview, then Sign; e2e in gui/e2e/views.mjs). The printed summary in poo itself is left to Source, optionally |

## Summary

A release of part K is signed by the issuer and "releases every part
before it". A valid release for part K brings serials 0 through the end
of part K into existence, and can never be taken back (D163). poo signs
any K the series allows. With no `max_parts`, that is any K at all. The
signer is offline, so neither poo nor the GUI knows which part was last
released.

The GUI's form puts the signature first and the check after it:

1. **Before the seed,** the only safeguard is a confirm dialog that
   repeats the command line: `Run poo series release …/series.json -part
   50 -seed-file -?`. It says nothing of which serials part 50 covers, or
   how many mints come into being with it.
2. **The seed** goes into the masked field, and poo signs.
3. **The result** is the release JSON (series, part, previous,
   witness), with Save as file. It has no line saying what it releases.
   `poo series release FILE -check RELEASE` prints "releases part 50,
   serials 200 to 203, and with it every part before: serials 0 to 203
   exist", but the signing path doesn't.

The form's own text says "Check a release first with the form below".
Check needs the signed file, though, so it can only come after the
signature.

So a slip of one digit (20 for 2, or 50 for 5) gives a signed release
that would create many more mints than meant, and nothing in the GUI
puts that in numbers before or after the signature. The signed release
sits in the page, in Copy output and in Save as file, ready to publish.

## Reproduction

poo and poo-gui at 88bd534.

1. Make a variable series with a test key as issuer:
   `poo series new -issuer ADDR -variable -part-size 4 -mints 8 >
   series.json`. It has no `-max-parts`.
2. In Series and keys → Release a part, enter that file and Part `50`,
   then press Release.
   - The dialog reads
     `Run poo series release …/series.json -part 50 -seed-file -?`.
   - The masked seed field follows. With the seed entered, the result
     is "done": the release JSON with Save as file.
   - Nothing in it mentions serials or how many mints it releases
     (checked with Playwright). The seed itself was not in the page.
3. `poo series release series.json -check rel50.json` says: "releases
   part 50, serials 200 to 203, and with it every part before: serials
   0 to 203 exist". That's 204 mints, where the series started with 8.

## Impact

The issuer's key signs an authorization bigger than intended. Once the
release is published, or simply leaked from the page, a Save as file
download, or a copied output, anyone can hand it to the network. Every
mint through that part then exists, for good. That changes the supply
the issuer promised holders. Nothing outside the GUI user can cause it,
and the release isn't published by the GUI. It needs the issuer to mistype
and then to publish without checking. Rated **low**, because it is the
issuer's own action, but the outcome can't be undone, which is why the
gate should show the numbers.

## Recommendation

GUI/Blue (crypto.js):

- **Show what the release creates before the seed is asked for.** Before
  Release runs, run `poo series parts FILE -from K -to K`. It needs no
  seed and changes nothing. Put its range in the confirm, for example:
  "Release part 50: serials 200 to 203, and every part before it.
  Serials 0 to 203 will exist (204 mints). This can never be taken
  back." Better still, make it a two-step form like the imports:
  - Preview prints the range.
  - A confirmed "Sign release" then asks for the seed.
  - Any change to the form removes the "Sign release" button (GUI-023).
- **After signing, check the result automatically.** Run
  `series release FILE -check` on the signed output, and show its
  summary next to Save as file.
- Fix the about text ("Check a release first …"), which reads the wrong
  way round.
- Optional (Source): have `poo series release -part K` print the same
  summary line `-check` does (perhaps to stderr, so the JSON stays as it
  is), so the terminal shows it too.
- e2e: Release with Part 50 on a series without max parts. Expect the
  range ("serials 0 to 203", "204 mints") in the confirm or preview
  before the masked field appears.

## Fix (GUI/Blue, Round 25C)

"Release a part" is now two steps, like the import forms (GUI-023).
`releaseForm` in gui/web/static/views/crypto.js does this:

- **Preview.** The form's own line is `poo series parts FILE -from K
  -to K`, which needs no seed. From its JSON, a `release-range` "Read
  first" alert names the release: "releasing part K of series
  <id12> brings into being serials F to L, and with every part before it,
  serials 0 to L: L+1 mints. It can never be taken back." Part 1 gets no
  Sign. It is released by the series' issuer witness, which "Sign a
  series" makes.
- **Sign part K**, under the preview, runs `poo series release FILE
  -part K -seed-file -`. It uses the previewed file and part, never what
  the form holds now. Its confirm names the part, the series and the
  range again. Any input or change in the form takes Sign away, so the
  range on show is always the range Sign signs.
- **After signing,** the release poo printed is held to the preview, by
  its `series` and `part`. If they match, it says "Signed: part K of
  series …, <range>. Save it, then Check a release below confirms it."
  If they don't, for example because the series file changed between
  Preview and Sign, a `release-differs` alert says not to publish it.
- **About text.** The Release form's text now says what Preview and Sign
  do. "Check a release" says it checks a saved release, one made
  elsewhere or after Sign. The old "Check a release first" read the
  wrong way round.

Not done: running `series release FILE -check` on the signed output.
`-check` reads a file (`os.ReadFile`), and the GUI writes none. Its
summary (`releases part K, serials F to L …`) is the range the preview
already showed, and the series and part are compared above. A
`-check -` that reads standard input, or the summary printed with the
release itself, would be a pkg/cli change for Source.

Regression test: gui/e2e/views.mjs. On the 8-mint series with part size
4 and at most 5 parts, the test does the following:
- The form's line is `poo series parts … -from 2 -to 2`.
- The preview names "serials 8 to 11, … serials 0 to 11: 12 mints" with
  no seed asked for.
- Changing Part takes Sign away.
- Sign's confirm names the range and the release line.
- After the seed, `release-made` and Save as file are shown.
- On a second series with no most parts, part 50 is named as
  "serials 0 to 203: 204 mints" before anything is signed.

On the old code (88bd534 + Red's R25B docs) it fails at "the form runs
the preview". In gui/e2e/parity.mjs, `series release -part` is noted as
run by Sign, as the imports' `-apply` is.

## Notes — Round 26B verification (held up)

I checked this with Playwright on eb74797, on the same 8-mint series
with no max parts, at Part 50.

- **Preview comes first, and asks for no seed.** It runs
  `poo series parts FILE -from 50 -to 50`, and the alert reads "Read
  first: releasing part 50 of series 61d1cc6c5ea1 brings into being
  serials 200 to 203, and with every part before it, serials 0 to 203:
  204 mints. It can never be taken back."
- **Sign repeats the range in its confirm** before the masked field
  appears.
- **Any edit to the form removes Sign.**
- **Changing the file on disk between Preview and Sign is caught.** I
  changed `part_size` in the series file after Preview. The signed
  output then names another series. The GUI says "This release is not
  the part previewed (the series file changed?): do not publish it." It
  runs no `-check`.
  - Hardening, not a finding: in that case Save as file is still
    offered for the release it says not to publish. That release is for
    a different series id, so it can't release mints of the previewed
    series. Leaving out Save as (and Copy output) there would make the
    warning final.
- **On success, `-check -` runs on the signed output**, and its summary
  is shown next to Save as file.
