# GUI-021: Stop on `poon submit -f -` hands the pasted entry or layer to the node, because Stop is the GUI's only end of input

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed (GUI/Blue, Round 15C, PR #68), merged; verified by GUI/Red R16B @ 6a1cdce |
| Found on | claude/gui-4-watch @ 55cf745 |
| Component | gui/jobs/jobs.go (`Job.Cancel` closes the job's input, and a read then sees `io.EOF`), pkg/cli/submit.go (`submitHex`: `-f -` reads standard input to its end with `io.ReadAll`), the Pools view's "Submit an entry or layer" form ("From a file": `-`), the GUI terminal's Ctrl-C and Stop |
| Surface | node (an entry or layer handed to the user's node, which keeps it and serves it to peers) |
| Fixed by | GUI/Blue, Round 15C, on claude/gui-blue: a cancelled job's input read fails with `context.Canceled`; `POST /api/jobs/{id}/eof` ends the input (Ctrl-D, End input, "Send and end input") |

## Summary

The new Submit form takes "From a file: `-`", "to paste it when asked".
poon then runs `poon submit entry|layer -f -`, and `submitHex` reads
standard input with `io.ReadAll`, so it reads until end of input.

The GUI has no way to send end of input:

- the form's answer field sends one line (`/api/jobs/{id}/input`) and then
  shows the field again, because the read is still waiting;
- the GUI terminal has Ctrl-C and Stop, but no Ctrl-D.

The only way to end the job is Stop (or Ctrl-C in the terminal). Stop
cancels the job's context, and the job's input is closed when the context
ends (`input.close` in jobs.go). `io.ReadAll` takes that close as a normal
end of input. It returns the lines pasted so far, and the command goes on:
it decodes them, dials the node (the RPC client does not use the job's
context), and **submits**. The job then exits 0 with the node's reply.

So the button that should mean "don't" is the one that sends. A user who
pastes the wrong entry, or one from a node they don't trust, and presses
Stop to back out has handed it to their node anyway.

This also breaks the rule that the GUI terminal works as headless does. In
a shell, Ctrl-C kills `poon submit … -f -` and nothing is sent. Ctrl-D is
what sends it. In the GUI terminal, Ctrl-C does what Ctrl-D does.

## Reproduction

poon-gui at 55cf745, two data directories from
`poon init -example -listen 127.0.0.1:0 -p2p 127.0.0.1:0`, A and B, each
with a node run as a GUI job. Every step below goes through the GUI's
`/api/jobs`, the same requests the forms send.

1. `claim -seed-file alice.seed -serial 5 -root A`, then
   `build -pool 1 -bytes -root A`. That gives entry `bb44df1f…` and its hex.
2. Start `submit entry -pool 1 -root B -f -`. It waits for input.
3. Send the hex as one input line. The job is still running 1.5 s later,
   because `ReadAll` is waiting for more.
4. Press Stop (`POST /api/jobs/{id}/cancel`).
5. The job exits **0** with
   `{"hash": "bb44df1f…", "switched": false, …}`. On B, `pools` shows pool 1
   with `entries: 1`.

The steps above are enough to reproduce it. The driving script was a
Playwright page calling `fetch`, kept outside the repository.

## Impact

An entry or layer the user tried to withdraw is handed to their node, and
the node treats it as it treats a peer's: it checks it, keeps it, and
serves it on. That can't be undone. The node applies every rule to it, so
it adds nothing the node would not take from a peer, and there's no
consensus effect. Only the user can start the job (`submit` is refused to
the view token). Rated **low**.

## Recommendation

GUI/Blue. The GUI must not hold core logic, and both changes are in the
GUI's own job layer:

- **Stop must not read as end of input.** When the job's context is
  cancelled, have `input.Read` return an error such as `context.Canceled`,
  not `io.EOF`. Keep `io.EOF` for a real end of input. Then `ReadAll`
  fails, `submitHex` returns the error, and nothing is sent, as with Ctrl-C
  in a shell.
- **Give the GUI an end of input.** Add `POST /api/jobs/{id}/eof` (or an
  `eof: true` on input) that closes the input with `io.EOF`. Use it from:
  - Ctrl-D in the terminal, as in a shell;
  - the Submit form. With "From a file: `-`", the form can send the pasted
    hex and then end the input, or it can show a "Done" button next to the
    field.
- Test, in jobs_test: a Main that does `io.ReadAll(s.In)` gets one line,
  then a Cancel. Expect an error, not the line. Add an e2e test: Stop on
  the Submit form's `-f -` job leaves the node without the entry.

Source could also have `submitHex` check `contextOf(out).Err()` after the
read. That would be belt and braces, and the GUI fix is enough without it.

## Fix (GUI/Blue, Round 15C)

All of it is in the GUI's job layer and page. pkg/cli is unchanged.

- **Stop is not an end of input.** `gui/jobs`: once a job is cancelled
  (or has returned), a read of its input fails with
  `input: the job was stopped: context canceled`, never `io.EOF`. So
  `io.ReadAll` in `submitHex` fails, `submit` returns that error, and
  nothing is decoded or sent. This matches Ctrl-C in a shell.
- **A real end of input.** `Job.EOF` and `POST /api/jobs/{id}/eof` end
  the input. What was sent is still read, and then `io.EOF` follows. A
  line sent after that is refused (409).
  - The terminal binds Ctrl-D to it. On a line with text, Ctrl-D sends the
    text first, as a shell does. It also has an End input button.
  - A form's answer field has "Send and end input", which is hidden at a
    secret prompt. The Submit form's "From a file" hint now says to use it.
- **Tests.**
  - `gui/jobs`: `TestStopIsNotEndOfInput` (a line, then Stop: an error,
    not the line; a line, then EOF: the line) and `TestEOFKeepsWhatWasSent`.
    `TestCancelEndsReadAndCommand` now expects the read to fail.
  - `gui/server`: `TestJobEOF`.
  - e2e `views.mjs`: Stop on the Submit form's `-f -` job fails on the
    stopped read, and nothing is decoded. "Send and end input" reaches
    poon.
  - e2e `terminal.mjs`: the same for Ctrl-C and Ctrl-D.
  - Both e2e checks fail on the old code.
- docs/gui.md (How it works, and the terminal's keys).

## Notes — Round 16B verification (held up)

These were checked live against poon-gui at main 6a1cdce. Two
`init -example` data directories, A and B, each ran a node as a GUI job,
and every step used the GUI's `/api/jobs` requests.

- **Stop no longer submits.** `submit entry -pool 1 -root B -f -`, the
  hex of an entry built on A sent as one line, then Stop. The job exits
  1 with "input: the job was stopped: context canceled". B's pool 1 still
  has 0 entries.
- **End of input does submit.** The same steps with
  `POST /api/jobs/{id}/eof` in place of Stop exit 0 with the entry's hash,
  and B's pool 1 then has 1 entry. This is what Ctrl-D does in a shell.
- **End of input at a secret prompt fails.** `wallet create` waiting at
  `new passphrase:` and given an EOF exits 1 with "nothing was entered".
  No wallet file is written, so an empty passphrase is never taken. The
  form's "Send and end input" is hidden while the prompt is secret.
- **Code checks.**
  - Once the input is closed, `Read` returns `errStopped` before anything
    buffered. Line readers (`readSecret`, `seed -`) map that error to
    "no input", so a cancelled prompt never goes ahead.
  - Input after an EOF is refused with 409, so nothing can be added once
    the input has ended.
  - `/eof` is behind the same `X-Poon-GUI` header and Origin check as every
    other POST, and the token covers it.
