# ORANGE-012: Send a large submitted entry as it was read; Round 15 measurements

| Field | Value |
| --- | --- |
| Status | adopted by Source as D147 (PR #70, Round 16); the GUI side merged only the docs, and the prototype is not merged on the GUI side, because the change is in `pkg/cli` and `pkg/rpc`. Source: the change is in `pkg/cli` and `pkg/rpc` |
| Based on | claude/gui-4-watch @ 55cf745 |
| Area | pkg/cli/submit.go (`poon submit entry`), pkg/rpc/client.go (`SubmitEntryHex`) |
| Effort | small |
| Decided by | |

## Summary

Round 15 asked for three things to be measured: Submit with `-f` on large
files, the new state event for each ask, and the Wallet view with the
wallet lock.

- **Submit with `-f`.** A 128 MiB entry took about 12 s to the node's
  reply. `poon submit` used about 1 GB of heap and allocated 2 GB, and the
  node peaked at 1.3 GB. The GUI runs `poon` in its own process, so that
  gigabyte is poon-gui's. Most of the client's share went into encoding
  the entry again after decoding it, plus three copies of its hex. The
  prototype checks the entry, lets the decoded form go, and streams the
  hex it read into the request. It takes the 128 MiB entry to 5 to 7 s,
  a peak of 625 MiB and 617 MiB allocated. The bytes the node gets are
  the ones in the file, so nothing about what is checked or kept changes.
- **State events.** There is exactly one state event per ask: 1,000 asks
  gave 1,001 events (the first is the opening state), about 150 µs each
  end to end. Nothing to change.
- **Wallet lock.** Wallet commands are as fast as in Round 14: about
  160 ms for `wallet address`, almost all of it the passphrase's Argon2id,
  and 6 ms for commands that don't unlock. The lock does change what
  happens when commands overlap. With 16 `wallet address` commands at
  once, Round 14 reported 16 successes and Round 15 reports 1 success
  and 15 refusals ("the file changed since it was read ... run the
  command again"). That refusal is the GUI-019 fix doing its job, so this
  is a note for Source, not a regression. Two Wallet forms pressed within
  about 160 ms of each other, or a form and a terminal command, will see
  it.

The 5-minute soak and every earlier scenario held against Round 14 (see
the re-check table in the README).

## Submit with `-f`: measurements

A test made entries of 1 to 128 MiB of small transactions, wrote each as
hex to a file, and ran `cli.Main(["submit", "entry", "-pool", "0", "-f",
FILE])` in-process, as poon-gui does, against a real `poon node`. The node
refuses these entries ("transaction of another series") only after
decoding and checking them, so the whole path is measured. Heap is the
client's, sampled every millisecond. Two runs each:

| Entry (hex file) | Before: time, heap peak, allocated | After: time, heap peak, allocated |
| --- | --- | --- |
| 1 MiB (2 MiB) | 50 to 55 ms, 8 to 11 MiB, 16 MiB | 31 to 36 ms, 5 MiB, 5 MiB |
| 16 MiB (32 MiB) | 0.80 to 0.84 s, 126 MiB, 253 MiB | 0.49 to 0.51 s, 78 MiB, 77 MiB |
| 64 MiB (128 MiB) | 4.4 to 4.5 s, 504 MiB, 1,013 MiB | 2.5 to 2.9 s, 313 MiB, 309 MiB |
| 128 MiB (256 MiB) | 12.1 to 12.7 s, 1,009 to 1,137 MiB, 2,025 MiB | 5.2 to 6.8 s, 625 MiB, 617 MiB |

A CPU profile of the 64 MiB case before the change showed where the
client's time went: `rpc.Client.SubmitEntry` took 27% of it, of which
`entry.Encode` was 18% and `hex.EncodeToString` 16%. A further 30% was
the garbage collector, and `memmove` made up 29% of all samples (string
copies, `bytes.Clone`).

The node's own peak for the 128 MiB entry was 1.3 GB (`VmHWM`), and it
held 700 MB afterwards. That is Source's side, for Source Orange: the
node decodes a JSON string of 256 MiB, then the hex, then the entry. The
body is unbounded only with the full token (D144), so this is not a
security issue.

## Change (prototype, on claude/project-thread-f0df7u)

- `submitHex` returns the trimmed bytes it read, not a `string` copy.
- `poon submit entry` decodes the hex into one buffer to check the entry,
  as before. It then keeps only the parent hash (for the GREEN-039
  message), lets the buffer and the decoded entry go, and sends with
  `SubmitEntryHex`.
- `rpc.Client.SubmitEntryHex(ref, hex)` writes `{"series":…,"pool":…,`
  `"entry":"` and the hex, then `"}`, as one streamed request body
  through `io.MultiReader`. It is the same JSON `SubmitEntry` sends. The
  hex is safe inside a JSON string because it has already decoded as hex.
  The node decodes it with the same `entry.Decode` the client just ran,
  so it accepts or refuses what it did before. The only difference is
  that it now gets the person's bytes, where before it got the client's
  re-encoding of them.
  `doHeader` now calls a new `doReader`, so every other call is
  unchanged.
- `SubmitEntry` stays for library callers and is listed in the parity
  test's `noCommandYet`, since `poon submit entry` now calls the hex form.

`poon submit layer` is unchanged: a layer is small.

`go test ./pkg/cli ./pkg/rpc` passes, including `submit_test.go` and the
parity test.

## For Source: the wallet lock under overlap

| 16 `poon wallet address` at once | Round 14 (3a7a39c) | Round 15 (55cf745) |
| --- | --- | --- |
| Succeeded | 16 | 1 |
| Refused, "run the command again" | 0 | 15 |
| Wall time | 1.07 s | 1.07 s |
| One `wallet address` alone | 169 ms | 163 ms |

Round 14's 16 successes were the lost updates GUI-019 found, so Round 15
is right to refuse. One idea for Source, which is not a performance
change: a command that changed only derived keys and no longer needs the
passphrase, such as `wallet address`, could read the file again and
redo its change once when `Save` returns `ErrChanged`, instead of asking
the person to run it again. That decision belongs to Source and Red.

## State events for each ask

A job asked for 1,000 lines one at a time, and the test answered each
`waiting` state. Result: 1,001 state events, 1,000 of them waiting, in
150 ms in total. An ask costs one small event, and no event is sent
while nothing changes.

## Consensus

None. The node receives the same bytes. Nothing here needs Black.
