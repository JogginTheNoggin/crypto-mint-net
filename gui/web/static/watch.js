// Watch and Explore: the node's live events and its views, selected by the
// same filter flags as poo watch, poo pools, poo ledger and the rest. The
// filter is only turned into flags here; poo applies it (pkg/filter).
import * as api from './api.js';
import { el, cmdLine, panel, form, flag, mountAll } from './views.js';
import { render } from './render.js';
import * as where from './where.js';

const KINDS = ['tx', 'entry', 'layer', 'selected', 'reverted', 'switch', 'dropped', 'grind'];
// Bounds on what the view keeps, so a busy node cannot exhaust the tab
// (GUI-007 in security/gui/findings): table rows and events held while
// paused, the error text's tail, and one unfinished line.
const MAX_EVENTS = 1000;
const LIVE_ROWS = 200;
const DRAW_EVERY = 100;
const MAX_ERR = 64 << 10;
const MAX_LINE = 1 << 20;

// filterFields are the form fields of a filter (Watch adds the kinds as
// boxes); filterFlags turns their values into poo's filter flags (comma
// lists, as poo takes them).
const filterFields = () => [
  { name: 'series', label: 'Series ids', placeholder: 'comma list' },
  { name: 'pool', label: 'Pools', placeholder: 'comma list' },
  { name: 'serial', label: 'Mint serials', placeholder: 'comma list' },
  { name: 'address', label: 'Addresses', placeholder: 'comma list' },
  { name: 'addressPolicy', label: 'Address policies', placeholder: 'hex, comma list' },
  { name: 'tid', label: 'Transaction ids', placeholder: 'comma list' },
];
const filterFlags = v => [
  ...flag('series', v.series), ...flag('pool', v.pool), ...flag('serial', v.serial),
  ...flag('address', v.address), ...flag('address-policy', v.addressPolicy),
  ...flag('tid', v.tid), ...flag('kind', v.kind),
].map(a => a.replace(/\s+/g, ''));

const short = h => (typeof h === 'string' && h.length > 16) ? `${h.slice(0, 10)}…${h.slice(-6)}` : (h ?? '');

// clock shows poo's UTC time as local time to the millisecond, like the
// Jobs view; the time exactly as printed is in the tooltip.
function clock(t) {
  const d = new Date(t);
  if (!t || isNaN(d)) return el('span', {}, t || '');
  const pad = (n, w = 2) => String(n).padStart(w, '0');
  return el('time', { datetime: t, title: t }, `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${pad(d.getMilliseconds(), 3)}`);
}

// eventRow is one line of the live table: what poo watch printed, laid out.
function eventRow(ev) {
  const what = ev.kind === 'switch' && ev.switch ? `to ${short(ev.switch.to)} at ${ev.switch.fork_height}`
    : ev.entry ? `entry ${short(ev.entry.hash)} h${ev.entry.height ?? ''}`
    : ev.layer ? `layer s${ev.layer.strength} on ${short(ev.layer.entry)}` : '';
  const tr = el('tr', { class: `ev ev-${ev.kind}` },
    el('td', { class: 'num' }, String(ev.seq ?? '')),
    el('td', {}, clock(ev.time)),
    el('td', {}, el('span', { class: `kind kind-${ev.kind}` }, ev.kind + (ev.void ? ' (void)' : ''))),
    el('td', { class: 'num' }, String(ev.pool ?? '')),
    // The tid is shown, copied and linked the same way as in every other view.
    el('td', {}, ev.tid ? render(ev.tid, 0, 'tid') : ''),
    el('td', {}, what),
    el('td', { class: 'num' }, ev.weight ?? ''),
    el('td', {}, ev.origin ? `${ev.origin}${ev.from ? ' ' + ev.from : ''}` : ''));
  tr.tabIndex = 0;
  tr.title = 'Show the whole event';
  return tr;
}

// The Watch panel is built once and kept, like the terminal's pane, so a
// running watch and its table survive switching views (GREEN-015): a
// look-up link from an event to Explore does not lose the stream. Stop
// stays explicit, and the Watch tab shows a dot while it runs.
//
// While the view is away the poo watch job keeps running on the server,
// but the page closes its event stream and remembers the next output
// sequence number; coming back follows again from there (?from=), so the
// browser holds no stream and parses nothing in the background, and no
// output is missed unless the server dropped it (GUI-008 in
// security/gui/findings).
let watchPanel = null;
let panelHooks = null; // {detach, attach} of the built panel

