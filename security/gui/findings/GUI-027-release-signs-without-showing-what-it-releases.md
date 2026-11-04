# GUI-027: "Release a part" asks for the issuer's seed and signs before it shows which mints the release creates

| Field | Value |
| --- | --- |
| Severity | low |
| Status | open |
| Found on | main @ 88bd534 (PR #161, Series and keys → "Release a part", D163) |
| Component | gui/web/static/views/crypto.js (the Release form: one danger confirm naming only the command line, then `poo series release FILE -part K -seed-file -`); pkg/cli/seriesparts.go (`seriesRelease` prints the signed release only, without the range `-check` prints) |
| Surface | the series itself (a signed release, valid for anyone to publish, creating every mint up to the end of part K) |
| Fixed by | (GUI/Blue; the printed summary, optionally, by Source) |

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
