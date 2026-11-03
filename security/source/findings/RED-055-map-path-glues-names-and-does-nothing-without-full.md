# RED-055: `-map-path` glues names together and does nothing without `-full`, which takes the listen addresses too

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 20 Part B, on `main` @ `522e891` (D155) |
| Area | `pkg/nodepolicy/settings.go` (`PathMap.apply`, `mapPaths`, `Fit`), `pkg/nodepolicy/bundle.go` (`Merge`) |
| For Black | No consensus rule is touched. |
| Fixed by | `claude/project-thread-ggo8dt` @ HASH (BLUE-021) |

## What is wrong

1. **Names are glued.** `apply` cuts `From` off and puts `To` in front
   of the rest. When `From` ends in a slash (what shell completion gives)
   and `To` does not, the slash is lost:

   | `-map-path` | `/home/alice/reg/a.json` becomes |
   | --- | --- |
   | `/home/alice/=/srv/bob` | `/srv/bobreg/a.json` |
   | `/=/srv` | `/srvhome/alice/reg/a.json` |
   | `C:\Users\alice=/srv` | `/srv\r.json` (mixed separators) |

   A `From` in another case on Windows (`c:\users\alice`) is not matched
   at all and is left as it was, with no line saying so.
2. **No effect without `-full`.** `Merge` drops local registry files
   before it imports anything (only http and https registries are
   kept) unless `-full`. `-map-path` is applied first, but the result is
   only listed as "skipped, node settings (-full takes them)". The one
   way to bring a registry file is `-full`, which takes the machine-local
   settings too: `rpc.listen`, `rpc.allow_remote`, `no_sync`,
   `p2p.listen`, `p2p.advertise`, the grind device and threads. The
   preview warns about each (the "read first" and "may not fit" lines),
   but a user who wanted one path mapped has to take them all or none.
3. The "may not fit" line says "-map-path OLD=NEW points it at one" also
   for a path that was already mapped.

## How I checked it

A settings bundle with registries `/home/alice/reg/a.json`,
`/home/alice/regs.json`, `/home/alice`, `C:\Users\alice\r.json`,
`/home/alice/../bob/x.json` and `reg.json`, imported with `-full` and
each map above (the table is the output). `/home/alice=/srv/bob` is
right for all but the Windows path; `reg=/srv` does not touch `reg.json`
(good).

## What could happen

Nothing hostile. A user maps a folder, gets a wrong path, and the
node's registry fetch fails later. The "may not fit" line is the only
catch, and only because the wrong path does not exist.

## Suggested fix (Blue's call)

- Join with a separator when `From` ended in one and `To` did not; match
  case-insensitively where the machine is; compare after cleaning.
- Let `-map-path` (or a `-registries` switch) take local registry files
  without the rest of `-full`, or say in the preview that `-map-path`
  is used only with `-full`.

## Not tested

Windows. The case and separator points are from reading the code.

## Resolution

Fixed (D156).

1. `PathMap.apply` joins the rest of the path to `To` with one separator,
   `To`'s own: a backslash when `To` is written the Windows way, else a
   slash. So `/home/alice/=/srv/bob` gives `/srv/bob/reg/a.json`, `/=/srv`
   gives `/srv/home/alice/reg/a.json`, and `C:\Users\alice=/srv` gives
   `/srv/r.json`. A `From` written the Windows way (a drive letter or a
   backslash) matches in any case and with either separator. The path
   boundary rule stays: `/data` never rewrites `/database`.
2. A registry file that a map rewrote is taken without `-full`. The rest
   of `-full` (listen addresses, rpc, grind device and threads) is not.
   A registry file no map touched is still skipped, and the line now says
   "-map-path OLD=NEW or -full takes it".
3. The fit line for a missing file now reads "-map-path OLD=NEW rewrites
   where the file's paths point", which is true also for a path that was
   already mapped.

Tests: `TestMapPathJoins` (Red's table and more) and
`TestMapPathWithoutFull` (pkg/nodepolicy).
