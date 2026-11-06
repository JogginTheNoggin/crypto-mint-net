# GREEN-037: Say when a node.json change waits for a reload

| Field | Value |
| --- | --- |
| Status | adopted (D141) |
| Based on | `claude/m10-watch` @ `bc2e4bc` |
| Area | `pkg/nodepolicy` (`SameApplied`), `pkg/rpc` (`/v1/status`), `pkg/cli` (`doctor`), `docs/modules/rpc.md` |
| Effort | small |
| Consensus | none: it reports what the node runs and changes nothing it does |
| Decided by | Grey (Black may skip it) |

## Summary

Some `node.json` changes take effect at once: `poon grind schedule`, and
`follow`/`unfollow` without `-no-reload`, which reload the node. Others
wait for the next `poon reload` or start: `follow -no-reload` and any hand
edit (p2p, K, limits). Nothing told the person which state the node was
in. After `poon follow -series S -pool 1 -no-reload`, `poon follow` listed
pools `[0, 1]` from the file, while `poon pools` showed only pool 0, and
`poon doctor` said all was well.

The prototype has `GET /v1/status` add `policy_changed` when node.json
differs from the policy the node runs, ignoring the grind section, which
the node applies at once. `poon doctor`'s `rpc` check then warns
"... node.json has changed since it loaded it, and the change waits for a
reload", with fix `poon reload` (`fix_args` `["reload"]`, plus `-dir` per
GREEN-036). Prototyped, with tests.

## Idea

- `nodepolicy.Policy.SameApplied(q)` compares two policies as JSON with `Grind` set to nil. `Lowered` is `json:"-"`, and both sides are loaded the same way, so a lowered value never counts as a change.
- `rpc.status` loads node.json (on a load error it reports nothing) and sets `PolicyChanged = !file.SameApplied(n.Policy())`.
- `doctor`'s `rpc` check: `policy_changed` gives `warn` with the fix `poon reload`. Otherwise it reports `ok` as before.
- The GUI could show the same flag as a "reload to apply" banner. That is GUI/Grey's call.

## Testing

- `TestSameApplied` (`pkg/nodepolicy`): a grind change needs no reload, and a change of K does.
- `TestDoctorRunningNode` (`pkg/cli`): after `grind schedule add`, the check stays `ok`. After `unfollow -pool 1 -no-reload`, it warns with `reload -dir DIR`.
- By hand: the warning appears after `-no-reload` and goes away after `poon reload`.
- gofmt, vet, staticcheck, and `go test -race ./...` pass.

## Risks

`/v1/status` now reads node.json on each call. That is a small file, and
`/v1/follow` and the schedule already read it the same way. A hand edit
that only reformats the file is not a change, since both sides are
compared after loading.

## Smaller notes (not prototyped)

- `poon follow -mint 99` says "mint 99 out of range" without the range. "the series has mints 0 to 7" would say what to type. The same goes for `-pool 7`.
- `poon follow -pool 1 -mint 5` stores mint 5 even though pool 1 already covers it.