function buildWatch(ctx) {
  const fields = filterFields();
  const inputs = {};
  const f = el('form', { class: 'fields', autocomplete: 'off' });
  for (const fd of fields) {
    inputs[fd.name] = el('input', { name: fd.name, placeholder: fd.placeholder, spellcheck: 'false' });
    f.append(el('label', {}, el('span', {}, fd.label), inputs[fd.name]));
  }
  // Kinds are boxes to tick rather than words to remember. None ticked
  // means every kind, as an empty -kind does.
  const kindBoxes = KINDS.map(k => el('input', { type: 'checkbox', name: 'kind', value: k }));
  f.append(el('fieldset', { class: 'kinds' }, el('legend', {}, 'Kinds (none ticked: all)'),
    ...KINDS.map((k, i) => el('label', { class: 'check' }, kindBoxes[i], ` ${k}`))));
  const notices = el('input', { type: 'checkbox', name: 'notices' });
  f.append(el('label', { class: 'check' }, el('span', {}, 'Show stream notices (hello, ping, missed, end)'), notices));
  const values = () => ({ ...Object.fromEntries(fields.map(fd => [fd.name, inputs[fd.name].value.trim()])),
    kind: kindBoxes.filter(b => b.checked).map(b => b.value).join(',') });
  const args = () => ['watch', ...where.flags(), ...flag('notices', notices.checked), ...filterFlags(values())];
  const line = cmdLine(args(), ctx);
  f.addEventListener('input', () => line.set(args()));
  const startBtn = el('button', { type: 'submit', class: 'primary' }, 'Start watching');
  const pauseBtn = el('button', { type: 'button', disabled: true }, 'Pause');
  const stopBtn = el('button', { type: 'button', disabled: true }, 'Stop');
  const clearBtn = el('button', { type: 'button' }, 'Clear');
  f.append(el('div', { class: 'actions' }, startBtn, ' ', pauseBtn, ' ', stopBtn, ' ', clearBtn));

  const state = el('div', { class: 'run-status' }, 'Not watching.');
  const counts = el('div', { class: 'counts hint' });
  const body = el('tbody');
  const table = el('table', { class: 'data events' },
    el('thead', {}, el('tr', {}, ...['seq', 'time', 'kind', 'pool', 'tid', 'what', 'weight', 'origin'].map(h => el('th', {}, h)))), body);
  // While events arrive, only the newest LIVE_ROWS rows are laid out: the
  // browser lays out a whole table on every change, and 1000 rows cost
  // most of a frame (ORANGE-001). All MAX_EVENTS rows stay in the table,
  // and "Show all" or Pause shows them.
  const allBox = el('input', { type: 'checkbox' });
  const showAll = () => table.classList.toggle('all', allBox.checked || paused);
  allBox.addEventListener('input', showAll);
  const rowsNote = el('label', { class: 'check hint' }, allBox, ` Show all ${MAX_EVENTS} kept rows (while live, the newest ${LIVE_ROWS} show; Pause shows all)`);
  const detail = el('div', { class: 'event-detail' });
  const errOut = el('pre', { class: 'text-out err', hidden: true });

  let job = null, unfollow = null, paused = false, held = [], heldDropped = 0, buf = '';
  let next = 0; // the job's next output sequence number, to follow from
  // errAppend adds text to the error box, keeping its last MAX_ERR
  // characters.
  const errAppend = text => {
    errOut.hidden = false;
    let t = errOut.textContent + text;
    if (t.length > MAX_ERR) t = '[earlier errors dropped]\n' + t.slice(-MAX_ERR);
    errOut.textContent = t;
  };
  const tally = {};
  const showCounts = () => { counts.textContent = Object.entries(tally).map(([k, n]) => `${k} ${n}`).join(' · '); };
  // Events are counted as they arrive but drawn once per frame
  // (ORANGE-001): a frame takes at most the newest MAX_EVENTS waiting,
  // builds their rows in one fragment, and lays the table out once.
  // Rows and events are kept in a WeakMap for the one click handler.
  const evOf = new WeakMap();
  // A busy stream is drawn at most every DRAW_EVERY ms (ORANGE-006):
  // each draw lays the table out again, so ten draws a second cost a
  // sixth of sixty, and a table that changes ten times a second still
  // reads as live.
  let pending = [], drawn = 0, frame = 0, timer = 0, lastDraw = -Infinity;
  const schedule = () => {
    if (frame || timer) return;
    const wait = lastDraw + DRAW_EVERY - performance.now();
    if (wait <= 0) frame = requestAnimationFrame(flush);
    else timer = setTimeout(() => { timer = 0; frame = requestAnimationFrame(flush); }, wait);
  };
  const flush = () => {
    frame = 0;
    lastDraw = performance.now();
    const evs = pending.length > MAX_EVENTS ? pending.slice(-MAX_EVENTS) : pending;
    pending = [];
    showCounts();
    const frag = document.createDocumentFragment();
    for (let i = evs.length - 1; i >= 0; i--) {
      const tr = eventRow(evs[i]);
      evOf.set(tr, evs[i]);
      frag.append(tr);
    }
    if (evs.length >= MAX_EVENTS) body.replaceChildren(frag);
    else {
      body.prepend(frag);
      for (let n = body.children.length; n > MAX_EVENTS; n--) body.lastChild.remove();
    }
    drawn += evs.length;
    body.dataset.drawn = String(drawn);
  };
  const add = ev => {
    if (ev.kind === 'hello' || ev.kind === 'ping' || ev.kind === 'missed' || ev.kind === 'end') {
      state.textContent = ev.kind === 'missed' ? `${ev.missed} events missed (the GUI read too slowly)` : `stream: ${ev.kind}`;
      if (ev.kind === 'missed') { tally.missed = (tally.missed || 0) + ev.missed; showCounts(); }
      return;
    }
    tally[ev.kind] = (tally[ev.kind] || 0) + 1;
    pending.push(ev);
    // Only the newest MAX_EVENTS can be shown, so no more than twice
    // that wait, even while the tab is hidden and frames do not run.
    if (pending.length > 2 * MAX_EVENTS) pending = pending.slice(-MAX_EVENTS);
    schedule();
  };
  const show = tr => {
    const ev = tr && evOf.get(tr);
    if (!ev) return;
    detail.textContent = '';
    detail.append(el('h2', {}, `Event ${ev.seq}: ${ev.kind}`), render(ev));
  };
  body.addEventListener('click', e => show(e.target.closest('tr')));
  body.addEventListener('keydown', e => { if (e.key === 'Enter' && e.target.matches('tr')) show(e.target); });
  const lineIn = text => {
    if (!text.trim()) return;
    let ev;
    try { ev = JSON.parse(text); } catch (_) { errAppend(text + '\n'); return; }
    if (!paused) { add(ev); return; }
    held.push(ev);
    if (held.length > MAX_EVENTS) { held.shift(); heldDropped++; pauseBtn.textContent = `Resume (${heldDropped} dropped)`; }
  };
  const setRunning = on => {
    startBtn.disabled = on;
    stopBtn.disabled = pauseBtn.disabled = !on;
    const tab = document.querySelector('nav a[data-id=watch]');
    if (tab) { tab.classList.toggle('busy', on); tab.title = on ? 'A watch is running' : ''; }
  };
  f.addEventListener('submit', async e => {
    e.preventDefault();
    const a = args();
    line.set(a);
    errOut.hidden = true;
    errOut.textContent = '';
    try { job = await api.start(a); } catch (err) { state.textContent = err.message; return; }
    setRunning(true);
    state.textContent = `watching: ${job.command}`;
    next = 0;
    buf = '';
    follow();
  });
  // follow follows the job's output from next on.
  const follow = () => {
    const id = job.id;
    unfollow = api.follow(id, {
      chunk: c => {
        next = c.next ?? c.seq + 1; // a chunk may join several writes
        if (c.stream === 'err') { errAppend(c.data); return; }
        buf += c.data;
        const lines = buf.split('\n');
        buf = lines.pop();
        lines.forEach(lineIn);
        if (buf.length > MAX_LINE) { errAppend(`[dropped a line longer than ${MAX_LINE} bytes]\n`); buf = ''; }
      },
      // The server keeps a bounded amount of a job's output; what it
      // dropped while the view was away is gone, and so is any partial line.
      dropped: d => {
        next = d.next;
        buf = '';
        state.textContent = 'some events were dropped while the view was away (poo-gui keeps the newest 4 MiB of output)';
        tally['dropped while away'] = (tally['dropped while away'] || 0) + 1;
        showCounts();
      },
      exit: x => { unfollow = null; setRunning(false); state.textContent = `stopped (exit ${x.code})`; job = null; },
    }, next);
  };
  panelHooks = {
    detach: () => { if (unfollow) { unfollow(); unfollow = null; } },
    attach: () => { if (job && !unfollow) follow(); },
  };
  pauseBtn.onclick = () => {
    paused = !paused;
    pauseBtn.textContent = paused ? `Resume` : 'Pause';
    showAll();
    if (!paused) {
      if (heldDropped) { state.textContent = `${heldDropped} events dropped while paused (only the newest ${MAX_EVENTS} are held)`; tally['dropped while paused'] = (tally['dropped while paused'] || 0) + heldDropped; showCounts(); }
      held.forEach(add);
      held = [];
      heldDropped = 0;
      // What was held shows at once, not on the next frame.
      if (frame || timer) { cancelAnimationFrame(frame); clearTimeout(timer); timer = 0; flush(); }
    }
  };
  stopBtn.onclick = () => { if (job) api.cancel(job.id).catch(() => {}); };
  clearBtn.onclick = () => { pending = []; body.textContent = ''; detail.textContent = ''; for (const k in tally) delete tally[k]; showCounts(); };

  const sec = el('section', { class: 'panel' },
    el('div', { class: 'panel-head' }, el('h2', {}, 'Live events')),
    el('p', { class: 'hint' }, 'Runs poo watch with these filters and shows each event as it arrives, newest first. Empty fields match everything; within a field, any listed value matches. Click an event for all of it.'),
    f, line, state, counts, errOut, rowsNote, el('div', { class: 'table-wrap events-wrap' }, table), detail);
  return sec;
}

