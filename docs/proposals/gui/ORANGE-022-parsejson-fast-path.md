# ORANGE-022: Let parseJSON's fast path skip hex strings

| Field | Value |
| --- | --- |
| Status | adopted (GUI Round 31 Part A) |
| Based on | main @ 0ade046 |
| Area | gui/web/static/render.js (`parseJSON`), gui/e2e/qol.mjs |
| Effort | small |
| Decided by | |

## Summary

Round 30 added `parseJSON` (GUI-GREEN-055). It keeps a whole number
above 2^53 exact as a BigInt, where `JSON.parse` would round it. Every
JSON result, the two series panels and every Watch event go through it.

To stay cheap, it calls `JSON.parse` at once when the text has no run of
16 digits, since a number above 2^53 has at least 16. But poon's hashes,
keys and ids are hex strings, and almost all of them hold 16 digits in a
row somewhere. So nearly every long answer took the slow path: a scan of
every character in JavaScript, then `JSON.parse`. That costs 3 to 7
times what `JSON.parse` costs alone.

This branch makes the fast path look only for 16 digits where a JSON
number can start: at the start of the text, or after a colon, bracket or
comma (spaces allowed). A hex string never matches that, so the common
answers go straight to `JSON.parse` again:

- `poon status` with 1,024 pools: 7 ms → 1 ms;
- `poon series followed` at 4,096 series: 20 ms → 9 ms;
- the 16,384-address peer book: 58–67 ms → 37–41 ms;
- 20,000 Watch events: 38 ms → 10–11 ms.

## Problem

The check was `/\d{16}/.test(text)`. A tid, a pool id, a series id or a
public key is 64 or more hex characters. In random hex, a run of 16
digits (0 to 9 only) turns up about once every 5,000 characters, so
any answer with a few hundred hashes has one. Every answer measured
below took the slow path, and none of them has a number above 2^53.

The slow path is correct. It is only slower than it needs to be, and
the Node view runs it on every refresh.

## Measurements

The probe served `gui/web/static` from main @ 0ade046 and loaded
`render.js` in Chromium through Playwright. It ran `JSON.parse`, the
`parseJSON` on main and the one on this branch on real poon output from
the probe nodes, five times each, and kept the best. "Slow path" says
whether main's check sent the text through the scan.

| Output | Size | Slow path on main | `JSON.parse` | `parseJSON` on main | This branch |
| --- | --- | --- | --- | --- | --- |
| `poon status`, reserve series (1,024 pools) | 352 KB | yes | 1 ms | 7 ms | 1 ms |
| `poon status`, 8,192 pools | 2.8 MB | yes | 2 ms | 14 ms | 7 ms |
| `poon series followed`, 4,096 series | 3.4 MB | yes | 4 ms | 20 ms | 9 ms |
| `poon peers`, 16,384 addresses | 12.8 MB | yes | 18 ms | 58 to 67 ms | 37 to 41 ms |
| 20,000 small Watch events, one call each | 4.6 MB in all | yes, most | 7 to 22 ms | 38 ms | 10 to 11 ms |

The new check is a regular expression over the whole text, which costs
0 to 13 ms on these outputs. That is the gap left between this branch
and `JSON.parse`.

Where this shows up in the page:

- **Node view, Each pool panel:** runs `poon status` once on opening. On
  the reserve series that is 7 ms of a 0.54 to 0.57 s settle.
- **Followed pools (5 s refresh):** since ORANGE-021 it runs
  `poon status -counts`, 629 B on the reserve series, so the parse is
  small either way. At 4,096 series the counts answer is still 910 KB,
  one row per series, every 5 s.
- **Watch at full rate:** one `parseJSON` per event. 20,000 events cost
  about 27 ms less.

## Idea

```js
const NUMBER16 = /(?:^|[:[,])\s*-?\d{16}/;
if (!NUMBER16.test(text)) return JSON.parse(text);
```

In JSON a number can only start the text or follow `:`, `[` or `,`,
with spaces between. So a number above 2^53 always matches, and a hex
string almost never does. When it does match (for example a string that
holds ", 1234567890123456"), the slow path runs and gives the same exact
answer as before. The check can only send text to the slow path more
often than needed, never less.

Nothing else changes: the slow path, the BigInt values and every caller
are as before. The GUI still shows what poon printed.

## Cost and risk

- Two lines in render.js.
- The risk would be a number above 2^53 that the check misses. The
  check matches every place JSON allows a number, so none is missed.
  qol.mjs checks a number after `[`, one after a colon and a newline
  in an indented answer, one at the start of the text, and a hex string
  that must stay a string.

## Prototype

Built on this branch. `gui/e2e/qol.mjs` keeps the GUI-GREEN-055 checks
and adds:

- `[1, 9007199254740993]` gives a BigInt;
- an indented `{"a": [18446744073709551615]}` gives a BigInt;
- ` 9007199254740993` on its own gives a BigInt;
- `{"tip": "0ac5991086712345678901234567890fe0", "h": 12}` parses with
  the hex string unchanged.

All ten e2e scripts pass.

## Modularity

Nothing to challenge.

## Consensus

None.
