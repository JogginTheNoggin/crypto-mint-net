# GUI-GREEN-061: A key kept off this machine, step by step

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | main @ 58273f7 |
| Area | gui/web/static/views/wallet.js (Keep the key off this machine, Pay's confirm, unsignedNote), views.js (form `dangerText`), style.css, gui/e2e/qol.mjs, CHANGELOG.md, docs/FEATURES.md |
| Effort | small |
| Decided by | (Grey) |

## Summary

Round 35 Part A added the cold-key forms (D193). I walked the whole flow
in the GUI on a real node (the worked example series), as someone who
keeps the key off the node's machine:

1. Make a key with a key file.
2. Create a watch-only wallet.
3. Record the key with Record a shared policy (`ed25519:PUB`).
4. Claim mint 5 with `poo claim -seed-file`, in the terminal, then Watch
   mint 5.
5. Pay 30 with a co-signing file.
6. Sign, with Show only first.
7. Submit.

Every step worked, and the sign line read right: "This key (address …)
added 1 signature to 1 transaction …; no more signatures are needed:
Submit a co-signed payment sends it." Two things made it hard to follow,
and one of them cost the units of the payment.

- **The steps are scattered.** The only place that lists them is the
  help under Watch-only, in the last of the view's 18 forms. The steps
  move between this machine and the key's machine, and nothing says which
  step runs where. The claim step is not in the GUI at all: the Wallet
  view's Claim a mint is refused for a watch-only wallet.
- **Pay with no co-signing file holds the units, and nothing can sign
  them.** On a watch-only wallet, Pay without Co-signing file ends with
  exit 0 and poo prints "signatures missing (per leg, per input): [[1]]
  / pass -out FILE …". But poo has already recorded the payment.
  - Balance went from 100 to 0.
  - Payments sent lists the leg as `unsigned`.
  - Pay again with a co-signing file says "not enough spendable units:
    0 held, 30 asked".
  - No command writes the file again or drops the payment.

  The Co-signing file field is optional and sits fifth in the form, so
  this is the easy mistake. The same has been true of a shared k-of-n
  policy since the wallet began (pkg/cli/wallet.go, `walletPay`). D193
  makes it the main way to pay.

## Idea

- **Keep the key off this machine.** This is a panel near the top of the
  Wallet view. The steps start folded under "Seven steps, from making the
  key to a payment sent". Each step says where it runs ("Where the key
  is" or "Here") and has a button that goes to its form. The first step
  is a link to Make a key in Series and keys.
  - The claim step says the claim is made where the key is, with `poo
    claim -seed-file`, since the wallet holds no key.
  - The Pay step says to give a co-signing file every time.
- **Pay's one confirm warns when Co-signing file is empty.** It still
  asks "Run poo wallet pay …?". When the field is empty it adds: "No
  co-signing file is filled in. If this wallet is watch-only, or pays
  from a shared policy, fill one in first: without it poo records the
  payment and holds its units, but writes no file for the key to sign."
  The GUI cannot tell which kind of wallet it is, so the warning says
  "if". It replaces the plain question, so there is still one dialog. To
  do this, a form can now pass `dangerText(values)`.
- **A line after the fact.** When poo printed "signatures missing" with no
  co-signing file, the result opens with: "Nothing was sent, and no file
  was written for the key to sign. The wallet has recorded this payment
  and holds its units: Payments sent lists it as unsigned. Next time,
  fill in Co-signing file before you press Pay."
- parity's TERMINAL_ONLY reason for `claim` now says a watch-only wallet
  claims this way, where the key is.

## For Source (not built here)

1. **Refuse before recording.** `poo wallet pay` with signatures missing
   and no `-out` should refuse before it records the payment, as it
   refuses on too few units. Otherwise it should give a way to write the
   file again or drop the payment, for example `poo wallet pay -resume
   TID -out FILE` and `poo wallet cancel TID`. Today the units stay held
   with nothing to sign, and I found no command that gives them back. This matters most for watch-only wallets, but a shared
   policy has the same problem.
2. **The watch-only error for claim says the wrong thing.** `poo wallet
   claim` on a watch-only wallet says "sign elsewhere, with poo sign, and
   submit the signed file". `poo sign` signs payments, not claims. The
   claim is made with `poo claim -seed-file` where the key is (D193). The
   same error on `wallet address` and `wallet seed` would read better as
   "it holds no private key".
3. **`poo wallet passwd` asks first, then refuses.** On a watch-only
   wallet it asks for a new passphrase, and only then refuses.
4. **Ideas:**
   - `poo keygen` could print the key as `ALG:HEX` (e.g. `"key":
     "ed25519:d404…"`), ready for `poo wallet policy -key`. Today a
     person joins `alg` and `public_key` by hand.
   - A `poo claim -out` that writes the claim for `poo sign`, as `wallet
     pay -out` does for a payment, would let a key that never touches a
     networked machine claim too. Today the claiming key must be on a
     machine that reaches a node.

## White's checklist, for this change

| Line | Answer |
| --- | --- |
| 2. CHANGELOG and FEATURES | A sentence on the GUI bullet under "Unreleased (0.1.0)", and the Wallet row in FEATURES.md |
| 6. Only through pkg/cli | Page only; no Go change |
| 8. Browser storage | Unchanged |
| 9. `/api/` shapes | Unchanged |
| 14. e2e | All 10 scripts pass locally on this commit |

## Cost and risk

About 50 lines in wallet.js, 3 in views.js and 4 in style.css. Pay keeps
one confirm, and its text is longer only when Co-signing file is empty.
No rule, consensus, policy or `/api/` change. The GUI reads only poo's
"signatures missing" line, and decides nothing about the wallet.

## Prototype

This PR. qol.mjs checks:
- Pay's confirm with no co-signing file carries the warning, and with one
  does not; a dismissed confirm runs nothing;
- `unsignedNote` gives the line only for exit 0 with "signatures missing"
  and no file;
- the steps start folded, there are seven, the first links to Series and
  keys, and a step's button focuses its form's heading.
