# RED-104: `poo sign` shows the outputs but not the series, the spent inputs or the hash it used, so a payment file can have a cold key sign in a series the owner did not see

| Field | Value |
| --- | --- |
| Severity | Medium |
| Status | fixed |
| Found | Source Round 35 Part B, on `main` @ `866a4ac` (D193) |
| Area | `pkg/cli/sign.go` (`signCmd`, `SignReport`, `SignLeg`) |
| Fixed by | `claude/project-thread-ggo8dt` @ f835882 (BLUE-036) |
| For Black | Not a consensus rule: nothing about what a witness means or how it is checked changes. |

## What is wrong

`poo sign` is the step where a person decides, on an offline machine, what
a cold key authorises. The file comes from the online machine, which is the
machine an attacker is assumed to hold. What the command prints, with
`-show` or when it signs, is for each leg: the mint **serial**, the **tid**,
the **number** of inputs, and the outputs (address, amount); then the key,
its address, and what is still missing.

It does not print:

- **the series.** The file has a `series` field and so does each
  transaction (`t.Mint.Series`); neither is shown, and the command does not
  check that they agree. An address is the same in every series that uses
  the same hash function (`keygenOut` says so), so a cold key's address
  holds coins in every such series at once, and `1000` is not the same
  amount of anything in two of them;
- **which inputs** the leg spends (`tid:index`): only how many. Someone with a
  watch-only wallet cannot compare what the offline machine shows with
  what the watch-only wallet says it is spending;
- **the hash function** it used. `tid` is computed with the file's `hash`
  field (`hash.Lookup(hid)` then `t.TID(h)`), so a file that names another
  hash prints a tid that matches nothing online; the signature it adds would then
  fail in the node (not run), with no sign on the offline machine of why.

`poo sign` also never checks the signatures it adds (`SignWith` returns a
count), so a wrong key reaches the file as an invalid signature (see
RED-106).

## How I checked it

`pkg/cli` test (throwaway, removed; copy kept with Red's notes): a payment
file whose `series` is `aa…` and whose one leg's transaction is in series
`bb…` (both made up), paying `dd…` 1000 from one input, key an Ed25519 seed.
`poo sign -show` and `poo sign` both succeeded. The output, in full, is the
leg `{serial 5, tid, inputs 1, outputs [dd… 1000]}`, the key and address, and
`"added": 1`. Neither series appears anywhere in it.

I did not build a case where one key holds coins in two live series.

## Suggested fix (Blue's call)

- Print, per leg, the series (full id, and its name if the offline machine
  has a node.json that knows it), the hash function used, and each input as
  `tid:index`.
- Refuse a file where `u.Series` differs from any transaction's mint series.
- Optionally let the person pass `-series` or `-expect-to`/`-expect-amount` so
  the command checks what the person intended instead of only showing it.
- Say, in `-show`, that the amounts are in the units of that series.

## Not tested

A real two-series wallet; whether the node's `wallet pay` fills `series`
and `hash` in every file it writes (read from the code only).

## Resolution

Fixed in BLUE-036 (D194). `poo sign` prints the payment's `series`, the
`hash` each tid uses, and each leg's `spends` as `tid:index`. A file
whose series is not its transactions' is refused (`Unsigned.DecodeLeg`),
and `SignWith` checks each signature before it is written. The optional
`-series` or `-expect-*` flags are not added. Test:
`TestBlue036ColdKeySteps`.
