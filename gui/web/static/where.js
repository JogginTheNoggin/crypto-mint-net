// The node policy the views run commands against: -root and -policy, kept
// per browser. The terminal never adds them.
const KEY = 'poon-gui-where';
const THEME = 'poon-gui-theme';
const listeners = [];

function load(k) { try { return localStorage.getItem(k); } catch (_) { return null; } }
function save(k, v) { try { localStorage.setItem(k, v); } catch (_) { /* storage off */ } }

export function get() {
  try { return { root: '', policy: '', ...JSON.parse(load(KEY) || '{}') }; } catch (_) { return { root: '', policy: '' }; }
}

export function set(w) { save(KEY, JSON.stringify(w)); listeners.forEach(f => f(w)); }

export function onChange(f) { listeners.push(f); }

// flags are the flags views add to node and client commands.
export function flags() {
  const w = get();
  return [...(w.root ? ['-root', w.root] : []), ...(w.policy ? ['-policy', w.policy] : [])];
}

export function theme() { return load(THEME) || 'system'; }
export function setTheme(t) { save(THEME, t); applyTheme(); }
export function applyTheme() {
  const t = theme();
  if (t === 'system') document.documentElement.removeAttribute('data-theme');
  else document.documentElement.dataset.theme = t;
}
