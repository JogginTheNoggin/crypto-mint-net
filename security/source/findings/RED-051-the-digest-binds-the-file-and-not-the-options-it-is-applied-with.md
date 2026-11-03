# RED-051: `-if-digest` ties an apply to the file previewed but not to the options it was previewed with, so the same digest applies with `-settings`, `-full` or `-replace`

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed |
| Found on | `main` @ 4240d32 (Source Round 19 Part B) |
| Component | pkg/cli/policyport.go (`-if-digest`) |
| Surface | local operator or script (CLI, GUI) |
| Fixed by | `claude/project-thread-ggo8dt` @ HASH (BLUE-020) |

## Summary

The digest is the SHA-256 of the file. A preview prints it as "applies only
this file". But what an apply does depends as much on the options
(`-settings`, `-full`, `-replace`, `-prefer-file`) as on the file: the same
bundle previewed as a follow-only import (plan: three lines) applies, with
the same digest, as a `-full` import of every setting and the machine-local
ones. A script or the GUI that previews with one set of flags and applies
with another passes the check, and the person who read the plan approved
something else.

## Reproduction

By reading `policyImportCmd`: `digestGiven && !EqualFold(*ifDigest, b.Digest)`
is the only comparison, and `b.Digest` is `sha256(file bytes)`; `opt` is built
from the flags after it. Not run as a test.

## Recommendation

Make the digest cover what is applied: the file's bytes plus the options in
a fixed order (and, ideally, the plan: print the digest of the plan and have
`-apply -if-digest` recompute the plan and compare), so only what was
previewed can be applied. The preview already prints the digest "that
-apply -if-digest checks"; it should check that.

## Resolution

Fixed (D154). The digest `-if-digest` checks, and the preview prints
("import digest"), is now the SHA-256 of the file's digest, the merge
options, and the plan of every node policy by name, directory and mode.
`-apply -if-digest` plans first (as it already did, D150), computes the
digest from that plan, and refuses a difference before anything is saved.
So other options, another target, or a node.json changed since the
preview are all refused. Test: `TestIfDigestBindsOptionsAndPlan`.
