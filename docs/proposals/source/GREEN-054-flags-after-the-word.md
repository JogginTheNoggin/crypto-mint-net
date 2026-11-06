# GREEN-054: Flags may follow the word in tx, address, tracking and policy use

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `main` @ `d1a9128` |
| Area | `pkg/cli` (`viewFlags.parse` for `poon tx` and `poon address`, `trackingCmd`, the `poon policy list/use/archive/unarchive/delete` branch) |
| Effort | small |
| Consensus | none: how a command line is read. Every flag and word means what it meant |
| Decided by | Grey (Black may skip it) |

## Summary

Green ran D175 as a node runner:

1. Set `recent_window` to 16, its floor, with `poon policy limits -set`. It
   is listed with `in_place: false`, as D175 says it applies on reload.
2. Took 23 entries into one pool, so `bodies.cold` read 7.
3. Read a cold entry's payment back with `poon ledger -below 2`, and an
   address's 22 outputs, most of them in cold entries, with `poon address`.

Everything D174 to D176 describe behaved as written.

One loose end showed up on the way: four commands refused a flag placed
after their word. Each said only "usage":

- `poon tx TID -o table` and `poon tx TID -in-serial 5`
- `poon address ADDR -o table` and `poon address ADDR -limit 3`
- `poon tracking ADDR -o json`
- `poon policy use NAME -root DIR`, and archive, unarchive and delete

Most commands already take flags on either side, through `parseMixed`
(GREEN-039 onward): `poon series parts FILE -o table`, `poon grind jobs 3
-o table`, `poon policy import FILE -apply`. A person who reads an
address, then adds `-o table` to the same line, hit a usage error, and so
did a script that appends `-root`.

## Idea

- **The two view commands.** `viewFlags.parse` reads with `parseMixed`
  and keeps the words that are not flags in `v.pos`. `poon tx` and
  `poon address` take their TID or ADDRESS from there. They still refuse
  two words, and every other view command still refuses any word.
- **`poon tracking`** reads with `parseMixed`, and `[ADDR]` may come before
  or after the flags.
- **The `poon policy` names.** `list`, `use`, `archive`, `unarchive` and
  `delete` read with `parseMixed`. `export`, `import` and `limits` were
  already handled.
- **`poon watch`** takes the same `viewFlags.parse`. It still reads flags
  only, so nothing changes for it.

## How it was tested

- **`TestFlagsAfterTheWord`** runs against a running node and covers:
  - `tx TID -o table`;
  - `address ADDR -o table -limit 1`;
  - `tracking ADDR -o json`;
  - `policy use NAME -root DIR`;
  - that `tx` with two words is still a usage error.

  It fails on `main` at the first case.
- **Checks run locally:** gofmt, go vet, staticcheck, `go test -race ./...`,
  and the nine GUI e2e scripts.

## Ideas for later

- `poon follow` and `poon unfollow` take flags only. A stray word there says
  "takes flags only", which is right.
- A test could walk every command's usage line and check that it takes its
  flags after its words, so a new command cannot miss this.
