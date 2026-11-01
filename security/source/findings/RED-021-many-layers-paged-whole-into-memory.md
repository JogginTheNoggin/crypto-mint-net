# RED-021: A sender's many-layers entry is paged and counted whole, with no stop once the work beats the node's, at 15 times the memory it sends

| Field | Value |
| --- | --- |
| Severity | high |
| Status | open |
| Found on | `claude/m10-watch` @ ae8a103 (Source Round 8 Part B) |
| Component | pkg/p2p/chase.go (`askLayers`, `layersFrom`, `checkWork`), pkg/wire/messages.go (`LayersFrom`) |
| Surface | peers (a sender the node chases) |
| Fixed by | |

## Summary

Black's C-3 (D121) has the node page all the layers of an entry that holds
more than a reply carries whole: `askLayers` sees a many-layers reply,
`layersFrom` asks page after page until `More` is clear and returns every
nonce in one slice, and only then does `checkWork` hash each nonce and put
it into `st.counted` (a map from the layer hash) and add its credit to
`st.total`. Nothing stops the paging when the work counted already beats
`beat`, and nothing bounds what is held: the sender decides the number
(`ManyLayers.Count` and `LayersFrom.Total` are u32) and sends as many pages
as it likes. The nonces need only reach the series' `min_layer_strength`,
which is 0 for a series made with the defaults, where any 64-bit number is
a layer. The entry they sit on is a hash the sender listed, it need not exist.

The node holds 8 bytes a layer in `ns`, plus about 100 in `counted`, for
each 8 bytes on the wire. Before D121 `checkWork` stopped asking once
`st.total` beat `beat`, so what it held was bounded by the node's own weight
after the common entry. With the paging that bound is lost.

## Reproduction

A temporary test in `pkg/p2p` (not committed): node `a` with two entries; a
raw peer states a weight of 2^62 with a made-up tip, lists one made-up hash,
answers the layers request with a many-layers reply of 4,000,000 and answers
the layers-from request with the nonces 0 to 3,999,999 in 100,000-nonce
chunks:

```
layers sent 4,000,000 (wire 30 MB); heap base 9 MB, peak 473 MB
```

The sender's work was a loop and 30 MB of traffic; the node's heap grew by
464 MB, about 116 bytes a layer. At 100 million layers, 800 MB of traffic,
that is about 11 GB. The 4 million layers also kept the node's chase
busy and its CPU hashing for the length of the test. Each other sender (a
node id costs nothing to make) does the same on its own chase, and a
chase that then yields keeps `st.counted` in `h.prog` (up to 256 of them).

## Impact

High. A remote peer exhausts the memory of the node with traffic that is
fifteen times smaller than the memory it takes, with no work behind it
at the default strength, and at strength S it pays 2^S hashes a layer, which
is little for a small S. The same class as RED-015 and RED-016, which BLUE-008
fixed for the hash listing and for parts.

## Recommendation

Count the pages as they arrive and stop paging once the work counted beats
`beat`. This is the rule `checkWork` already had for hashes, and it
rejects nothing a heavier ledger needs: a ledger whose work beats the node's
is accepted for fetching as soon as that is shown, and what is not read
later is read from the entries themselves. So:

- have `layersFrom` take a callback (or return an iterator) that `checkWork`
  feeds with each page: hash, check the strength, add to `st.total`, and
  return "enough" once `st.total` is greater than `beat`;
- do not keep the nonces of an entry (the marks need the entry, not its
  nonces), and keep `st.counted` only for what is needed to avoid counting a
  nonce twice, which is then bounded by `beat` divided by the credit of the
  minimum strength;
- when the total is not enough after the last page, nothing is held that the
  node's own weight does not already bound.

Do not cap the number of layers or the pages: a branch can hold a great deal
of work on one entry, and a limit there would be a way to refuse a heavier
ledger. Suggestion for Black's view: the stop is the same decision as before
D121 (the work beats `beat`), so it adds no consensus rule and cannot cause a
fork.

Test: a raw sender with 4,000,000 nonces (as above); the node's heap stays
within a few MB of its base and the chase moves on to fetching or ends as
"not heavier" after about as many nonces as `beat` needs.

## Resolution

(Blue fills in.)
