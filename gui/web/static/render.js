// render turns a command's JSON output into tables and lists. It only
// lays out what the command printed: no value is computed or changed.
const MAX_ROWS = 500;

function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
}

const isHex = s => typeof s === 'string' && s.length >= 24 && /^[0-9a-f]+$/i.test(s);
const isObj = v => v !== null && typeof v === 'object' && !Array.isArray(v);

// LOOKUPS are the fields whose value Explore can look up. A value under
// one of these keys gets a link that opens Explore with it filled in, and
// Explore runs the same poo command its form shows.
const LOOKUPS = { tid: 'tid', address: 'address', to: 'address' };

export function lookupLink(key, v) {
  const param = LOOKUPS[key];
  if (!param || !isHex(v)) return null;
  const a = el('a', 'lookup', 'look up');
  a.href = `#/explore?${param}=${v}`;
  a.title = `Open Explore with this ${param === 'tid' ? 'transaction' : 'address'}`;
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
// poo printed in the tooltip.
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
  return el('span', typeof v === 'number' ? 'num' : '', String(v));
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
