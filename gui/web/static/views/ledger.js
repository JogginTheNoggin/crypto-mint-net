// The Ledger view: what the node holds, to look through by clicking, with
// no command to build (jerry, R41: "i cannot see or navigate the ledger").
//
// It starts at the pools the node follows. A pool opens its selected
// ledger a page at a time, newest first; an entry opens its transactions;
// a transaction opens where it stands, what it spent and what it paid; an
// address opens its balance, holdings and history. Where it is shows in
// the address bar (#/ledger?pool=1&height=3 and so on), so Back works and a
// place can be linked to.
//
// Every screen is poon pools, poon ledger, poon pending, poon tx, poon
// unspent or poon address run once (api.run), laid out. Nothing here computes anything
// about the ledger: the only arithmetic is which height the next page
// starts at. The command lines each screen ran are shown under it when
// Settings shows commands.
import * as api from '../api.js';
import { el, cmdLine, needsNode, rowsTable } from '../views.js';
import { parseJSON, render } from '../render.js';
import * as where from '../where.js';
import { ledgerPicture } from './watch.js';

// PAGE is how many entries a page of a pool's ledger shows, unless the
// address says otherwise (?limit=, which the checks use).
const PAGE = 25;
// HISTORY is how many entries of an address's history are walked.
const HISTORY = 50;

const short = h => (typeof h === 'string' && h.length > 16) ? `${h.slice(0, 10)}…${h.slice(-6)}` : String(h ?? '');
const str = v => (v === undefined || v === null) ? '' : String(v);
const plural = (n, one, many = one + 's') => `${n} ${Number(n) === 1 ? one : many}`;

// href is the address of a place in this view. series is kept when known:
// poon needs it only on a node that follows more than one series, and it
// is harmless on one that follows one.
function href(p) {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(p)) if (v !== undefined && v !== null && v !== '') q.set(k, String(v));
  return `#/ledger?${q}`;
}

// go is a link to a place in this view, showing a hash shortened (the
// whole of it in the tooltip) or a text.
function go(p, text, title) {
  return el('a', { class: 'go', href: href(p), title: title || undefined }, text);
}
const hashGo = (p, h) => go(p, el('code', {}, short(h)), h);
// mintGo links a mint's serial to its own screen (GUI-GREEN-067).
const mintGo = (n, s) => (n === undefined || n === null || n === '') ? '' : go({ serial: n, series: s }, String(n), `Mint ${n}: who holds it`);

const when = (() => {
  let fmt = null;
  return t => {
    const d = new Date(t);
    if (!t || isNaN(d)) return el('span', {}, str(t));
    try { fmt = fmt || new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'medium' }); } catch (_) { return el('time', { datetime: t }, t); }
    return el('time', { datetime: t, title: t }, fmt.format(d));
  };
})();

// facts is a list of label and value pairs, the way a detail screen shows
// one thing.
function facts(rows) {
  const dl = el('dl', { class: 'kv facts' });
  for (const [k, v] of rows) {
    if (v === undefined || v === null || v === '') continue;
    dl.append(el('dt', {}, k), el('dd', {}, v instanceof Node ? v : String(v)));
  }
  return dl;
}

// clickable makes a table's rows open their first link when clicked
// anywhere, not only on the link itself.
function clickable(wrap) {
  wrap.classList.add('clickable');
  wrap.addEventListener('click', ev => {
    if (ev.target.closest('a, button, code.hex, summary, input')) return;
    const a = ev.target.closest('tr')?.querySelector('a.go');
    if (a) location.hash = a.getAttribute('href');
  });
  return wrap;
}

function section(title, ...kids) {
  return el('section', { class: 'panel ledger-part' }, el('div', { class: 'panel-head' }, el('h2', {}, title)), ...kids);
}

