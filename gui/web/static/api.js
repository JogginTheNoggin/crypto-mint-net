// The GUI's only way to do anything: run a poo command line on the server,
// which passes it to cli.Main exactly as the poo binary would.

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
// or windows for cmd.exe and poo.exe. viewOnly is true when every job
// runs in poo's view-only mode (poo-gui -view-only). loadInfo asks the
// server once. secretFlags are the poo flags whose value is a secret
// (shell.SecretFlags).
let grammar = 'posix';
export let viewOnly = false;
let secretFlags = ['restore', 'seed'];
export async function loadInfo() {
  try {
    const r = await fetch('/api/info');
    if (r.ok) ({ grammar, viewOnly, secretFlags = secretFlags } = await r.json());
  } catch (_) { /* keep posix, not view-only */ }
}

// secretLine reports whether a typed line may hold a secret: a secret
// flag such as -restore or -seed (except in poo sim, where -seed is a
// random-number seed). It errs toward true, since it only keeps a line
// out of saved history (GUI-006 in security/gui/findings).
export function secretLine(text) {
  const words = text.trim().split(/\s+/).map(w => w.replace(/^['"]+/, ''));
  if (words[0] === 'poo') words.shift();
  if (words.find(w => !w.startsWith('-')) === 'sim') return false;
  return words.some(w => secretFlags.some(f => new RegExp(`^--?${f}(=|$)`).test(w)));
}

// quote shows args as the poo command line the platform's shell would
// take, and that the terminal splits back into args. It is shell.Quote in
// gui/shell/shell.go, which the tests pin.
export function quote(args) {
  return ['poo', ...args.map(grammar === 'windows' ? quoteWindows : quotePOSIX)].join(' ');
}

function quotePOSIX(a) {
  return /^[A-Za-z0-9_\-.,\/:@%+=]+$/.test(a) ? a : `'${a.replaceAll("'", `'"'"'`)}'`;
}

function quoteWindows(a) {
  if (a !== '' && !/[ \t"&|<>^]/.test(a)) return a;
  // Backslashes before a quote, and at the end, are doubled.
  return '"' + a.replace(/(\\*)"/g, (_, b) => b + b + '\\"').replace(/(\\+)$/, '$1$1') + '"';
}
