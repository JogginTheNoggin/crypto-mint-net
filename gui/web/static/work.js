// Work: the node's hash grinding (M11, poo grind). Devices, benchmarks,
// the node's jobs and a job run and followed live are all poo grind
// commands; the view only turns fields into flags and draws what the
// commands print. Which device and grinder run, and why, is the plan poo
// prints, never a guess here.
import * as api from './api.js';
import { el, cmdLine, panel, form, flag, mountAll } from './views.js';
import { render } from './render.js';
import * as where from './where.js';
import { paced } from './scroll.js';

// Bounds, as in Watch (GUI-007): events kept, rate points drawn, one line.
const MAX_EVENTS = 500;
const RATE_POINTS = 120;
const MAX_LINE = 1 << 20;
const MAX_ERR = 64 << 10;

// jobFields are poo grind's JOB FLAGS; jobFlags turns their values into
// the flags.
const jobFields = () => [
  { name: 'series', label: 'Series id', placeholder: 'default: every followed series' },
  { name: 'pool', label: 'Pool', type: 'number', placeholder: 'default: every followed pool' },
  { name: 'serial', label: 'Mint serial', type: 'number', placeholder: 'or a mint serial' },
  { name: 'device', label: 'Device', placeholder: 'auto, cpu, gpu:0 (default: node.json, else auto)' },
  { name: 'threads', label: 'Threads', type: 'number', placeholder: 'default: every core but one' },
  { name: 'for', label: 'For', placeholder: 'for example 30s or 10m' },
  { name: 'untilFrom', label: 'Until from reaches', placeholder: 'with an entry hash target' },
  { name: 'hours', label: 'Only in hours', placeholder: 'for example 22-06 (local time)' },
];
const jobFlags = v => [...flag('series', v.series), ...flag('pool', v.pool), ...flag('serial', v.serial),
  ...flag('device', v.device), ...flag('threads', v.threads), ...flag('for', v.for),
  ...flag('until-from', v.untilFrom), ...flag('hours', v.hours)];
const short = h => (typeof h === 'string' && h.length > 16) ? `${h.slice(0, 10)}…${h.slice(-6)}` : (h ?? '');

// rateText shows tries a second the way people read them.
function rateText(r) {
  if (!r) return '';
  for (const [n, u] of [[1e9, 'G'], [1e6, 'M'], [1e3, 'k']]) if (r >= n) return `${(r / n).toFixed(2)} ${u}H/s`;
  return `${r.toFixed(0)} H/s`;
}

// rateChart is the last rates as a line, with no inline styles so the
// page's CSP allows it, coloured by the theme (style.css). It is built
// once; set(points) moves the line (ORANGE-009).
function rateChart() {
  const NS = 'http://www.w3.org/2000/svg';
  const svg = document.createElementNS(NS, 'svg');
  svg.setAttribute('viewBox', '0 0 300 60');
  svg.setAttribute('class', 'rate-chart');
  svg.setAttribute('role', 'img');
  const line = document.createElementNS(NS, 'polyline');
  svg.append(line);
  svg.set = points => {
    let max = 1;
    for (const p of points) if (p > max) max = p;
    svg.setAttribute('aria-label', `Rate over the last ${points.length} progress lines; latest ${rateText(points[points.length - 1])}, highest ${rateText(max)}`);
    const step = points.length > 1 ? 300 / (points.length - 1) : 0;
    line.setAttribute('points', points.map((p, i) => `${(i * step).toFixed(1)},${(58 - (p / max) * 54).toFixed(1)}`).join(' '));
  };
  return svg;
}

// The live run is built once and kept, as Watch is: a running grind and
// its picture survive switching views, and its stream is closed while
// the view is away and followed again from where it stopped (GUI-008).
let livePanel = null;
let liveHooks = { attach: () => {}, detach: () => {} };