export const watchView = {
  id: 'watch',
  title: 'Watch',
  mount(root, ctx) {
    if (!watchPanel) watchPanel = buildWatch(ctx);
    else panelHooks.attach();
    // Leaving the view detaches the panel and closes its stream; the
    // watch keeps running on the server.
    return mountAll(root, 'Watch', 'What the node does, live: transactions arriving, entries and layers kept, the selected ledger moving. The watch keeps running when you switch views; Stop ends it.', [{ el: watchPanel, stop: () => { panelHooks.detach(); watchPanel.remove(); } }]);
  },
};

// ledgerPicture draws one page of poo ledger: each entry a bar, oldest on
// the left, linked to its parent, its height under it and its own weight
// on it. Bar heights are scaled to the largest own weight on the page, and
// every number shown is a field poo ledger printed (the tooltip has the
// rest). It computes nothing about the ledger itself. fromTip marks the
// newest bar as the tip.
const SVG = 'http://www.w3.org/2000/svg';
function svg(tag, attrs, text) {
  const e = document.createElementNS(SVG, tag);
  for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v);
  if (text !== undefined) e.textContent = text;
  return e;
}
export function ledgerPicture(page, { fromTip = false } = {}) {
  const entries = Array.isArray(page && page.entries) ? page.entries.slice().reverse() : [];
  if (!entries.length) return null;
  const W = 46, GAP = 18, H = 90, TOP = 16, BOTTOM = 34;
  const own = e => Number(e.own) || 0;
  const max = Math.max(1, ...entries.map(own));
  const width = entries.length * (W + GAP) + GAP;
  const s = svg('svg', { class: 'ledger-pic', viewBox: `0 0 ${width} ${TOP + H + BOTTOM}`, width, height: TOP + H + BOTTOM, role: 'img',
    'aria-label': `Selected ledger of pool ${page.pool}: ${entries.length} ${entries.length === 1 ? 'entry' : 'entries'}, heights ${entries[0].height} to ${entries[entries.length - 1].height}` });
  entries.forEach((e, i) => {
    const x = GAP + i * (W + GAP);
    const h = Math.max(3, Math.round(H * own(e) / max));
    const g = svg('g', { class: i === entries.length - 1 && fromTip ? 'lp-entry lp-tip' : 'lp-entry' });
    g.append(svg('title', {}, `entry ${e.hash}\nheight ${e.height}\nown ${e.own} · ledger ${e.ledger} · from ${e.from}\nlayers ${e.layers} · transactions ${e.tx_count}`));
    g.append(svg('rect', { class: 'lp-slot', x, y: TOP, width: W, height: H, rx: 4 }));
    g.append(svg('rect', { class: 'lp-bar', x, y: TOP + H - h, width: W, height: h, rx: 4 }));
    g.append(svg('text', { class: 'lp-num', x: x + W / 2, y: TOP - 4, 'text-anchor': 'middle' }, String(e.own)));
    g.append(svg('text', { class: 'lp-h', x: x + W / 2, y: TOP + H + 14, 'text-anchor': 'middle' }, `h${e.height}`));
    g.append(svg('text', { class: 'lp-tx', x: x + W / 2, y: TOP + H + 28, 'text-anchor': 'middle' }, `${e.tx_count} tx`));
    // The link to the parent, when the parent is the bar on the left.
    if (i > 0 && entries[i - 1].hash === e.parent) g.append(svg('line', { class: 'lp-link', x1: x - GAP, y1: TOP + H / 2, x2: x, y2: TOP + H / 2 }));
    s.append(g);
  });
  const wrap = el('figure', { class: 'ledger-fig' }, s,
    el('figcaption', { class: 'hint' }, 'Oldest on the left. Bar: the entry\'s own weight, scaled to the largest on this page; the number above is exact. Hover for the rest.'));
  return wrap;
}

