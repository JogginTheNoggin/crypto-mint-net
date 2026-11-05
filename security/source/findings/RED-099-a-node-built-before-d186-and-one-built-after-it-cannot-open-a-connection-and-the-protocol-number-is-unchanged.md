# RED-099: A node built before D186 and one built after it cannot open a connection, and the hello's protocol number is still 1

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | open |
| Found | Source Round 33 Part B, on `main` @ `941049d` (D186) |
| Area | `pkg/wire/messages.go` (`DecodeHello`), `pkg/wire/discovery.go` (`DecodePing`, `DecodePongReply`), `pkg/p2p/host.go` (`Protocol`) |
| For Black | Not a rule: no ledger decision depends on it. |

## What is wrong

D186 adds fields to the end of the hello (release, features, extensions)
and extensions to ping and pong. Every one is required: the decoders read
them and `finish` refuses trailing bytes. So:

- A D186 node reading a hello, ping or pong from an earlier build fails
  with `input too short`.
- An earlier build reading a D186 hello, ping or pong refuses the trailing
  bytes.

Either way the connection closes with a decode error, in both directions,
and `Protocol` is still `1`, so neither side can say "protocol not spoken"
or choose a format. The extension lists make later additions safe; this
change is the one that is not. No release has been published, so this
costs nothing today. It matters if any build from before D186 is in use
(the alpha testers' nodes), and for the version the release checklist
calls 0.1.0.

## How I checked it

`pkg/wire` test (throwaway, removed; copy kept with Red's notes):

```
old hello into new decoder: malformed message: hello: encoding: input too short
old ping into new decoder: malformed message: ping: encoding: input too short
new hello is 112 bytes; old one 43 (the old decoder reads 43 and refuses the rest)
```

The old decoder's refusal is read from `finish`, not run against an old
binary.

I also decoded 3,000,000 mutations of valid hellos, pings and pongs (bit
flips, truncations, appended bytes, out-of-order and repeated tags and
names, extension values of 1,024 bytes, release bytes of every kind):
no panic, nothing decoded that `Hello.Check` refuses, and every message
that decoded re-encoded to the same bytes.

## Suggested fix (Blue's call)

- Say in the release notes (and in WHITE-002) that builds from before D186
  do not connect, or let the new decoders take a body that ends before the
  new fields as "none" (so a D186 node still reads an old one; the old one
  still cannot read it).
- If any pre-D186 build must keep working, send the old hello to a peer that
  has not shown the new one, or move the new fields behind a hello
  `Protocol` 2 that an old node refuses by number.

## Not tested

Against a real pre-D186 binary.