function buildLive(ctx) {
  const fields = jobFields();
  const target = el('input', { name: 'target', placeholder: 'tip, pending or an entry hash', spellcheck: 'false', value: 'tip' });
  const inputs = {};
  const f = el('form', { class: 'fields', autocomplete: 'off' },
    el('label', {}, el('span', {}, 'Target'), target,
      el('small', {}, 'tip: layers on each pool\'s selected tip. pending: entries from pending transactions. An entry hash: layers on that entry. The series says which hash to grind.')));
  for (const fd of fields) {
    inputs[fd.name] = el('input', { name: fd.name, placeholder: fd.placeholder, spellcheck: 'false', inputmode: fd.type === 'number' ? 'numeric' : undefined });
    f.append(el('label', {}, el('span', {}, fd.label), inputs[fd.name]));
  }
  const values = () => Object.fromEntries(fields.map(fd => [fd.name, inputs[fd.name].value.trim()]));
  const args = () => ['grind', 'run', ...where.flags(), ...jobFlags(values()), target.value.trim() || 'tip'];
  const line = cmdLine(args(), ctx);
  f.addEventListener('input', () => line.set(args()));
  const runBtn = el('button', { type: 'submit', class: 'primary' }, 'Run and follow');
  const stopBtn = el('button', { type: 'button', disabled: true }, 'Stop');
  f.append(el('div', { class: 'actions' }, runBtn, ' ', stopBtn));

  const state = el('div', { class: 'run-status' }, 'Not running.');
  const plan = el('div', { class: 'grind-plan' });
  const stats = el('div', { class: 'counts hint' });
  const chart = el('div', { class: 'chart-wrap' });
  const found = el('tbody');
  const foundTable = el('table', { class: 'data' },
    el('thead', {}, el('tr', {}, ...['found', 'hash', 'strength', 'from'].map(h => el('th', {}, h)))), found);
  const errOut = el('pre', { class: 'text-out err', hidden: true });

  let job = null, unfollow = null, next = 0, buf = '', rates = [], tries = 0, nFound = 0;
  const setRunning = on => {
    runBtn.disabled = on;
    stopBtn.disabled = !on;
    const tab = document.querySelector('nav a[data-id=work]');
    if (tab) { tab.classList.toggle('busy', on); tab.title = on ? 'A grind is running' : ''; }
  };
  // Lines are read as they arrive and drawn at most ten times a second
  // (ORANGE-009): the newest plan and state, one move of the rate line,
  // the found rows in one fragment, and the counts once.
  const svg = rateChart();
  let newFound = [], newPlan = null, newState = null, chartDirty = false;
  const draw = paced(() => {
    if (newState !== null) { state.textContent = newState; newState = null; }
    if (newPlan) { plan.textContent = ''; plan.append(el('h3', {}, 'Plan'), render(newPlan)); newPlan = null; }
    if (chartDirty) { if (!svg.isConnected) chart.append(svg); svg.set(rates); chartDirty = false; }
    if (newFound.length) {
      const frag = document.createDocumentFragment();
      for (let i = newFound.length - 1; i >= 0; i--) {
        const ev = newFound[i], h = ev.layer || ev.entry;
        frag.append(el('tr', {}, el('td', {}, ev.layer ? 'layer' : 'entry'),
          el('td', { title: h || '' }, el('code', {}, short(h))), el('td', { class: 'num' }, String(ev.strength ?? '')), el('td', {}, ev.from ?? '')));
      }
      newFound = [];
      found.prepend(frag);
      for (let n = found.children.length; n > MAX_EVENTS; n--) found.lastChild.remove();
    }
    stats.textContent = [tries ? `${tries} tries` : '', rates.length ? `rate ${rateText(rates[rates.length - 1])}` : '', `${nFound} found`].filter(Boolean).join(' · ');
  });
  const add = ev => {
    if (ev.state || ev.reason) newState = [ev.kind, ev.state, ev.reason].filter(Boolean).join(': ');
    if (ev.plan) newPlan = ev.plan;
    if (ev.tries) tries = ev.tries;
    if (ev.kind === 'progress' && ev.rate) {
      rates.push(ev.rate);
      if (rates.length > RATE_POINTS) rates.shift();
      chartDirty = true;
    }
    if (ev.kind === 'found') {
      nFound++;
      newFound.push(ev);
      if (newFound.length > 2 * MAX_EVENTS) newFound = newFound.slice(-MAX_EVENTS);
    }
    draw.schedule();
  };
  // errAppend keeps the error box's last MAX_ERR characters, for stderr
  // and for stdout lines that are not JSON alike.
  const errAppend = text => { errOut.hidden = false; errOut.textContent = (errOut.textContent + text).slice(-MAX_ERR); };
  const lineIn = text => {
    if (!text.trim()) return;
    let ev;
    try { ev = JSON.parse(text); } catch (_) { errAppend(text + '\n'); return; }
    add(ev);
  };
  const follow = () => {
    unfollow = api.follow(job.id, {
      chunk: c => {
        next = c.next ?? c.seq + 1;
        if (c.stream === 'err') { errAppend(c.data); return; }
        buf += c.data;
        const lines = buf.split('\n');
        buf = lines.pop();
        lines.forEach(lineIn);
        if (buf.length > MAX_LINE) buf = '';
      },
      dropped: d => { next = d.next; buf = ''; },
      exit: x => { draw.now(); unfollow = null; setRunning(false); state.textContent += ` (exit ${x.code})`; job = null; },
    }, next);
  };
  f.addEventListener('submit', async e => {
    e.preventDefault();
    const a = args();
    line.set(a);
    errOut.hidden = true;
    errOut.textContent = '';
    plan.textContent = chart.textContent = found.textContent = '';
    rates = []; tries = 0; nFound = 0; next = 0; buf = '';
    newFound = []; newPlan = newState = null; chartDirty = false;
    try { job = await api.start(a); } catch (err) { state.textContent = err.message; return; }
    setRunning(true);
    state.textContent = `running: ${job.command}`;
    follow();
  });
  // Stop is the job's interrupt: poo grind run stops the node's job and
  // exits 0, as Ctrl-C does in a shell.
  stopBtn.onclick = () => { if (job) api.cancel(job.id).catch(() => {}); };
  liveHooks = {
    detach: () => { if (unfollow) { unfollow(); unfollow = null; } },
    attach: () => { if (job && !unfollow) follow(); },
  };
  return el('section', { class: 'panel' },
    el('div', { class: 'panel-head' }, el('h2', {}, 'Run a job')),
    el('p', { class: 'hint' }, 'Starts a job in the running node with poo grind run and follows it: the plan poo picked and why, the rate, and each layer or entry found. It keeps running when you switch views; Stop ends it.'),
    f, line, state, plan, stats, chart, errOut, el('div', { class: 'table-wrap' }, foundTable));
}

