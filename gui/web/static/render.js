// render turns a command's JSON output into tables and lists. It only
// lays out what the command printed: no value is computed or changed.
const MAX_ROWS = 500;

function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
}

// parseJSON is JSON.parse that keeps every whole number exact. poon prints
// amounts, balances and totals as JSON numbers up to 2^64 - 1, and a
// plain JSON.parse rounds any above 2^53 (9,007,199,254,740,991): on the
// reserve series (10^14 units a mint) that is a balance of 91 mints.
// Such a number is read as a BigInt instead, which render prints digit
// for digit; every other value is JSON.parse's own (GUI-GREEN-055).
//
// Where the browser gives a reviver the number's source text (JSON.parse
// source text access, Chromium 114 on), the reviver reads the digits
// from it, and only numbers are ever changed. Elsewhere the numbers are
// swapped for marked strings first; the mark holds a random part made
// for each call, so no string poon printed can carry it (GUI/Red R30B: a
// fixed mark turned any string that began with it into a BigInt, or
// dropped the whole result to text).
const SOURCE = (() => { try { let ok = false; JSON.parse('1', (k, v, c) => { ok = !!c && c.source === '1'; return v; }); return ok; } catch (_) { return false; } })();
const bigOf = (k, v, c) => typeof v === 'number' && !Number.isSafeInteger(v) && c && /^-?\d+$/.test(c.source) ? BigInt(c.source) : v;
const NUMBER16 = /(?:^|[:[,])\s*-?\d{16}/;
export function parseJSON(text) {
  // Fast path: a number above 2^53 has at least 16 digits, and a JSON
  // number starts the text or follows a colon, bracket or comma. Any 16
  // digits would not do: poon's hashes and keys are hex strings with such
  // runs, so nearly every long answer took the slow path below, at 3 to
  // 4 times JSON.parse (ORANGE-022).
  if (!NUMBER16.test(text)) return JSON.parse(text);
  return SOURCE ? JSON.parse(text, bigOf) : parseMarked(text);
}

// parseMarked is parseJSON for a browser without source text access; it
// is exported for the e2e check, which runs on Chromium (which has it).
export function parseMarked(text) {
  const BIG = `\u0000big${Math.random().toString(36).slice(2)}${Date.now().toString(36)}:`;
  let out = '', last = 0, big = false;
  for (let i = 0; i < text.length;) {
    const c = text[i];
    if (c === '"') {
      // Skip a string, escapes included: digits in it are text.
      for (i++; i < text.length; i++) { if (text[i] === '\\') i++; else if (text[i] === '"') { i++; break; } }
      continue;
    }
    if (c === '-' || (c >= '0' && c <= '9')) {
      const start = i;
      for (i++; i < text.length && /[0-9.eE+-]/.test(text[i]); i++);
      const tok = text.slice(start, i);
      if (/^-?\d{16,}$/.test(tok) && !Number.isSafeInteger(Number(tok))) {
        out += text.slice(last, start) + JSON.stringify(BIG + tok);
        last = i; big = true;
      }
      continue;
    }
    i++;
  }
  if (!big) return JSON.parse(text);
  return JSON.parse(out + text.slice(last), (k, v) => typeof v === 'string' && v.startsWith(BIG) ? BigInt(v.slice(BIG.length)) : v);
}

const isHex = s => typeof s === 'string' && s.length >= 24 && /^[0-9a-f]+$/i.test(s);
const isObj = v => v !== null && typeof v === 'object' && !Array.isArray(v);

// LOOKUPS are the fields whose value the Ledger view can open. A value
// under one of these keys gets a link that opens it there (R41: it was
// Explore's form, filled in; Explore still takes ?tid= and ?address=).
const LOOKUPS = { tid: 'tid', address: 'address', to: 'address' };

export function lookupLink(key, v) {
  const param = LOOKUPS[key];
  if (!param || !isHex(v)) return null;
  const a = el('a', 'lookup', 'look up');
  a.href = `#/ledger?${param}=${v}`;
  a.title = `Open this ${param === 'tid' ? 'transaction' : 'address'} in the Ledger view`;
  return a;
}

// secretValue shows a secret, such as a seed, hidden until Show is
// pressed, and never copies it on click (GUI-009 in security/gui/findings).
export function secretValue(v) {
  const wrap = el('span', 'secret');
  const text = el('code', 'secret-text', '••••••••');
  const btn = el('button', 'small', 'Show');
  btn.type = 'button';
  btn.setAttribute('aria-pressed', 'false');
  btn.addEventListener('click', () => {
    const show = btn.getAttribute('aria-pressed') !== 'true';
    text.textContent = show ? String(v) : '••••••••';
    btn.textContent = show ? 'Hide' : 'Show';
    btn.setAttribute('aria-pressed', String(show));
  });
  wrap.append(text, ' ', btn);
  return wrap;
}

// rateText shows tries a second the way people read them; the Work view's
// live counter and every printed "rate" field use it, with the exact value
// poon printed in the tooltip.
export function rateText(r) {
  if (!r) return '0 H/s';
  for (const [n, u] of [[1e9, 'G'], [1e6, 'M'], [1e3, 'k']]) if (r >= n) return `${(r / n).toFixed(2)} ${u}H/s`;
  return `${r.toFixed(0)} H/s`;
}

function scalar(v, key) {
  if (v === null) return el('span', 'nil', '—');
  if (key === 'rate' && typeof v === 'number') {
    const s = el('span', 'num', rateText(v));
    s.title = `${v} tries a second, as printed`;
    return s;
  }
  if (key === 'seed' && typeof v === 'string' && v) return secretValue(v);
  if (typeof v === 'boolean') return el('span', v ? 'yes' : 'no', v ? 'yes' : 'no');
  if (isHex(v)) {
    // Long hex (hashes, addresses, keys) is shortened on screen; the full
    // value is in the tooltip and copied on click.
    const s = el('code', 'hex', `${v.slice(0, 10)}…${v.slice(-6)}`);
    s.title = `${v}\n(click to copy)`;
    s.tabIndex = 0;
    // Copying can be refused (no clipboard permission); then the full value
    // is still in the tooltip, and nothing is reported as copied.
    s.addEventListener('click', () => {
      if (!navigator.clipboard) return;
      navigator.clipboard.writeText(v).then(() => {
        s.classList.add('copied');
        setTimeout(() => s.classList.remove('copied'), 800);
      }, () => { s.title = `${v}\n(copying was refused by the browser)`; });
    });
    const link = lookupLink(key, v);
    if (!link) return s;
    const span = el('span', 'hex-wrap');
    span.append(s, ' ', link);
    return span;
  }
  return el('span', typeof v === 'number' || typeof v === 'bigint' ? 'num' : '', String(v));
}

function table(rows) {
  const keys = [];
  for (const r of rows.slice(0, MAX_ROWS)) for (const k of Object.keys(r)) if (!keys.includes(k)) keys.push(k);
  const t = el('table', 'data');
  const head = t.createTHead().insertRow();
  for (const k of keys) head.append(el('th', '', k.replaceAll('_', ' ')));
  const body = t.createTBody();
  for (const r of rows.slice(0, MAX_ROWS)) {
    const tr = body.insertRow();
    for (const k of keys) tr.insertCell().append(k in r ? render(r[k], 2, k) : el('span', 'nil', ''));
  }
  const wrap = el('div', 'table-wrap');
  wrap.append(t);
  if (rows.length > MAX_ROWS) wrap.append(el('p', 'hint', `Showing ${MAX_ROWS} of ${rows.length} rows; the terminal shows all.`));
  return wrap;
}

// depth is how deep v is in the output; deep lists of rows start folded.
// key is the field v was printed under, if any.
export function render(v, depth = 0, key = '') {
  if (!isObj(v) && !Array.isArray(v)) return scalar(v, key);
  if (Array.isArray(v)) {
    if (v.length === 0) return el('span', 'nil', 'none');
    if (v.every(isObj)) return depth >= 2 && v.length > 3 ? collapsed(`${v.length} items`, () => table(v)) : table(v);
    const ul = el('ul', 'list');
    for (const x of v.slice(0, MAX_ROWS)) { const li = el('li'); li.append(render(x, depth + 1)); ul.append(li); }
    // Like a table, a long list says it was cut: a part's serials, say,
    // must not seem to end at the 500th (ORANGE-018).
    if (v.length > MAX_ROWS) { const w = el('div'); w.append(ul, el('p', 'hint', `Showing ${MAX_ROWS} of ${v.length} items; the terminal shows all.`)); return w; }
    return ul;
  }
  const keys = Object.keys(v);
  if (keys.length === 0) return el('span', 'nil', 'none');
  const dl = el('dl', 'kv');
  for (const k of keys) {
    dl.append(el('dt', '', k.replaceAll('_', ' ')));
    const dd = el('dd');
    dd.append(render(v[k], depth + 1, k));
    dl.append(dd);
  }
  return dl;
}

function collapsed(label, make) {
  const d = el('details');
  d.append(el('summary', '', label));
  d.addEventListener('toggle', () => { if (d.open && d.children.length === 1) d.append(make()); });
  return d;
}
