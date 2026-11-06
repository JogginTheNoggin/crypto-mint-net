# RED-122: The old `POO_` environment variables are ignored without a word, and `POO_VIEW_ONLY=1` now turns view-only off

| Field | Value |
| --- | --- |
| Severity | Low (it needs a person or script that set the old variable; it fails open for the one that is a safety switch) |
| Status | open |
| Found | Source Round 40 Part B, on `main` @ `46b638d` (D204, the rename to `poon`) |
| Area | `pkg/brand/brand.go` (`EnvPrefix`), `pkg/cli/scope.go` (`viewOnlyEnv`), `pkg/cli/node.go` (`defaultRoot`, seed), `pkg/cli/wallet.go` (passphrase) |
| Fixed by | |
| For Black | Not a rule. No consensus byte is involved. |

## What is wrong

D204 renames every variable the tool reads, `POO_*` to `POON_*`, and reads no old name. A person or a script that has `POO_VIEW_ONLY`, `POO_HOME`, `POO_PASSPHRASE`, `POO_NEW_PASSPHRASE` or `POO_SEED` set gets no message that they are no longer read. What happens depends on the variable:

- `POO_VIEW_ONLY=1`: **not read, so the command runs with full powers.** The variable is how a shell, a service or a login profile makes every `poo` command only look (D101). After the rename the same profile runs `poon init`, `poon pay`, `poon node` and the rest as before the switch existed. This one fails open.
- `POO_HOME=/path`: not read, so the root is `~/.poon`. A `poon init` or `poon node` then starts a second, empty node policy there, and `~/.poo` or the old `POO_HOME` is left unread. The person sees "wrote ~/.poon/…" and may not notice it is not their node.
- `POO_PASSPHRASE`, `POO_SEED`, `POO_NEW_PASSPHRASE`: not read. A command with a terminal asks for the secret; one with no terminal fails with "no passphrase: set POON_PASSPHRASE …". Loud, and the message names the new variable. One thing I did not run: a script that pipes other input on standard input may have `readSecret` take its first line as the passphrase, because the variable that used to answer it is gone.

## How I checked it

Built `./cmd/poon` at `46b638d`, empty `HOME` each time:

```
$ POO_VIEW_ONLY=1 poon init
wrote …/.poon/policies/default/node.json (1 series)
following series fe52c1ac…f139, the built-in default series

$ POON_VIEW_ONLY=1 poon init
poon: view-only: poon init changes things
```

The first wrote the policy; the second was refused. I did not run the `POO_HOME` and passphrase cases; they are from the code (`os.Getenv(brand.EnvPrefix + …)`; no other name is read anywhere: `grep -rn "Getenv\|LookupEnv"` finds six places, all through `brand.EnvPrefix`). `go vet ./...` and `go test ./pkg/...` pass at `46b638d`.

## Suggested fix (Blue's call)

Cheap and until the first public tag only:

- Read the old names for one release, only to warn: if any `POO_*` variable is set and its `POON_*` is not, print one line to standard error: "POO_VIEW_ONLY is set; poon reads POON_VIEW_ONLY now".
- For `POO_VIEW_ONLY` with a true value, do not ignore it: treat the command as view-only and say so (fail closed), or refuse to run until it is renamed.
- When `~/.poo` exists and `~/.poon` does not (and `POON_HOME` is unset), say so once: "found ~/.poo from an earlier name; move it to ~/.poon to keep that node".
- Or decide no one has these yet (nothing is public) and note it in `CHANGELOG.md`; then `POO_VIEW_ONLY` alone is worth the guard.

## Not tested

The `POO_HOME`, passphrase and seed cases; the GUI path (`poon-gui` passes `POON_*` the same way).
