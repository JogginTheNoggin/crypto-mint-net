// The GUI's only way to do anything: run a poon command line on the server,
// which passes it to cli.Main exactly as the poon binary would.

const HEADERS = { 'Content-Type': 'application/json', 'X-Poo-GUI': '1' };

async function post(path, body) {
  const r = await fetch(path, { method: 'POST', headers: HEADERS, body: body === undefined ? undefined : JSON.stringify(body) });
  if (!r.ok) {
    let msg = r.statusText;
    try { msg = (await r.json()).error || msg; } catch (_) { /* not JSON */ }
    throw new Error(msg);
  }
  return r.status === 204 ? null : r.json();
}

// run runs args to the end: {code, out, err, command}.
export function run(args) { return post('/api/run', { args }); }

// start starts a job from args or a typed line: {id, args, command}.
// refresh marks a view re-running a command on a timer, whose finished
// jobs are dropped before the user's own.
export function start(cmd, { refresh = false } = {}) {
  return post('/api/jobs', { ...(typeof cmd === 'string' ? { line: cmd } : { args: cmd }), ...(refresh ? { refresh } : {}) });
}

// A line is sent in parts of at most PART characters, each a request well
// under the server's 1 MiB body limit, so a pasted entry of any size
// reaches the command whole (ORANGE-013). The parts go in order, the last
// one ending the line.
const PART = 256 << 10;
export async function input(id, line) {
  let i = 0;
  for (; line.length - i > PART; i += PART) await post(`/api/jobs/${id}/input`, { line: line.slice(i, i + PART), more: true });
  return post(`/api/jobs/${id}/input`, { line: line.slice(i) });
}

// inputFile sends a file picked in the browser to a job's input as it is
// read, PART bytes at a time, then ends the input. Only what the command
// reads is read: once it stops (it refused the file, or ended), the next
// send fails and nothing more of the file is read (ORANGE-014). Text split
// inside a character is joined again by the streaming decoder.
export async function inputFile(id, file) {
  const dec = new TextDecoder();
  for (let off = 0; off < file.size; off += PART) {
    const text = dec.decode(await file.slice(off, off + PART).arrayBuffer(), { stream: true });
    await post(`/api/jobs/${id}/input`, { line: text, more: true });
  }
  const rest = dec.decode();
  if (rest) await post(`/api/jobs/${id}/input`, { line: rest, more: true });
  return eof(id);
}
// eof ends a job's input, as Ctrl-D does in a shell; cancel is Ctrl-C,
// and is never read as an end of input (GUI-021).
export function eof(id) { return post(`/api/jobs/${id}/eof`); }
export function cancel(id) { return post(`/api/jobs/${id}/cancel`); }

export async function jobs() {
  const r = await fetch('/api/jobs');
  if (!r.ok) throw new Error(r.statusText);
  return r.json();
}

// follow streams a job's events to handlers {chunk, dropped, state, exit}.
// It returns a function that stops following (the job keeps running).
export function follow(id, h, from = 0) {
  const es = new EventSource(`/api/jobs/${id}/events?from=${from}`);
  for (const ev of ['chunk', 'dropped', 'state']) {
    es.addEventListener(ev, e => h[ev] && h[ev](JSON.parse(e.data)));
  }
  es.addEventListener('exit', e => { es.close(); h.exit && h.exit(JSON.parse(e.data)); });
  es.onerror = () => { if (es.readyState === EventSource.CLOSED && h.error) h.error(); };
  return () => es.close();
}

// grammar is how the server splits a typed line (shell.Native): posix,
// or windows for cmd.exe and poon.exe. viewOnly is true when every job
// runs in poon's view-only mode (poon-gui -view-only). loadInfo asks the
// server once. secretFlags are the poon flags whose value is a secret
// (shell.SecretFlags).
let grammar = 'posix';
export let viewOnly = false;
let secretFlags = ['restore', 'seed'];
// version is the release poon-gui and poon share (cli.Version), empty when
// the server did not say (GUI White G4, GUI-GREEN-058).
export let version = '';
export async function loadInfo() {
  try {
    const r = await fetch('/api/info');
    if (r.ok) ({ grammar, viewOnly, secretFlags = secretFlags, version = '' } = await r.json());
  } catch (_) { /* keep posix, not view-only */ }
}

