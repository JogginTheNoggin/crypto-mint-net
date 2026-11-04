# RED-073: A release in node.json that does not check makes the node follow nothing of the series, and `add-release` will not replace it

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | open |
| Found | Source Round 25 Part B, on `main` @ `e93eb29` (D166) |
| Area | `pkg/nodepolicy` (`Accepted`, `AddRelease`, import merge), `pkg/cli` (`series add-release`, `policy import`, `doctor`) |
| For Black | No consensus rule is touched. |
| Fixed by | |

## What is wrong

`Accepted()` builds a variable series and then runs `AddRelease(*si.Release)`
on the kept release. If that fails, the **whole series** is refused
(`refused = append(refused, ...)`), including part 1, which the issuer
witness releases. A release is information the node was given; one that does
not check should be ignored, not make the node drop what it knows.

`Policy.AddRelease` then checks the kept release first and returns
"the release node.json keeps: ...", so the command meant to give the node a
good release refuses while a bad one is kept.

## How I checked it

1. A policy file with a forged release (last bytes of the witness changed)
   for a series the node does not list yet:

```
$ poo policy import -dir n5 pexp_forged.json
  + series: 8dfdf9a3
  warning: series 8dfdf9a3: release: ... signature for key 0; it is listed but unused until accepted (issuers)
$ poo policy import -dir n5 -apply -no-reload pexp_forged.json      -> saved
```

   The plan warns, `-apply` goes on, and node.json has the series with the
   bad release. The same state follows from a hand edit or a damaged file.

2. A node started on that node.json (with a follow rule on part 1):

```
node: warning: series not accepted index=0 err="release: series: invalid release: witness: ..."
node: warning: follow rule names a series not known or not accepted follow=0 series=8dfdf9a3
node: started dir=n3 pools=0 ...
```

   It runs, listens and answers, following no pool. `poo doctor`: "warn
   series 0 accepted, 1 not ... fix: correct or remove it in the policy".

3. The repair the docs point to does not work:

```
$ poo series add-release -dir n5 rel3.json
poo: the release node.json keeps: series: invalid release: witness: ... signature for key 0
```

   The owner has to edit node.json by hand, or remove the series and lose
   its follow rules.

(A merge import of a file whose release is bad for a series the node already
lists changes nothing: the merge does not carry releases, which is safe.)

## What could happen

A shared policy file, a damaged node.json or a hand edit takes a node out of
a variable series without a stop: it runs, with a warning in the log. The
owner cannot fix it with `add-release`.

## Suggested fix (Blue's call)

- In `Accepted()`, treat a kept release that fails as absent: follow the
  series with the parts proven so far (part 1), report it as a warning with
  the reason, as `refused` does today.
- In `Policy.AddRelease`, ignore a kept release that fails, and replace it.
- `policy import` should drop (and name) a release that does not check
  instead of saving it, as it refuses a series whose params do not build.
- A test beside `TestVariableFollowed`: a bad kept release leaves the series
  followed at part 1, and `AddRelease` of a good one replaces it.
