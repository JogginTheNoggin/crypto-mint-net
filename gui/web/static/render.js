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

function scalar(v) {
  if (v === null) return el('span', 'nil', '—');
  if (typeof v === 'boolean') return el('span', v ? 'yes' : 'no', v ? 'yes' : 'no');
  if (isHex(v)) {
    // Long hex (hashes, addresses, keys) is shortened on screen; the full
    // value is in the tooltip and copied on click.
    const s = el('code', 'hex', `${v.slice(0, 10)}…${v.slice(-6)}`);
    s.title = `${v}\n(click to copy)`;
    s.tabIndex = 0;
    s.addEventListener('click', () => { navigator.clipboard && navigator.clipboard.writeText(v); s.classList.add('copied'); setTimeout(() => s.classList.remove('copied'), 800); });
    return s;
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
    for (const k of keys) tr.insertCell().append(k in r ? render(r[k], 2) : el('span', 'nil', ''));
  }
  const wrap = el('div', 'table-wrap');
  wrap.append(t);
  if (rows.length > MAX_ROWS) wrap.append(el('p', 'hint', `Showing ${MAX_ROWS} of ${rows.length} rows; the terminal shows all.`));
  return wrap;
}

// depth is how deep v is in the output; deep lists of rows start folded.
export function render(v, depth = 0) {
  if (!isObj(v) && !Array.isArray(v)) return scalar(v);
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
    dd.append(render(v[k], depth + 1));
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
