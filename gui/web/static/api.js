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
export function start(cmd) {
  return post('/api/jobs', typeof cmd === 'string' ? { line: cmd } : { args: cmd });
}

export function input(id, line) { return post(`/api/jobs/${id}/input`, { line }); }
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
// or windows for cmd.exe and poo.exe. loadGrammar asks the server once.
let grammar = 'posix';
export async function loadGrammar() {
  try {
    const r = await fetch('/api/grammar');
    if (r.ok) grammar = (await r.json()).grammar;
  } catch (_) { /* keep posix */ }
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
