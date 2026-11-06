# GREEN-057: Doctor names unknown settings and a release mismatch

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `main` @ `6260590` |
| Area | `pkg/nodepolicy` (`UnknownDropped`), `pkg/node` (a start warning), `pkg/cli` (doctor: `policy` lines, a new `release` check), docs (nodepolicy, poon) |
| Effort | small |
| Consensus | none: what doctor and the start log say about node.json and the running release. Nothing is read, kept or saved differently |
| Decided by | Grey (Black may skip it; White may want to look, as it builds on W-1 and W-10) |

## Summary

Green ran D184 as a node runner:

1. Added three settings this release does not know to node.json:
   - `future_top` at the top level;
   - `p2p.future_inner` inside a section;
   - `series[0].future_in_list` inside a list entry.
2. Started the node. It warned about `future_top` and `p2p.future_inner`
   only.
3. Saved with `poon policy limits -set recent_window=20`. The two named
   settings were kept, and `series[0].future_in_list` was gone.
4. Read `release` in `poon status` and in `poon status -counts`, and
   checked the snapshot count fields.

Everything behaved as D184 describes. D184 does say unknowns inside list
entries are not kept. Three loose ends showed up:

1. **A setting the next save drops was never named.** The start warning
   lists only what is kept, so the one setting that will be lost got no
   word at all. A newer release that adds a field to a series, a follow
   rule or a grind job gets this silent loss on every rollback that
   saves.
2. **Doctor said nothing about unknown settings.** The warning is in the
   start log only. A misspelt key, such as `recent_windw`, is "kept but
   not used" just like a newer release's, so a typo can pass unnoticed.
3. **Doctor did not use `release`.** After an upgrade the node keeps
   running the old binary until it restarts. W-10 put the running
   release in status, and doctor is where a runner looks after
   upgrading.

## Idea

- **`Policy.UnknownDropped()`.** `Load` also lists the unknown settings
  inside list entries and map values, at any depth, by path:
  `series[0].x`, `follow[1].y`, or `MAP.KEY.z`. What is kept does not
  change.
- **The start log** adds a second warning: "node.json holds settings this
  release does not know inside list entries; the next save of node.json
  drops them", naming each one.
- **Doctor, `policy`.**
  - Kept settings get an ok line that lists them and says "a newer
    release wrote them, or they are misspelt".
  - Settings the next save drops get a warning. Its fix says to run the
    newer release, or to copy node.json aside before changing settings,
    or to correct a misspelt key.
- **Doctor, `release`.** This new check comes after `rpc`, when the node
  answers.
  - It is ok when the node's version and commit match this `poon`'s.
  - It warns when they differ: "the node runs 0.1.0-dev, but this poon
    is 0.1.1-dev; if this poon is the upgrade, restart the node".
  - It also warns when the node gives no release, which means a node
    from before W-10.
- **Docs.** nodepolicy.md, under W-1, and the `poon doctor` row in poon.md.

## How it was tested

- **`TestUnknownInsideListEntriesNamed`** (pkg/cli):
  - Setup: an example node.json with unknown keys in `follow[0]`, in
    `series[0]` and in `rpc`.
  - Expected: `UnknownDropped` names the two entry keys, and doctor
    gives an ok line for `rpc.future_rpc` and a warning for the two
    others.
  - It fails on `main`, where `UnknownDropped` does not exist.
- **`TestReleaseCheck`** (pkg/cli) covers three cases: the same release
  (ok), an older one (warn), and one the node does not give (warn).
- **By hand:**
  - The start log shows both warnings.
  - Doctor shows the two `policy` lines.
  - A `poon` built with `-X ...cli.Version=0.1.1-dev` warns on
    `release` against the running 0.1.0-dev node.
- **Checks run locally:**
  - gofmt, go vet and staticcheck;
  - `go test -race ./...`;
  - the nine GUI e2e scripts.

## Ideas for later (White's call)

- Keep unknown settings inside list entries too. Entries have no stable
  key, so the next save would have to match them by `id` (series), by
  series or issuer (follow rules), or by position (grind jobs). That
  choice belongs to White.
- The GUI's doctor view shows the new lines as they are. Whether it
  also compares its own release with the node's is GUI/White's call
  (D184).
