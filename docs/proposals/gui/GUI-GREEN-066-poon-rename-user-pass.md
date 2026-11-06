# GUI-GREEN-066: The poon rename, a user pass after Round 40

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | main @ a79be4b |
| Area | gui/server/server.go, web/static/api.js, perf/measure.mjs (the request header), views/bounty.js (the asked count), docs/white/gui/README.md, docs/gui.md, gui/e2e/qol.mjs, CHANGELOG.md |
| Effort | small |
| Decided by | (Grey) |

## Summary

Round 40 renamed poo to poon (D204). It also added what 0 means to Set limits, and the number of peers asked to Ask. I checked that the rename reads well and that nothing a person sees still says poo.

- **The page is clean.** I opened all 14 views on a running node and read their text, placeholders, titles and labels. None says poo. The tab reads "Getting started · poon GUI".
- **Outside the page, the old name is gone too:**
  - `poon-gui -h`, the start line ("poon-gui 0.1.0-alpha.1-dev: open …") and poon's help in the Terminal all say poon.
  - A mistyped command answers "poon: unknown command …; did you mean version?".
- **The code is clean too.** A whole-word search of `gui/` finds no `poo` and no `POO_`. The environment variables are `POON_*`, and the storage keys are `poon-gui-*`.
- **Two places were left:**
  - The page's request header was still `X-Poo-GUI`. A case-sensitive replace of the lowercase word misses it. The page and server ship together, and pkg/brand does not list it as frozen, so it can follow.
  - GUI White's charter (docs/white/gui/README.md) said `poo` 14 times, including checklist line 8's "`poo-gui-*`" storage keys. It is a living document. The dated baseline and round reports keep their name, as D204 says.
- **The new notes:**
  - Set limits now reads, for example, "0: no least price: offers at any price are kept".
  - Ask with no peer to ask gave both "Asked 0 peers for offers." and poon's note "no connected peer has the bounty market on, so no peer was asked; poon peers lists the connected peers". That says the same thing twice.

The node with a bounty book did not start on the renamed binary. See Source note 2.

## Idea (built here)

- **`X-Poon-GUI`** replaces `X-Poo-GUI` in the server, the page and the perf script. docs/gui.md is updated to match.
- **GUI White's charter** reads poon throughout, by whole-word replace. The labels pkg/brand keeps (`poo-rpc-hello`, `PoO-…`) do not appear in it.
- **The asked count shows only when it is above 0.** With none, poon's own note says so and what to check.

## For White and Grey (not built here)

1. **Settings do not survive the rename.**
   - The keys moved from `poo-gui-*` to `poon-gui-*`. The last-port file moved from `poo-gui/last-port` to `poon-gui/last-port`.
   - So a tester's GUI opens on a new port, which is a new browser origin. It forgets the wallet file, `-root`/`-policy` and terminal history.
   - This is what checklist lines 8 and 16 guard against. D204 accepts it because nothing is released.
   - jerry expects the name, the network name and the domain to change again. Before the alpha tag, the keys could be name-free (`gui-where`, `gui-wallet`, `gui-history`; they are per origin anyway). The last-port folder could also read the earlier name once, from a list in pkg/brand.

## For Source (not built here)

2. **A file written under the old name stops the node.**
   - The payer node from Round 39 would not start on a79be4b: "poon: …/bounties.json: not a poon-bounties file". The file is a bounty book; its kind is `poo-bounties`.
   - The message does not say that, or what to do. The same holds for every shared kind (`-ledger`, `-payment`, `-peers`, `-node-policies`, `-bounties-done`).
   - `~/.poo` is not read at all, so a tester's node seems to have vanished.
   - Before the next rename, either of these would help:
     - Reading kinds accepts the earlier names that pkg/brand lists.
     - The error names the old kind ("written as poo-bounties, by the name before D204").
3. **"0: any strength (255 is sent)".** The bracket is a wire detail a person cannot act on. "0: any strength" says it all.

## White's checklist, for this change

| Line | Answer |
| --- | --- |
| 2. CHANGELOG and FEATURES | A sentence on the GUI bullet under "Unreleased (0.1.0)"; FEATURES unchanged (no feature added) |
| 6. Only through pkg/cli | Unchanged |
| 8. Browser storage | Unchanged by this PR; see note 1 for the rename's own change |
| 9. `/api/` shapes | The header the page sends is renamed; page and server change in the same commit |
| 14. e2e | All 10 scripts pass locally on this commit |

## Cost and risk

A header name in three files, one condition in bounty.js, and doc words. No rule, consensus, policy or `/api/` path change.

## Prototype

This PR.
- qol.mjs checks that the asked count shows for 3 peers and not for 0.
- Every e2e script sends its changes through the page, so all 10 passing shows the renamed header is accepted.
