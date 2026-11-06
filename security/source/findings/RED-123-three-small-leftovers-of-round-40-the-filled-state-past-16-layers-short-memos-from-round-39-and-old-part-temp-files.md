# RED-123: Three small leftovers of Round 40: the filled state past 16 layers, short memos from Round 39, and old part temp files

| Field | Value |
| --- | --- |
| Severity | Info (each is cosmetic or needs an old file; none costs anything by itself) |
| Status | open |
| Found | Source Round 40 Part B, on `main` @ `46b638d` (D203, D204) |
| Area | `pkg/bounty/book.go` (`Acked`, `MaxAcks`), `pkg/cli/bounty.go` (`sentFor`, `BountyMemoID`), `pkg/daemon/daemon.go` and `pkg/p2p/chase.go` (`TempPrefix`) |
| Fixed by | |
| For Black | Not a rule. |

## 1. A held offer of more than 16 layers never reads filled (GUI-GREEN-065)

`Acked` marks a held offer filled when the retained acks hold `Count` distinct accepted nonces. The acks kept are bounded by `MaxAcks` (16, RED-113), and `Count` is up to 65,535. For `Count` over 16 the set can never be large enough, so a grinder that was accepted for every layer sees the offer as open.

Book-level test (throwaway, removed): `Count` N, then N accepted acks with distinct nonces, then the state:

```
count 3,  3 accepted: filled
count 16, 16 accepted: filled
count 17, 17 accepted: open
count 40, 40 accepted: open
```

Display only: the payer still answers `filled`. Suggested: count accepted acks with a small counter on the held offer (`Accepted uint16`, nonces kept only to avoid counting one twice), or note that the state is exact only up to 16.

## 2. Payments made in Round 39 carry the 8-character memo and are no longer found

D203 puts 16 bytes (32 characters) of the id in the memo, and `sentFor` matches `"bounty " + BountyMemoID(id)`. A payment made by `main` @ `fa8dad7` has `"bounty " + Short()` (8 characters), which does not start with the longer text. An unrecorded payment of that kind (RED-119's case) is not found, and `poon bounty pay` pays the claim again. It needs a wallet from before the rename; nothing is public yet. Not run; from the code.

## 3. `poo-parts-*` temp files are never swept

`daemon.go` removes leftover `TempPrefix+"parts-*"` files at start, and `TempPrefix` is now `poon-`. A part temp file left by a crash under the old name (the chase writes them in `PartsDir`) is not matched and stays. They are up to a part's size each. Only a crash before the rename leaves one. Not run; from the code.