// secretLine reports whether a typed line may hold a secret: a secret
// flag such as -restore or -seed (except in poon sim, where -seed is a
// random-number seed). It errs toward true, since it only keeps a line
// out of saved history (GUI-006 in security/gui/findings).
//
// A secret flag whose value is - (standard input) holds no secret: poon
// asks for it in a masked field, so poon wallet create -restore - is not
// one (GUI-GREEN-047).
//
// A key file flag (-key-file, -seed-file) takes a path, which is no
// secret; but a key typed where the path goes is, so a line whose value
// looksLikeKey is kept out too (GUI-034). The value may be split into
// words (a quoted key printed in groups), so each run of words after the
// flag is tried.
export function secretLine(text) {
  const words = text.trim().split(/\s+/).map(w => w.replace(/^['"]+|['"]+$/g, ''));
  if (words[0] === 'poon') words.shift();
  if (words.find(w => !w.startsWith('-')) === 'sim') return false;
  if (words.some((w, i) => secretFlags.some(f => {
    const m = new RegExp(`^--?${f}(=(.*))?$`).exec(w);
    return m && (m[1] !== undefined ? m[2] !== '-' : words[i + 1] !== '-');
  }))) return true;
  return words.some((w, i) => {
    const m = /^--?(key-file|seed-file)(=(.*))?$/.exec(w);
    if (!m) return false;
    // A key's hex run, once there, stays as words are added, until a word
    // that marks a path or a file (/ \ or .); so the value is read up to
    // that word, or KEY_RUN characters, and tested once without it and
    // once with it (a key may end in a dot). A long line costs time in
    // proportion to its length (GUI/Red R38B).
    let v = '';
    for (let j = m[2] !== undefined ? i : i + 1; j < words.length; j++) {
      const x = j === i ? m[3] : words[j];
      const w = v ? `${v} ${x}` : x;
      if (v && w.length > KEY_RUN) break; // the first word is always tested (GUI/Red R39B)
      if (/[/\\.]/.test(x)) return looksLikeKey(v) || looksLikeKey(w);
      v = w;
    }
    return looksLikeKey(v);
  });
}

// KEY_RUN is the most characters after a key file flag that secretLine
// reads as one value: far past any key written with separators (an
// SLH-DSA-256s secret key is 128 bytes, 383 characters as colon hex).
const KEY_RUN = 512;

// looksLikeKey says s, given where a path goes, is a key: 32 or more hex
// digits in a row once the separators a key is often printed with are
// taken out (colons as openssl prints it, spaces, dashes, a dot at the
// end), and no / or \ or other dot in it (GUI-032). The Wallet view's
// sign form, the terminal's history (secretLine) and poon-gui's
// shell.LooksLikeKey use the same test.
export function looksLikeKey(s) {
  if (typeof s !== 'string') return false;
  const t = s.trim().replace(/\.+$/, '');
  return !/[/\\.]/.test(t) && /[0-9a-fA-F]{32,}/.test(t.replace(/[\s:-]/g, ''));
}

// quote shows args as the poon command line the platform's shell would
// take, and that the terminal splits back into args. It is shell.Quote in
// gui/shell/shell.go, which the tests pin.
export function quote(args) {
  return ['poon', ...args.map(grammar === 'windows' ? quoteWindows : quotePOSIX)].join(' ');
}

// copyLine is the line Copy puts on the clipboard, to be pasted into a
// shell, or null when it cannot be quoted safely. On posix it is quote's.
// On Windows the paste goes to PowerShell or cmd.exe, which do not split
// as CommandLineToArgvW does (quoteWindows): PowerShell runs $(...) and
// ; in a bare word, and cmd.exe ends a quoted word at \" and runs what
// follows & (GUI-024). No quoting means the same to both, so a word is
// copied only from a set both take literally, double-quoted when it has
// a space, and any other word turns Copy off for the line.
export function copyLine(args) {
  if (grammar !== 'windows') return quote(args);
  const words = args.map(copyWordWindows);
  return words.includes(null) ? null : ['poon', ...words].join(' ');
}

function copyWordWindows(a) {
  // Letters, digits, _ - . : / \ = + and space only: none of these is
  // special to either shell outside quotes, except the space, which
  // double quotes keep, unless a \ ends the word (CommandLineToArgvW reads
  // \" as a quote). An empty word is refused, as PowerShell may drop "".
  if (!/^[A-Za-z0-9_\-.:\/\\=+ ]+$/.test(a)) return null;
  if (!a.includes(' ')) return a;
  return a.endsWith('\\') || a.trim() !== a ? null : `"${a}"`;
}

function quotePOSIX(a) {
  return /^[A-Za-z0-9_\-.,\/:@%+=]+$/.test(a) ? a : `'${a.replaceAll("'", `'"'"'`)}'`;
}

function quoteWindows(a) {
  if (a !== '' && !/[ \t"&|<>^]/.test(a)) return a;
  // Backslashes before a quote, and at the end, are doubled.
  return '"' + a.replace(/(\\*)"/g, (_, b) => b + b + '\\"').replace(/(\\+)$/, '$1$1') + '"';
}
