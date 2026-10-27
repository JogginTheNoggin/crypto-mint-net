# Source findings

## Summary

Red team findings against the headless node (everything outside `gui/`).
The format and the team loop are in [AGENTS.md](../../../AGENTS.md#teams-read-first).
One file per finding, `RED-<NNN>-<slug>.md`. This index lists them all.

| Id | Title | Severity | Status |
| --- | --- | --- | --- |
| [RED-001](RED-001-remote-dial-target-injection.md) | A peer can make the node dial arbitrary addresses (internal-scan / reflection) | medium | open |
| [RED-002](RED-002-missing-chunks-amplification.md) | Missing-chunks request resends chunks without dedup or per-chunk cost | low | open |
| [RED-003](RED-003-unverified-stated-weight-chase.md) | An unverified inbound peer can start chases and pull in honest priority peers | info | open |
