// Watch and Explore: the node's live events and its views, selected by the
// same filter flags as poo watch, poo pools, poo ledger and the rest. The
// filter is only turned into flags here; poo applies it (pkg/filter).
import * as api from './api.js';
import { el, cmdLine, panel, form, flag, mountAll } from './views.js';
import { render } from './render.js';
import * as where from './where.js';

const KINDS = ['tx', 'entry', 'layer', 'selected', 'reverted', 'switch', 'dropped'];
const MAX_EVENTS = 1000;

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

export const watchView = {
  id: 'watch',
  title: 'Watch',
  mount(root, ctx) {
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
    const detail = el('div', { class: 'event-detail' });
    const errOut = el('pre', { class: 'text-out err', hidden: true });

    let job = null, unfollow = null, paused = false, held = [], buf = '';
    const tally = {};
    const showCounts = () => { counts.textContent = Object.entries(tally).map(([k, n]) => `${k} ${n}`).join(' · '); };
    const add = ev => {
      if (ev.kind === 'hello' || ev.kind === 'ping' || ev.kind === 'missed' || ev.kind === 'end') {
        state.textContent = ev.kind === 'missed' ? `${ev.missed} events missed (the GUI read too slowly)` : `stream: ${ev.kind}`;
        if (ev.kind === 'missed') { tally.missed = (tally.missed || 0) + ev.missed; showCounts(); }
        return;
      }
      tally[ev.kind] = (tally[ev.kind] || 0) + 1;
      showCounts();
      const tr = eventRow(ev);
      const show = () => { detail.textContent = ''; detail.append(el('h2', {}, `Event ${ev.seq}: ${ev.kind}`), render(ev)); };
      tr.addEventListener('click', show);
      tr.addEventListener('keydown', e => { if (e.key === 'Enter') show(); });
      body.prepend(tr);
      while (body.children.length > MAX_EVENTS) body.lastChild.remove();
    };
    const lineIn = text => {
      if (!text.trim()) return;
      let ev;
      try { ev = JSON.parse(text); } catch (_) { errOut.hidden = false; errOut.append(text + '\n'); return; }
      if (paused) held.push(ev); else add(ev);
    };
    const setRunning = on => { startBtn.disabled = on; stopBtn.disabled = pauseBtn.disabled = !on; };
    f.addEventListener('submit', async e => {
      e.preventDefault();
      const a = args();
      line.set(a);
      errOut.hidden = true;
      errOut.textContent = '';
      try { job = await api.start(a); } catch (err) { state.textContent = err.message; return; }
      setRunning(true);
      state.textContent = `watching: ${job.command}`;
      unfollow = api.follow(job.id, {
        chunk: c => {
          if (c.stream === 'err') { errOut.hidden = false; errOut.append(c.data); return; }
          buf += c.data;
          const lines = buf.split('\n');
          buf = lines.pop();
          lines.forEach(lineIn);
        },
        exit: x => { setRunning(false); state.textContent = `stopped (exit ${x.code})`; job = null; },
      });
    });
    pauseBtn.onclick = () => {
      paused = !paused;
      pauseBtn.textContent = paused ? `Resume` : 'Pause';
      if (!paused) { held.forEach(add); held = []; }
    };
    stopBtn.onclick = () => { if (job) api.cancel(job.id).catch(() => {}); };
    clearBtn.onclick = () => { body.textContent = ''; detail.textContent = ''; for (const k in tally) delete tally[k]; showCounts(); };

    const sec = el('section', { class: 'panel' },
      el('div', { class: 'panel-head' }, el('h2', {}, 'Live events')),
      el('p', { class: 'hint' }, 'Runs poo watch with these filters and shows each event as it arrives, newest first. Empty fields match everything; within a field, any listed value matches. Click an event for all of it.'),
      f, line, state, counts, errOut, el('div', { class: 'table-wrap' }, table), detail);
    const stop = mountAll(root, 'Watch', 'What the node does, live: transactions arriving, entries and layers kept, the selected ledger moving.', [{ el: sec, stop: () => {} }]);
    // Leaving the view stops the watch, as closing a shell would.
    return () => { stop(); if (unfollow) unfollow(); if (job) api.cancel(job.id).catch(() => {}); };
  },
};

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
    });
    // "Older" asks for the next page with -below set to the page's next.
    ledger.el.addEventListener('click', e => {
      const b = e.target.closest('button[data-next]');
      if (!b) return;
      const below = ledger.el.querySelector('input[name=below]');
      below.value = b.dataset.next;
      below.dispatchEvent(new Event('input', { bubbles: true }));
      ledger.el.querySelector('form.fields').requestSubmit();
    });
    new MutationObserver(() => {
      const res = ledger.el.querySelector('.result');
      if (!res || res.dataset.paged) return;
      res.dataset.paged = '1';
      const nextDt = [...res.querySelectorAll('dl.kv > dt')].find(dt => dt.textContent === 'next');
      const next = nextDt && nextDt.nextElementSibling.textContent;
      if (next && next !== '0') res.append(el('button', { type: 'button', 'data-next': next }, `Older (below height ${next})`));
    }).observe(ledger.el, { childList: true, subtree: true });
    const txForm = form(ctx, { title: 'A transaction', about: 'One transaction and where it stands.', fields: [{ name: 'tid1', label: 'Transaction id' }, series, { name: 'inSerial', label: 'In mint serial', type: 'number', placeholder: 'optional' }],
      args: v => ['tx', ...where.flags(), ...flag('series', v.series1), ...flag('in-serial', v.inSerial), ...(v.tid1 ? [v.tid1] : [])], submit: 'Look up' });
    const addrForm = form(ctx, { title: 'An address', about: 'Balance, holdings, history and pending.', fields: [{ name: 'addr', label: 'Address' }, series, { name: 'limit', label: 'History entries', type: 'number', placeholder: '50' }, ...filt.slice(1, 3)],
      args: v => ['address', ...where.flags(), ...flag('series', v.series1), ...flag('limit', v.limit), ...flag('pool', v.pool), ...flag('serial', v.serial), ...(v.addr ? [v.addr] : [])], submit: 'Look up' });
    // A "look up" link elsewhere opens Explore with #/explore?tid=… or
    // ?address=…, and the form runs as if the user had filled it in.
    const p = ctx.params || new URLSearchParams();
    if (p.get('tid')) setTimeout(() => txForm.run({ tid1: p.get('tid') }));
    else if (p.get('address')) setTimeout(() => addrForm.run({ addr: p.get('address') }));
    return mountAll(root, 'Explore', 'What the node holds, selected with the same filters as Watch. Each is one poo command.', [
      form(ctx, { title: 'Pools', about: 'Followed pools: tip, height, weight, pending.', fields: filt.slice(0, 3),
        args: v => ['pools', ...where.flags(), ...filterFlags(v)], submit: 'Show pools' }),
      ledger,
      form(ctx, { title: 'Pending transactions', fields: filt, args: v => ['pending', ...where.flags(), ...filterFlags(v)], submit: 'Show pending' }),
      form(ctx, { title: 'Unspent portions', fields: filt, args: v => ['unspent', ...where.flags(), ...filterFlags(v)], submit: 'Show unspent' }),
      txForm,
      addrForm,
    ]);
  },
};