export const workView = {
  id: 'work',
  title: 'Work',
  mount(root, ctx) {
    if (!livePanel) livePanel = buildLive(ctx);
    else liveHooks.attach();
    return mountAll(root, 'Work', 'The node\'s hash grinding: which devices it has, how fast each is, and the jobs it runs. Each part is a poo grind command; node.json\'s grind section sets the defaults and the jobs the node runs by itself.', [
      panel(ctx, { title: 'Devices', about: 'Each device, what it grinds fast, and which one auto picks for each hash.', args: () => ['grind', 'devices', ...where.flags()] }),
      panel(ctx, { title: 'Jobs', about: 'The node\'s grind jobs, running and recently ended: yours and the ones node.json schedules.', args: () => ['grind', 'jobs', ...where.flags()], refresh: 5000 }),
      { el: livePanel, stop: () => { liveHooks.detach(); livePanel.remove(); } },
      form(ctx, { title: 'Start a job in the background', about: 'The same job, left to the node: it shows under Jobs.', fields: [{ name: 'target', label: 'Target', value: 'tip' }, ...jobFields()],
        args: v => ['grind', 'start', ...where.flags(), ...jobFlags(v), v.target || 'tip'], submit: 'Start' }),
      form(ctx, { title: 'Stop a job', fields: [{ name: 'id', label: 'Job id', type: 'number' }, { name: 'all', label: 'Every job', type: 'check' }],
        args: v => ['grind', 'stop', ...where.flags(), ...(v.all ? ['-all'] : v.id ? [v.id] : [])], submit: 'Stop', danger: true }),
      form(ctx, { title: 'Benchmark', about: 'How fast each device grinds a series\' hash, on this machine. Runs without a node.', fields: [{ name: 'series', label: 'Series id', placeholder: 'default: every hash' }, { name: 'device', label: 'Device', placeholder: 'auto, cpu, gpu:0' }, { name: 'threads', label: 'Threads', type: 'number' }, { name: 'seconds', label: 'Seconds', type: 'number', placeholder: 'per hash' }],
        args: v => ['grind', 'bench', ...where.flags(), ...flag('series', v.series), ...flag('device', v.device), ...flag('threads', v.threads), ...flag('seconds', v.seconds)], submit: 'Benchmark' }),
    ]);
  },
};
