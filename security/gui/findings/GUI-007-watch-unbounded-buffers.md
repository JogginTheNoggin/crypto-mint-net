# GUI-007: Watch view's paused-event and error buffers grow without bound

| Field | Value |
| --- | --- |
| Severity | low |
| Status | open |
| Found on | claude/gui-4-watch @ 93ed36a |
| Component | gui/web/static/watch.js (`held`, `errOut`, `buf`) |
| Surface | local (browser tab) |
| Fixed by | _(pending — GUI/Blue)_ |

## Summary

The Watch view caps the visible event table at `MAX_EVENTS = 1000` rows (older
rows are removed), which is good. But three sibling buffers in the same view have
no cap, so a long-running `poo watch` against a busy node can grow the tab's memory
without limit:

1. **`held`** — while the view is paused, every arriving event is pushed to `held`
   (`if (paused) held.push(ev)`), and the SSE stream keeps running (pause only
   defers `add`, it does not stop following). A node emitting many events while the
   user leaves the view paused grows `held` unboundedly until Resume (or forever, if
   never resumed).
2. **`errOut`** — every stderr chunk and every unparseable line is appended to the
   `errOut` `<pre>` (`errOut.append(c.data)` / `errOut.append(text + '\n')`) with no
   cap. A node (or a corrupted/hostile stream) that produces a steady trickle of
   stderr or non-JSON lines grows this element without limit.
3. **`buf`** — the NDJSON line assembler (`buf += c.data; … buf = lines.pop()`)
   holds an unterminated trailing line. A stream that never emits a newline makes
   `buf` grow until one arrives. (Bounded indirectly by the 4 MiB per-job server cap,
   so this is the mildest of the three.)

## Reproduction

1. Start a node and open Watch with no filters against a pool with heavy traffic.
2. Click Pause and leave it; keep the node emitting events.
3. Watch the tab's memory climb as `held` accumulates every event with no ceiling;
   with enough events the tab slows or crashes. The visible table's 1000-row cap
   does not apply to `held`.
4. Separately, point Watch at a command/stream that writes many stderr or non-JSON
   lines and observe `errOut` grow without bound.

## Impact

A local, self-inflicted denial of service: the user's own browser tab can be driven
to exhaustion. No cross-origin or remote vector (the stream is the user's own node
via the loopback API, and rendering is `textContent` so there is no XSS even with
hostile event data — verified this round). Rated **low**: it degrades only the tab,
and only while the user watches a busy node, typically while paused.

## Recommendation

- Cap `held` the way the table is capped (keep at most `MAX_EVENTS`; drop oldest, and
  surface a "N events dropped while paused" notice like the existing `missed` line),
  or stop reading / tell the server to stop while paused.
- Cap `errOut` to a bounded tail (e.g. last N KiB or N lines) with a "truncated"
  marker.
- Optionally cap `buf` and treat an over-long unterminated line as a stream error.
- Test: with the view paused, feed > `MAX_EVENTS` synthetic events and assert
  `held`/DOM stay bounded; feed a large stderr volume and assert `errOut` is capped.

## Notes / what is already right

The visible table is correctly capped at 1000 rows, the server bounds per-job output
(4 MiB) and caps concurrent streams (`MaxStreams`, GUI-004 fix), and all event
rendering is `textContent`/`render()` so node- or peer-controlled event fields
(`origin`, `from`, `tid`, `kind`, …) cannot inject HTML. This finding is only about
the unbounded client-side accumulators.
