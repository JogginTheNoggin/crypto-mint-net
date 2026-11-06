# RED-047: `poon policy import -if-digest ""` applies the file unchecked, so an unset shell variable turns the protection off

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed |
| Found on | `main` @ 533dbcd (Source Round 18 Part B) |
| Component | pkg/cli/policyport.go (`policyImportCmd`) |
| Surface | local operator or script (CLI, GUI) |
| Fixed by | `claude/project-thread-ggo8dt` @ a133161 (BLUE-019) |

## Summary

`-if-digest` ties an apply to the file that was previewed. The flag is a
string with the default `""`, and the check is
`if *ifDigest != "" && !strings.EqualFold(*ifDigest, b.Digest)`. A flag given
as an empty string is therefore the same as no flag: `poon policy import f.json
-apply -if-digest "$DIGEST"` with `DIGEST` unset or empty (a failed
preview, a parse that returned nothing, a missing JSON field) applies the
file without the check, silently. The protection fails open at the one place
a script passes it.

## Reproduction

`TestRedIfDigestEmpty` (run in `pkg/cli`; a bundle holding the example
series, `init` of an empty root):

```
-apply -if-digest "":  err=<nil> saved=true
-apply -if-digest 00:  err=the file is not the one previewed (its digest is 741e...e28b, not 00), so nothing was changed: preview it again
```

## Impact

The check exists so a file swapped between preview and apply is not applied.
A caller that passes an empty digest gets none, and nothing tells it so. The
GUI (the main caller) is outside this report; whether it can pass an empty
value is for GUI/Red.

## Recommendation

Tell "not given" from "given empty": `fs.Visit` for the flag, and an empty or
non-hex-64 value is an error ("-if-digest needs the digest the preview
printed"). Also refuse `-if-digest` without `-apply`, which has nothing to
check.

## Resolution

Fixed (D152). `-if-digest` is detected with `fs.Visit`. When given, it
must be 64 hex digits, and it needs `-apply`; otherwise the import is
refused before anything is read or changed. Test:
`TestIfDigestEmptyIsRefused`.
