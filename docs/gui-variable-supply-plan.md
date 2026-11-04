# GUI plan: variable supply series

GUI/Grey, Round 24. This plan covers how the GUI will show variable supply
series, as set out in jerry's "Mint Series: Fixed and Variable Supply"
(Nov 3, 2026). The GUI builds each part only after Source merges the
headless command it runs. Every form runs that command through
`cli.Main`, so the GUI keeps no rule of its own: ranges, part ids and
validity always come from the headless output (D84). Command names below
are placeholders until Source names them; parity.mjs will list any flag
Source adds that no form sends.

## Fixed series stay as they are

A fixed series is shown exactly as it is today. A field or column for
parts appears only when the series is variable, so nothing changes for the
series that exist now.

## Planned views

| Where | What the GUI will show or run | Waits on |
| --- | --- | --- |
| Series and keys: New series | Supply type (fixed or variable). For a variable series, also part size M, an optional max parts, and the issuer, who must sign. The issuer's seed goes in the masked field. The form says that N and M must both be whole multiples of the pool size, and the headless command checks it | The series command's new flags |
| Series and keys: Series info | Supply type, M, max parts, the highest released part, the total supply so far, and a table of parts (part, serials, pools, units). The table is drawn from the headless output | The series info output |
| Series and keys: Release a part | The next part number by default and the issuer's seed in the masked field. A preview shows the part id, serials and pools the headless command prints, then a confirmed Release. Save as file, so the issuer can publish the release | The release command |
| Series and keys: Import a release | A path or a file picked in the browser, sent as `-`. A preview, then Apply, the same way the other imports work | The release import command |
| Node | Follow a variable series as today. If Source adds node policy for how many parts a node follows or stores, it is a settable limit (D159 style), never a validity rule | Source's node policy, if any |
| Explore, Pools and payments, Wallet | A mint named (series, part, serial) wherever a mint is shown or asked for. A part field only where the headless command takes one, otherwise worked out by poo. Claim forms are unchanged apart from that | The mint name in the JSON outputs |
| Explore: pending and tx | A transaction past the highest known release shown as "waiting for a release", never as invalid | The pending and tx output |
| Watch | A "release" event kind, if Source adds one; the kind boxes come from `-kind`'s list (GUI-GREEN-048) | The watch event kind |

## Tests

- views.mjs: make a variable series, release part 2 and check the parts
  table. Claim a mint of part 2. Import a release on a second home.
- parity.mjs already fails on any new flag or subcommand that no form
  covers, and on a new event kind that the boxes lack.
- FEATURES.md gets one GUI row per feature as each one lands.

## Round 27: key algorithms and series limits (Source #176 and after)

Source is adding `-alg` to the commands that take a seed (GREEN-051), plus
per-series size limits, a default reserve series and SLH-DSA-256s. Each
of these is built once it reaches main:

| Where | What the GUI will show or run | Waits on |
| --- | --- | --- |
| Sign a series, Release a part, Build a rival claim | A Key algorithm field, with suggestions from the list `poo crypto` prints (the same list Make a key uses). Release a part keeps the algorithm in what it previewed, so Sign signs with the key previewed | `-alg` on `series sign`, `series release`, `claim` |
| Settable limits | Per-series size limits show up as fields in Set limits by themselves, since the fields come from `poo policy limits` (GUI-GREEN-049). If Source puts them on each series instead, they go on a per-series form, with the floors and ceilings the headless output gives | Source's per-series limits |
| Node, Series and keys | The default reserve series, shown as the headless output gives it, with no rule of its own in the GUI | Source's reserve series |
| Make a key | SLH-DSA-256s appears in the algorithm suggestions by itself, because they come from `poo crypto` | Already covered |
