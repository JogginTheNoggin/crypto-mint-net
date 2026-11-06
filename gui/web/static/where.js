// The node policy the views run commands against: -root and -policy, kept
// per browser. The terminal never adds them.
import * as store from './store.js';

const KEY = store.WHERE;
const THEME = store.THEME;
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

// showCommands says whether views show the poon command line behind each
// panel and form (with Run in terminal and Copy). It is off unless turned
// on in Settings: the views are the way to act, and the command lines are
// for someone who wants to learn or script them (jerry, R41). Hiding them
// changes nothing that runs.
const SHOW = store.SHOW_COMMANDS;
export function showCommands() { return load(SHOW) === '1'; }
export function setShowCommands(on) { save(SHOW, on ? '1' : '0'); applyCommands(); }
export function applyCommands() { document.documentElement.classList.toggle('show-cmds', showCommands()); }
