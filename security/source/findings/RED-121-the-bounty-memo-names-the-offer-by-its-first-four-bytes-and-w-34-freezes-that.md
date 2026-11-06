# RED-121: The bounty memo names the offer by its first four bytes, and W-34 freezes that

| Field | Value |
| --- | --- |
| Severity | Low (a 32-bit collision among a payer's own offers to one grinder; it costs the grinder one payment) |
| Status | fixed |
| Found | Source Round 39 Part B, on `main` @ `fa8dad7` (D202, W-34: `BountyMemo`, `sentFor`) |
| Area | `pkg/cli/bounty.go` (`BountyMemo`, `sentFor`), `pkg/hash/hash.go` (`Short`) |
| Fixed by | |
| Fixed by | `claude/project-thread-ggo8dt` @ a29af78 (BLUE-040) |
| For Black | Not a rule. For White: this is the memo W-34 freezes before the first tag. |

## What is wrong

`sentFor` finds the wallet's payment of a claim by the memo `"bounty " + ID.Short()`, where `Short()` is the first 4 bytes (8 hex characters), plus the series, address and amount. W-34 now freezes that start of the memo.
`used` (the tids already on another claim) only covers the offers still in `Mine`; an offer that is finished and in the done file is not there.
So take a finished offer A and a live offer B whose ids start with the same 4 bytes, paid to the same grinder with the same series and amount. `sentFor` finds A's payment as B's, `bounty pay` "records" it, and nothing is sent for B. The grinder gets an ack with a tid that was already for A.
A payer that posts to one grinder at one price (the likely case) reaches a collision with 50 % chance at about 77,000 offers, and 1 % at about 9,300. It is rare, but the memo is the one thing W-34 asks to keep forever.

## How I checked it

`pkg/cli` test (throwaway, removed; copy kept with Red's notes): two offers, ids equal except the last byte, so equal `Short()`; the wallet has one selected payment with offer A's memo; `sentFor` for offer B:

```
A f0620088…90b4  B f0620088…904b  same Short: true
sentFor(B) found A's payment: true (err <nil>)
```

I forced the equal ids by editing the byte. I did not search for a real collision.

## Suggested fix (Blue's call)

- Put more of the id in the memo before the freeze: the first 16 bytes (32 hex characters) is far from any collision and is still short. The memo's start stays `bounty ` (W-34 says readers match on its start); only what follows `bounty ` is longer. `sentFor` then matches the longer text, and still finds the memo with nothing after it.
- `used` should also hold the tids on the done file's offers, or `sentFor` should refuse a payment that is a tid of any recorded claim; the done file already has the tids in each line.

## Not tested

A real collision; the end-to-end command line path.

## Resolution

Fixed in BLUE-040 (D203). The memo is `bounty ` followed by `BountyMemoID`, the first 16 bytes of the offer's id in hex (32 characters). The frozen start (W-34) is unchanged; only what follows it is longer. `sentFor` matches on the longer text, so a 4-byte collision no longer matches. Reading the done file's tids into `used` was not taken: a chance match of 16 bytes needs about 2^64 offers.

Tests: `TestBlue040MemoID` (Red's case: ids sharing the first 4 bytes); `TestW34UnknownLegStateIsSent` (a 4-byte memo is not found).
