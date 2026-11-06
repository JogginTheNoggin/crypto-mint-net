# GUI-001 — Launch secret leaks via process arguments and browser history, and never expires

- **ID:** GUI-001
- **Severity:** Medium (High on a shared/multi-user host; Low on a single-user desktop)
- **Status:** fixed (merged by GUI/Grey into claude/gui-5-release at fca4369, verified)
- **Fix branch:** `claude/gui-blue`
- **Component:** `gui/server/server.go` (`URL`, cookie swap), `gui/cmd/poon-gui/main.go` (`openURL`, `serve`)

## What

The whole GUI is gated by one 256-bit launch secret. The secret is handed to
the browser as a URL query token (`/?t=<secret>`) and is **also used verbatim
as the session cookie value** (`http.SetCookie{... Value: s.secret ...}`). That
one value is the master credential for the session, and it is exposed in
several places that outlive or escape the loopback boundary:

1. **Process arguments.** `serve` prints the full URL (secret included) and
   then `openURL` launches a browser with it:
   - `-app` mode: `exec.Command(browser, "--app="+url, "--new-window")` — the
     secret is an argv of the browser process.
   - default mode: `xdg-open <url>` / `open <url>` / `rundll32 ... <url>` —
     the secret is an argv of the opener (and usually propagates to the
     browser's own process args).
   Any other OS user can read these via `ps -ef`, `/proc/<pid>/cmdline`, or
   process-accounting logs. On a shared host this is a full compromise of the
   node: the observer pastes the URL and gets the cookie.

2. **Browser history.** `GET /?t=<secret>` is a top-level navigation, so the
   URL (with the secret) is written to history. The 303 redirect to `/` and
   `Referrer-Policy: no-referrer` stop the secret from leaking onward in
   referers, but history persists it on disk indefinitely.

3. **The terminal stdout** where poon-gui runs also contains the secret (needed
   to show the user the URL), so any capture of that output (tmux logs, CI
   logs, screen recordings, scrollback shipped in a bug report) leaks it.

4. **No expiry / single-use.** The `?t=` token equals the long-lived cookie
   value and is accepted for the entire lifetime of the process. It is never
   rotated and never invalidated after first use, so a secret recovered from
   any of the channels above grants access for as long as poon-gui runs.

## Why it matters

The loopback bind + secret is the GUI's entire authentication story. Channels
1–3 move that secret off the loopback boundary — into places a *different*
local principal (or a log pipeline) can read — and 4 means one glimpse is
enough for the rest of the session. That turns "local-only admin tool" into
"any local user / anyone who sees a log can control the node."

## Reproduce

- `poon-gui -app` (or default) on a multi-user box; from another shell run
  `ps -ww -ax | grep -- --app=` (or `tr '\0' ' ' </proc/<browser-pid>/cmdline`)
  and read the `?t=` secret. Open `http://127.0.0.1:<port>/?t=<secret>` → full
  access.
- After using the GUI, inspect browser history: the `/?t=<secret>` entry is
  present and still works.

## Suggested fix (for GUI/Blue — do not implement here)

- **Decouple the cookie from the secret.** Treat `?t=` as a one-time bearer
  token: on first successful swap, mint an *independent* random session id,
  store it server-side (or sign it), set that as the cookie, and immediately
  invalidate the launch token so the `?t=` URL stops working. History/ps then
  only ever expose a spent token.
- **Keep the secret out of argv.** Prefer opening the browser at a plain
  loopback URL and delivering the token another way:
  - write the token to a `0600` file in the user's runtime dir and have the
    page/opener read it, or
  - open `http://127.0.0.1:<port>/` and have poon-gui print a short-lived
    device-code the user pastes once, or
  - on Chromium `-app`, pass a profile/landing page rather than the secret in
    `--app=`.
- If a URL token must be used, scope it to a single use and a short TTL, and
  document that the printed URL is a secret (so operators don't paste it into
  tickets).

## Notes / what is already right

Entropy (32 bytes via `crypto/rand`), constant-time comparison
(`subtle.ConstantTimeCompare`), `HttpOnly`, `SameSite=Strict`, and
`Referrer-Policy: no-referrer` are all correct and should be kept. This
finding is about where the secret travels, not how it is compared.

## Fix (GUI/Blue, branch `claude/gui-blue`)

- `?t=` is now a one-time sign-in token with a time limit (`server.SignInURL`). The first visit spends it and sets a session cookie that is an independent random value, so the cookie is no longer the token. A spent or expired token gets a 403. The server stores only SHA-256 hashes of tokens and cookies.
- The address poon-gui opens itself lives for 1 minute and is not printed. The banner shows only the plain `http://127.0.0.1:PORT/`. With `-no-open`, poon-gui prints an address that lives for 5 minutes. Pressing Enter in poon-gui's window prints a fresh address, so expiry never locks a user out.
- Result: history, logs and scrollback only ever hold spent or expired tokens. The process-argv channel shrinks to the time between the browser starting and spending the token (usually under a second). This residual is documented in docs/gui.md (Security), with `-no-open` recommended on shared hosts. A file-based hand-off was considered and rejected, because snap-packaged browsers cannot read `/tmp` or dot-directories, so it would break the default open on Ubuntu.
- Kept: entropy, HttpOnly, SameSite=Strict, no-referrer.
- Regression tests: `TestSignIn`, `TestSignInTokenOneUse`, `TestSignInTokenExpires` (gui/server); `TestServeOpensAndStops`, `TestNoOpenPrintsOneUseAddresses` (gui/cmd/poon-gui); browser e2e check that a used address is refused in a second browser context (gui/e2e/terminal.mjs).