export const exploreView = {
  id: 'explore',
  title: 'Explore',
  mount(root, ctx) {
    const filt = filterFields();
    const series = { name: 'series1', label: 'Series id', placeholder: 'only if the node follows more than one' };
    const ledger = form(ctx, {
      title: 'Selected ledger',
      about: 'A pool\'s selected ledger, newest entry first, one page at a time. With filter fields set, only entries holding matching transactions.',
      fields: [{ name: 'pool1', label: 'Pool', type: 'number', value: '0' }, { name: 'below', label: 'Below height', type: 'number', placeholder: 'default: the tip' },
        { name: 'limit', label: 'Entries per page', type: 'number', placeholder: '50' }, ...filt.filter(fd => fd.name !== 'pool')],
      args: v => ['ledger', ...where.flags(), ...flag('pool', v.pool1), ...flag('below', v.below), ...flag('limit', v.limit), ...filterFlags(v)],
      submit: 'Show ledger',
      // "Older" asks for the next page with -below set to the page's
      // next, read from poo ledger's JSON rather than from the page.
      onResult: (r, area) => {
        if (r.code !== 0) return;
        let page, next;
        try { page = JSON.parse(r.out); next = page.next; } catch (_) { return; }
        // A page asked for without -below starts at the tip, which is marked.
        const pic = ledgerPicture(page, { fromTip: !area.closest('section').querySelector('input[name=below]').value.trim() });
        if (pic) area.querySelector('.result')?.prepend(pic);
        if (!next || String(next) === '0') return;
        const b = el('button', { type: 'button' }, `Older (below height ${next})`);
        b.onclick = () => ledger.run({ below: String(next) });
        area.append(b);
      },
    });
    const txForm = form(ctx, { title: 'A transaction', about: 'One transaction and where it stands.', fields: [{ name: 'tid1', label: 'Transaction id' }, series, { name: 'inSerial', label: 'In mint serial', type: 'number', placeholder: 'optional' }],
      args: v => ['tx', ...where.flags(), ...flag('series', v.series1), ...flag('in-serial', v.inSerial), ...(v.tid1 ? [v.tid1] : [])], submit: 'Look up' });
    // Below and Transaction ids are poo address's -below and -tid, which
    // the parity check found no form sent (GUI-GREEN-042).
    const addrForm = form(ctx, { title: 'An address', about: 'Balance, holdings, history and pending.', fields: [{ name: 'addr', label: 'Address' }, series, { name: 'limit', label: 'History entries', type: 'number', placeholder: '50' }, { name: 'below', label: 'Below height', type: 'number', placeholder: 'default: the tip' }, ...filt.slice(1, 3), filt[5]],
      args: v => ['address', ...where.flags(), ...flag('series', v.series1), ...flag('limit', v.limit), ...flag('below', v.below), ...flag('pool', v.pool), ...flag('serial', v.serial), ...flag('tid', (v.tid || '').replace(/\s+/g, '')), ...(v.addr ? [v.addr] : [])], submit: 'Look up' });
    // A "look up" link elsewhere opens Explore with #/explore?tid=… or
    // ?address=…, and the form runs as if the user had filled it in.
    const p = ctx.params || new URLSearchParams();
    if (p.get('tid')) setTimeout(() => txForm.run({ tid1: p.get('tid') }));
    else if (p.get('address')) setTimeout(() => addrForm.run({ addr: p.get('address') }));
    return mountAll(root, 'Explore', 'What the node holds, selected with the same filters as Watch. Each is one poo command.', [
      form(ctx, { title: 'Pools', about: 'Followed pools: tip, height, weight, pending.', fields: filt,
        args: v => ['pools', ...where.flags(), ...filterFlags(v)], submit: 'Show pools' }),
      ledger,
      form(ctx, { title: 'Pending transactions', fields: filt, args: v => ['pending', ...where.flags(), ...filterFlags(v)], submit: 'Show pending' }),
      form(ctx, { title: 'Unspent portions', fields: filt, args: v => ['unspent', ...where.flags(), ...filterFlags(v)], submit: 'Show unspent' }),
      txForm,
      addrForm,
    ]);
  },
};
