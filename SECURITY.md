# Security policy

## Supported versions

Only the latest release gets security fixes until 1.0.

## Reporting a vulnerability

Please do not open a public issue. Use GitHub's private vulnerability
reporting on this repository (Security tab, "Report a vulnerability"), and
include the version or commit, what an attacker can do, and how to
reproduce it. The maintainers will answer as soon as they can.

A bug that lets anyone make a node accept an invalid transaction, entry or
layer, crash a node with bytes from the network, or learn a wallet's seed
is the most serious kind. So is any way for two honest nodes to disagree
about the same bytes.

## What has been reviewed

[docs/security.md](docs/security.md) records the review before the first
release: the threat model, what was checked, what was fixed, and what is
still open. The code has not yet been reviewed by anyone outside the
project.