export const ledgerView = {
  id: 'ledger',
  title: 'Ledger',
  mount(root, ctx) {
    const p = ctx.params || new URLSearchParams();
    let gone = false;
    const series = p.get('series') || '';
    const sflag = s => (s ? ['-series', s] : []);

    const search = el('input', { name: 'find', placeholder: 'A transaction id, an address or a mint number', spellcheck: 'false', 'aria-label': 'Transaction id, address or mint number' });
    search.value = p.get('find') || p.get('tid') || p.get('address') || p.get('serial') || '';
    const searchForm = el('form', { class: 'ledger-search', role: 'search', autocomplete: 'off' }, search,
      el('button', { type: 'submit', class: 'primary' }, 'Find'));
    searchForm.onsubmit = ev => {
      ev.preventDefault();
      const v = search.value.replace(/\s+/g, '');
      if (v) location.hash = href({ find: v, series });
    };
    const crumbs = el('nav', { class: 'crumbs', 'aria-label': 'Where you are' });
    const body = el('div', { class: 'ledger-body', 'aria-live': 'polite' });
    const cmds = el('div', { class: 'ledger-cmds' });
    root.append(el('h1', {}, 'Ledger'),
      el('p', { class: 'hint' }, 'What this node holds. Pick a pool to see its ledger, then click an entry, a transaction or an address to go deeper. Back returns.'),
      searchForm, crumbs, body, cmds);

    const setCrumbs = (...parts) => {
      crumbs.replaceChildren();
      const all = [[{ series }, 'Pools'], ...parts];
      all.forEach(([to, text], i) => {
        if (i) crumbs.append(el('span', { class: 'sep', 'aria-hidden': 'true' }, ' › '));
        crumbs.append(i === all.length - 1 || !to ? el('span', { 'aria-current': i === all.length - 1 ? 'page' : undefined }, text) : go(to, text));
      });
    };

    // read runs one poon command and shows its line under the screen. It
    // resolves to {r, v}: the finished run, and its output read as JSON
    // when it ran (v is undefined when it did not, or printed no JSON).
    const read = async args => {
      cmds.append(cmdLine(args, ctx));
      const r = await api.run(args);
      let v;
      if (r.code === 0) { try { v = parseJSON(r.out); } catch (_) { v = undefined; } }
      return { r, v };
    };
    const busy = text => body.replaceChildren(el('p', { class: 'hint loading' }, text));

    // failed shows why a command printed nothing to lay out: no node
    // answering, or poon's own error text exactly as printed.
    const failed = (r, what) => {
      if (needsNode(r)) {
        body.replaceChildren(el('div', { class: 'node-banner ledger-no-node', role: 'status' },
          'No node is answering for this node policy, so there is nothing to show yet. ',
          el('a', { href: '#/node' }, 'Start one on the Node view'), '.'));
        return;
      }
      body.replaceChildren(el('div', { class: 'run-status bad ledger-error', role: 'alert' }, `Could not show ${what}.`),
        el('pre', { class: 'text-out err' }, (r.err || r.out || '').trim()));
    };

    // ---- Pools: the start.
    const pools = async () => {
      setCrumbs();
      busy('Reading the pools…');
      const { r, v } = await read(['pools', ...where.flags(), ...sflag(series)]);
      if (gone) return;
      if (v === undefined) return failed(r, 'the pools');
      const list = Array.isArray(v) ? v : Array.isArray(v && v.pools) ? v.pools : [];
      if (!list.length) {
        body.replaceChildren(el('p', { class: 'hint empty' }, 'This node follows no pools yet.'));
        return;
      }
      const many = new Set(list.map(x => x.series)).size > 1;
      const t = rowsTable(list, x => [
        ...(many ? [el('code', { title: x.series }, short(x.series))] : []),
        go({ pool: x.pool, series: x.series }, `Pool ${x.pool}`),
        `${x.first_serial}–${x.last_serial}`,
        str(x.height), str(x.weight), str(x.pending),
        x.branches ? str(x.branches) : '',
      ], { headers: [...(many ? ['Series'] : []), 'Pool', 'Mints', 'Height', 'Weight', 'Pending', 'Other branches'], cls: 'ledger-pools', more: 'ledger-more', noun: 'pools' });
      body.replaceChildren(section(plural(list.length, 'pool'), el('p', { class: 'hint' }, 'Each pool holds the mints in its range and has its own ledger. Height is how many entries its selected ledger has.'), clickable(t)));
    };

    // ---- A pool: one page of its selected ledger, and what is pending.
    const pool = async (k, belowText) => {
      const limit = Number(p.get('limit')) > 0 ? Number(p.get('limit')) : PAGE;
      setCrumbs([null, `Pool ${k}`]);
      busy(`Reading pool ${k}…`);
      const below = Number(belowText) > 0 ? Number(belowText) : 0;
      const [{ r, v }, pend] = await Promise.all([
        read(['ledger', ...where.flags(), '-pool', String(k), ...sflag(series), ...(below ? ['-below', String(below)] : []), '-limit', String(limit)]),
        read(['pending', ...where.flags(), '-pool', String(k), ...sflag(series)]),
      ]);
      if (gone) return;
      if (v === undefined) return failed(r, `pool ${k}`);
      const s = v.series || series;
      const entries = Array.isArray(v.entries) ? v.entries : [];
      const head = el('p', { class: 'ledger-summary' },
        `Height ${v.height} · weight ${v.weight}`,
        entries.length ? ` · showing ${entries.length === 1 ? `height ${entries[0].height}` : `heights ${entries[entries.length - 1].height} to ${entries[0].height}`}` : '');
      // Paging: Newer starts limit entries above this page's top, or at
      // the tip when that is past it; Older starts where poon says.
      const top = entries.length ? Number(entries[0].height) : 0;
      const nav = el('div', { class: 'actions ledger-pages' });
      const at = b => ({ pool: k, series: s, below: b, limit: p.get('limit') || undefined });
      if (below) {
        nav.append(go(at(undefined), 'Newest'), ' ');
        const up = top + limit;
        nav.append(go(at(up >= Number(v.height) ? undefined : up), 'Newer'), ' ');
      }
      if (v.next && String(v.next) !== '0') nav.append(go(at(v.next), 'Older'));
      const parts = [head];
      if (v.unreadable) parts.push(el('p', { class: 'next-step bad' }, `${plural(v.unreadable, 'entry was', 'entries were')} unreadable from the node's log, so transactions may be missing here. The Node view's Health check reports the damage.`));
      if (!entries.length) parts.push(el('p', { class: 'hint empty' }, 'No entries yet: nothing in this pool has been built on.'));
      else {
        parts.push(ledgerPicture(v, { fromTip: !below, link: e => href({ pool: k, series: s, height: e.height }) }));
        parts.push(clickable(rowsTable(entries, e => [
          go({ pool: k, series: s, height: e.height }, str(e.height)),
          el('code', { title: e.hash }, short(e.hash)),
          e.unreadable ? 'unreadable' : str(e.tx_count),
          str(e.layers), str(e.own), str(e.from), when(e.first_seen),
        ], { headers: ['Height', 'Entry', 'Transactions', 'Layers', 'Own weight', 'Weight behind it', 'First seen'], cls: 'ledger-entries', more: 'ledger-more', noun: 'entries' })));
      }
      if (nav.childNodes.length) parts.push(nav);
      const out = [section(`Pool ${k}: selected ledger`, ...parts)];
      const pl = pend.v === undefined ? null : Array.isArray(pend.v) ? pend.v : [];
      if (pl && pl.length) out.push(section(`Pending (${pl.length})`, el('p', { class: 'hint' }, 'Transactions the node holds that are not in an entry yet.'), txTable(pl, 'ledger-pending')));
      body.replaceChildren(...out);
    };

    // txTable lists transactions, each linked to its own screen.
    const txTable = (txs, cls) => clickable(rowsTable(txs, x => [
      hashGo({ tid: x.tid, series: x.series || series }, x.tid),
      x.claim ? 'Mint claim' : 'Payment', mintGo(x.serial, x.series || series), str(x.amount),
      plural(Array.isArray(x.outputs) ? x.outputs.length : 0, 'output'),
    ], { headers: ['Transaction', 'Kind', 'Mint', 'Amount', 'Pays'], cls, more: 'ledger-more', noun: 'transactions' }));

    // ---- An entry: the one at a height of a pool's selected ledger.
    const entry = async (k, h) => {
      setCrumbs([{ pool: k, series }, `Pool ${k}`], [null, `Height ${h}`]);
      busy(`Reading height ${h} of pool ${k}…`);
      const { r, v } = await read(['ledger', ...where.flags(), '-pool', String(k), ...sflag(series), '-below', String(h), '-limit', '1']);
      if (gone) return;
      if (v === undefined) return failed(r, `height ${h} of pool ${k}`);
      const s = v.series || series;
      const e = Array.isArray(v.entries) ? v.entries[0] : null;
      if (!e || String(e.height) !== String(h)) {
        body.replaceChildren(el('p', { class: 'hint empty' }, `The selected ledger of pool ${k} has no entry at height ${h} (its height is ${v.height}).`));
        return;
      }
      const nav = el('div', { class: 'actions ledger-pages' });
      if (Number(h) > 1) nav.append(go({ pool: k, series: s, height: Number(h) - 1 }, `Older: height ${Number(h) - 1}`), ' ');
      if (Number(h) < Number(v.height)) nav.append(go({ pool: k, series: s, height: Number(h) + 1 }, `Newer: height ${Number(h) + 1}`), ' ');
      nav.append(go({ pool: k, series: s, below: h }, 'This part of the ledger'));
      const txs = Array.isArray(e.txs) ? e.txs : [];
      body.replaceChildren(section(`Entry at height ${h}`,
        facts([
          ['Entry', render(e.hash, 0, 'hash')],
          ['Parent', Number(h) > 1 ? el('span', {}, render(e.parent, 0, 'parent'), ' ', go({ pool: k, series: s, height: Number(h) - 1 }, 'open')) : render(e.parent, 0, 'parent')],
          ['Pool', go({ pool: k, series: s }, `Pool ${k}`)],
          ['Own weight', str(e.own)],
          ['Ledger weight up to here', str(e.ledger)],
          ['Weight behind it', str(e.from)],
          ['Layers', str(e.layers)],
          ['First seen', when(e.first_seen)],
        ]), nav),
        section(`Transactions (${e.tx_count})`,
          e.unreadable ? el('p', { class: 'next-step bad' }, 'This entry\'s body could not be read back from the node\'s log. The Node view\'s Health check reports the damage.')
            : txs.length ? txTable(txs, 'ledger-txs') : el('p', { class: 'hint empty' }, 'None.')));
    };

    // ---- A transaction: where it stands, what it spent, what it paid.
    const txScreen = (st, s) => {
      const t = st.tx || {};
      const k = st.pool;
      setCrumbs([{ pool: k, series: s }, `Pool ${k}`], ...(st.selected && st.height ? [[{ pool: k, series: s, height: st.height }, `Height ${st.height}`]] : []), [null, `Transaction ${short(t.tid)}`]);
      const where_ = st.selected && st.entry
        ? el('span', {}, 'In the selected ledger, at ', go({ pool: k, series: s, height: st.height }, `height ${st.height}`), ` of pool ${k}.`)
        : st.pending ? el('span', {}, `Pending in pool ${k}: not in an entry yet.`)
          : el('span', {}, `Held in pool ${k}, but not in its selected ledger.`);
      const rivals = Array.isArray(st.rivals) ? st.rivals : [];
      const ins = Array.isArray(t.inputs) ? t.inputs : [];
      const outs = Array.isArray(t.outputs) ? t.outputs : [];
      const parts = [section(t.claim ? 'Mint claim' : 'Payment',
        facts([
          ['Where it stands', where_],
          ['Transaction', render(t.tid, 0, 'hash')],
          ['Mint', mintGo(t.serial, s)],
          ['Amount', str(t.amount)],
          ['Weight behind it', st.selected ? `${st.from} (a rival must beat this to reverse it)` : ''],
          ['Size', t.size !== undefined ? `${t.size} bytes` : ''],
        ]),
        rivals.length ? el('p', { class: 'next-step bad ledger-rivals' }, `${plural(rivals.length, 'rival branch has', 'rival branches have')} a transaction spending the same inputs (listed below).`) : null)];
      if (ins.length) {
        parts.push(section('Spends', clickable(rowsTable(ins, x => [
          hashGo({ tid: x.tid, series: s }, x.tid), str(x.index),
          x.address ? hashGo({ address: x.address, series: s }, x.address) : '',
        ], { headers: ['From transaction', 'Output', 'Address'], cls: 'ledger-inputs', more: 'ledger-more', noun: 'inputs' }))));
      }
      parts.push(section('Pays', outs.length ? clickable(rowsTable(outs, x => [
        str(x.index), hashGo({ address: x.address, series: s }, x.address), str(x.amount),
      ], { headers: ['Output', 'Address', 'Amount'], cls: 'ledger-outputs', more: 'ledger-more', noun: 'outputs' })) : el('p', { class: 'hint empty' }, 'Nothing.')));
      if (rivals.length) parts.push(section('Rival branches', render(rivals)));
      if (t.raw) parts.push(el('details', { class: 'raw ledger-raw' }, el('summary', {}, 'The transaction as bytes (hex)'), el('pre', { class: 'text-out' }, t.raw)));
      body.replaceChildren(...parts);
    };

    const tx = async tid => {
      setCrumbs([null, `Transaction ${short(tid)}`]);
      busy('Looking up the transaction…');
      const { r, v } = await read(['tx', ...where.flags(), ...sflag(series), tid]);
      if (gone) return;
      if (v === undefined) return failed(r, 'that transaction');
      txScreen(v, (v.tx && v.tx.series) || series);
    };

    // ---- An address: its balance, holdings, history and pending.
    const addrScreen = (a, s, note) => {
      setCrumbs([null, `Address ${short(a.address)}`]);
      const hold = Array.isArray(a.holdings) ? a.holdings : [];
      const hist = Array.isArray(a.history) ? a.history : [];
      const pend = Array.isArray(a.pending) ? a.pending : [];
      const parts = [];
      if (note) parts.push(el('p', { class: 'hint ledger-found-note' }, note));
      parts.push(section('Address', facts([
        ['Address', render(a.address, 0, 'hash')],
        ['Balance', el('strong', { class: 'ledger-balance' }, str(a.balance))],
        ['Unspent portions', String(hold.length)],
      ])));
      if (a.unreadable) parts.push(el('p', { class: 'next-step bad' }, `${plural(a.unreadable, 'entry was', 'entries were')} unreadable from the node's log, so the history may be missing items.`));
      parts.push(section(`Holdings (${hold.length})`, hold.length ? clickable(rowsTable(hold, x => [
        hashGo({ tid: x.input && x.input.tid, series: s }, x.input && x.input.tid), str(x.input && x.input.index), mintGo(x.serial, s), str(x.amount),
      ], { headers: ['Made by transaction', 'Output', 'Mint', 'Amount'], cls: 'ledger-holdings', more: 'ledger-more', noun: 'holdings' })) : el('p', { class: 'hint empty' }, 'Nothing unspent.')));
      const more = Array.isArray(a.next) ? a.next : [];
      parts.push(section(`History (${hist.length})`, hist.length ? clickable(rowsTable(hist, x => [
        hashGo({ tid: x.tx && x.tx.tid, series: s }, x.tx && x.tx.tid),
        go({ pool: x.pool, series: s, height: x.height }, `Pool ${x.pool}, height ${x.height}`),
        x.received ? `+${x.received}` : '', x.spends ? `spent ${plural(x.spends, 'portion')}` : '', str(x.from),
      ], { headers: ['Transaction', 'Where', 'Received', 'Spent', 'Weight behind it'], cls: 'ledger-history', more: 'ledger-more', noun: 'history items' })) : el('p', { class: 'hint empty' }, 'No transactions in the selected ledgers.'),
      ...more.map(n => el('p', { class: 'hint ledger-older' }, `Older history in pool ${n.pool}: `, go({ address: a.address, series: s, pool: n.pool, below: n.below ?? n.height }, 'show it')))));
      if (pend.length) parts.push(section(`Pending (${pend.length})`, txTable(pend, 'ledger-addr-pending')));
      body.replaceChildren(...parts);
    };

    const address = async (addr, k, below) => {
      setCrumbs([null, `Address ${short(addr)}`]);
      busy('Looking up the address…');
      const { r, v } = await read(['address', ...where.flags(), ...sflag(series), '-limit', String(HISTORY),
        ...(k ? ['-pool', k] : []), ...(k && below ? ['-below', below] : []), addr]);
      if (gone) return;
      if (v === undefined) return failed(r, 'that address');
      addrScreen(v, v.series || series);
    };

    // ---- A mint: who holds its units now (poon unspent -serial), and the
    // pool that holds it (from poon pools' serial ranges). GUI-GREEN-067.
    const mint = async n => {
      setCrumbs([null, `Mint ${n}`]);
      busy(`Looking up mint ${n}…`);
      const [u, pl] = await Promise.all([
        read(['unspent', ...where.flags(), ...sflag(series), '-serial', String(n)]),
        read(['pools', ...where.flags()])]);
      if (gone) return;
      if (u.v === undefined) return failed(u.r, `mint ${n}`);
      const rows = Array.isArray(u.v) ? u.v : [];
      const pool = (Array.isArray(pl.v) ? pl.v : []).find(x => (!series || x.series === series) && Number(x.first_serial) <= Number(n) && Number(n) <= Number(x.last_serial));
      if (pool) setCrumbs([{ pool: pool.pool, series: pool.series }, `Pool ${pool.pool}`], [null, `Mint ${n}`]);
      const s = (pool && pool.series) || series;
      const parts = [section(`Mint ${n}`, facts([
        ['Pool', pool ? go({ pool: pool.pool, series: s }, `Pool ${pool.pool}`) : 'No pool this node follows holds this mint.'],
        ['Unspent portions', String(rows.length)],
      ]))];
      parts.push(section(`Who holds it (${rows.length})`, rows.length ? clickable(rowsTable(rows, x => [
        hashGo({ address: x.address, series: s }, x.address), str(x.amount),
        hashGo({ tid: x.input && x.input.tid, series: s }, x.input && x.input.tid), str(x.input && x.input.index),
      ], { headers: ['Address', 'Amount', 'Made by transaction', 'Output'], cls: 'ledger-mint', more: 'ledger-more', noun: 'portions' }))
        : el('p', { class: 'hint empty' }, pool ? 'Nothing unspent in the selected ledger: the mint is not claimed yet, or its claim is still pending.' : 'Nothing.')));
      body.replaceChildren(...parts);
    };

    // ---- Find: a mint number, else a transaction, then an address.
    const find = async text => {
      if (/^\d+$/.test(text)) return mint(text);
      setCrumbs([null, `Find ${short(text)}`]);
      busy('Looking…');
      const t = await read(['tx', ...where.flags(), ...sflag(series), text]);
      if (gone) return;
      if (t.v !== undefined) return txScreen(t.v, (t.v.tx && t.v.tx.series) || series);
      if (needsNode(t.r)) return failed(t.r, '');
      const a = await read(['address', ...where.flags(), ...sflag(series), '-limit', String(HISTORY), text]);
      if (gone) return;
      if (a.v !== undefined) return addrScreen(a.v, a.v.series || series, 'Not a transaction this node holds, so it is shown as an address.');
      if (needsNode(a.r)) return failed(a.r, '');
      body.replaceChildren(el('div', { class: 'run-status bad ledger-not-found', role: 'alert' }, `Nothing found for ${text}: it is not a mint number, a transaction this node holds, or an address.`),
        el('details', {}, el('summary', {}, 'What poon said'),
          el('pre', { class: 'text-out err' }, `poon tx: ${(t.r.err || '').trim()}\npoon address: ${(a.r.err || '').trim()}`)));
    };

    const k = p.get('pool');
    if (p.get('tid')) tx(p.get('tid'));
    else if (p.get('serial')) mint(p.get('serial'));
    else if (p.get('address')) address(p.get('address'), k, p.get('below'));
    else if (p.get('find')) find(p.get('find'));
    else if (k !== null && k !== '' && p.get('height')) entry(k, p.get('height'));
    else if (k !== null && k !== '') pool(k, p.get('below'));
    else pools();
    return () => { gone = true; };
  },
};
