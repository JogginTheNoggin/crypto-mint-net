# RED-045: a node bundle that fails on its second policy has already applied its first, and the error names only the second

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed |
| Found on | `main` @ b17e892 (Source Round 17 Part B) |
| Component | pkg/cli/policyport.go (`policyImportCmd`, `importOne`); pkg/rpc/policyimport.go (body cap) |
| Surface | local operator (`poo policy import -apply` of a bundle of several policies) |
| Fixed by | `claude/project-thread-ggo8dt` @ HASH (BLUE-018) |

## Summary

`policyImportCmd` loops over the policies of a node bundle and calls
`importOne` for each in turn, applying each as it goes. The archived-name
check runs first for all, with the right message ("nothing was changed"),
but any other failure on a later policy (a damaged node.json, a locked
directory, a merge error) returns at that policy. The ones before it are
saved, nothing is printed about them (the plans are printed after the loop),
and the error names only the failing one.

## Reproduction

`TestRedNodeBundlePartialApply` (scratchpad `red_r17b_cli_test.go.txt`, goes
in `pkg/cli`): a node bundle with policies `a` (new) and `b` (its node.json
here is damaged), `poo policy import FILE -apply`:

```
import: err=node policy "b": nodepolicy: invalid: .../policies/b/node.json: invalid character 'b' ...
out=""
policy a was written: true
```

## Impact

The operator sees one error, assumes the import did nothing, and runs it
again after fixing `b`; the second run is safe (an import twice changes
nothing), so this is a misleading message, not a loss. For a backup restore
(`export -all` and back) it leaves the node half restored with no list of
which half.

## Recommendation

Plan every policy first (the dry path of `importOne`), and apply only if every
plan succeeded; otherwise return the error and say "nothing was changed". If
applying one by one is kept, print each result as it is done and make the
error say which policies were saved.

## Also

The CLI reads a bundle of up to `MaxBundleSize` (16 MiB) but the node's
`POST /v1/policy/import` takes a body of at most `MaxBody` (8 MiB). A bundle
between the two reads fine, is sent to the running node, and comes back as
`body: http: request body too large`, an `rpc.Error`, which the CLI returns
without falling back (`TestRedImportBodyCapsDiffer`, scratchpad
`red_r17b_rpc_test.go.txt`: 100,000 peers pass, 500,000 fail). Raise the
endpoint's cap to `MaxBundleSize` plus the options, or lower the CLI's.
Also untested (Windows only): `nameRE` allows a trailing dot and names like
`con` or `nul`, which Windows treats as an alias or a device.

## Resolution

Fixed (D150). `poo policy import` plans every node policy of the bundle
first, and applies only once every plan succeeded. A failure then says
"nothing was changed". If saving one fails part way (a lock, a disk), the
error names the node policies already saved and says that importing
again is safe. `POST /v1/policy/import` with the full token takes a body
of up to `rpc.MaxImportBody` (6 x 16 MiB + 8 MiB), so any bundle the CLI
reads fits even when encoded again. The Windows names are refused for a
new node policy and in a bundle (`checkNewName`: `con`, `prn`, `aux`,
`nul`, `com0-9`, `lpt0-9`, with or without an extension, and a trailing
`.`). One already saved under such a name still loads. Tests:
`TestNodeBundleImportsAllOrNothing`, `TestRunningNodeTakesALargeImport`
(pkg/cli), `TestBundleRefusesWindowsNames`.
