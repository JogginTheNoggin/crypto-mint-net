// Work: the node's hash grinding (M11, poo grind). Devices, benchmarks,
// the node's jobs and a job run and followed live are all poo grind
// commands; the view only turns fields into flags and draws what the
// commands print. Which device and grinder run, and why, is the plan poo
// prints, never a guess here.
import * as api from './api.js';
import { el, cmdLine, panel, form, flag, mountAll, gate } from './views.js';
import { render, rateText } from './render.js';
import * as where from './where.js';
import { paced } from './scroll.js';

// Bounds, as in Watch (GUI-007): events kept, rate points drawn, one line.
const MAX_EVENTS = 500;
const RATE_POINTS = 120;
const MAX_LINE = 1 << 20;
const MAX_ERR = 64 << 10;

// noUntil are the targets poo grind refuses -until-from with (an empty
// target is tip), so the job forms hide that field for them (GREEN-032).
const noUntil = ['', 'tip', 'pending'];

// jobFields are poo grind's JOB FLAGS; jobFlags turns their values into
// the flags.
const jobFields = () => [
  { name: 'series', label: 'Series id', placeholder: 'default: every followed series' },
  { name: 'pool', label: 'Pool', type: 'number', placeholder: 'default: every followed pool' },
  { name: 'serial', label: 'Mint serial', type: 'number', placeholder: 'or a mint serial' },
  { name: 'device', label: 'Device', placeholder: 'auto, cpu, gpu:0 (default: node.json, else auto)' },
  { name: 'threads', label: 'Threads', type: 'number', placeholder: 'default: every core but one' },
  { name: 'for', label: 'For', placeholder: 'for example 30s or 10m' },
  { name: 'untilFrom', label: 'Until from reaches', placeholder: 'a weight; a tx:TID target needs one', only: { field: 'target', not: noUntil } },
  { name: 'hours', label: 'Only in hours', placeholder: 'for example 22-06 (local time)' },
];
const jobFlags = v => [...flag('series', v.series), ...flag('pool', v.pool), ...flag('serial', v.serial),
  ...flag('device', v.device), ...flag('threads', v.threads), ...flag('for', v.for),
  ...flag('until-from', v.untilFrom), ...flag('hours', v.hours)];

// jobsTable lays out poo grind jobs as one row per job: what it works on,
// its state and the reason poo gives, the device and grinder of its plans,
// its counts and rate, and a Stop button for a job still going. Stop runs
// poo grind stop ID, as the Stop a job form does. Every value is printed.
function jobsTable(list, ctx, refresh) {
  if (!Array.isArray(list) || !list.length) return el('p', { class: 'hint empty' }, 'No jobs yet. Run one below, or let node.json\'s grind section start them.');
  const body = el('tbody');
  for (const j of list.slice().reverse()) {
    const s = j.spec || {};
    const what = [s.target, s.pools && s.pools.length ? `pools ${s.pools.join(',')}` : '', s.hours ? `hours ${s.hours}` : '', s.for ? `for ${s.for}` : ''].filter(Boolean).join(' · ');
    const plans = (j.plans || []).map(p => `${p.device} ${p.grinder} (${p.hash}, ${p.threads} threads)`).join('; ');
    const going = j.state === 'running' || j.state === 'waiting';
    const stop = going ? el('button', { type: 'button', class: 'small danger', onclick: async () => {
      stop.disabled = true;
      try { await api.run(['grind', 'stop', ...where.flags(), String(j.id)]); } finally { refresh(); }
    } }, 'Stop') : null;
    body.append(el('tr', {},
      el('td', { class: 'num' }, String(j.id)),
      el('td', {}, what, s.source ? el('div', { class: 'hint' }, `from ${s.source}`) : null),
      el('td', {}, el('span', { class: `pill pill-${going ? (j.state === 'running' ? 'ok' : 'warn') : j.state === 'failed' ? 'fail' : 'skip'}` }, j.state), j.reason && j.reason !== j.state ? el('div', { class: 'hint' }, j.reason) : null, j.hint ? el('div', { class: 'hint' }, j.hint) : null),
      el('td', {}, plans || el('span', { class: 'nil' }, '—')),
      el('td', { class: 'num' }, String(j.tries ?? 0)),
      el('td', {}, render(j.rate ?? 0, 0, 'rate')),
      el('td', { class: 'num' }, `${j.layers ?? 0} / ${j.entries ?? 0}`),
      el('td', {}, stop)));
  }
  return el('div', { class: 'table-wrap' }, el('table', { class: 'data grind-jobs' },
    el('thead', {}, el('tr', {}, ...['id', 'works on', 'state', 'device and grinder', 'tries', 'rate', 'layers / entries', ''].map(h => el('th', {}, h)))), body));
}

// scheduleTable lays out poo grind schedule's reply: the defaults, and
// one row per job node.json starts, numbered as poo numbers them, each
// with a Remove that runs poo grind schedule remove -target T N. A running node
// applies the change at once; without one poo edits node.json.
function scheduleTable(reply, edits, reload) {
  const g = (reply && reply.grind) || {};
  const layers = g.layer_floor ? `, keep layers of strength ${g.layer_floor} or more, at most ${g.layer_rate || 'the default'} a second` : '';
  const defaults = el('p', { class: 'counts hint' }, `Defaults: device ${g.device || 'auto'}, ${g.threads ? `${g.threads} thread${g.threads === 1 ? '' : 's'}` : 'every core but one'}${layers}.`);
  const failed = reply && reply.failed ? Object.entries(reply.failed).map(([i, why]) => el('p', { class: 'hint err' }, `Job ${Number(i) + 1} did not start: ${why}`)) : [];
  const jobs = g.jobs || [];
  if (!jobs.length) return el('div', {}, defaults, ...failed, el('p', { class: 'hint empty' }, 'No scheduled jobs. Add one below.'));
  const body = el('tbody');
  jobs.forEach((j, i) => {
    // A Remove holds the view's schedule gate, so every Remove, Add and
    // Set is off until the table is drawn again from the schedule poo then
    // reads: no click goes by a number the removal has already shifted
    // (GUI-013), and no change starts from a schedule another is about to
    // replace (GUI-014). -target makes poo refuse a number that no longer
    // names this row's job, as when the schedule changed elsewhere
    // (GREEN-031); the table is read again either way.
    const remove = edits.add(el('button', { type: 'button', class: 'small danger', onclick: async () => {
      if (!edits.take()) return;
      try { await api.run(['grind', 'schedule', 'remove', ...where.flags(), '-target', j.target, String(i + 1)]); } finally {
        await reload();
        edits.release();
      }
    } }, 'Remove'));
    body.append(el('tr', {},
      el('td', { class: 'num' }, String(i + 1)),
      el('td', {}, j.target, j.until_from ? el('div', { class: 'hint' }, `until from reaches ${j.until_from}`) : null),
      el('td', {}, j.series ? render(j.series) : el('span', { class: 'nil' }, 'every followed')),
      el('td', {}, j.pools && j.pools.length ? j.pools.join(', ') : el('span', { class: 'nil' }, 'every followed')),
      el('td', {}, j.device || el('span', { class: 'nil' }, 'default')),
      el('td', { class: 'num' }, j.threads ? String(j.threads) : el('span', { class: 'nil' }, 'default')),
      el('td', {}, j.hours || el('span', { class: 'nil' }, 'always')),
      el('td', {}, remove)));
  });
  return el('div', {}, defaults, ...failed, el('div', { class: 'table-wrap' }, el('table', { class: 'data grind-schedule' },
    el('thead', {}, el('tr', {}, ...['#', 'target', 'series', 'pools', 'device', 'threads', 'hours', ''].map(h => el('th', {}, h)))), body)));
}

// tableResult swaps a panel's printed JSON for the table draw makes of
// it, keeping the output as printed under a fold.
function tableResult(r, area, draw) {
  if (r.code !== 0) return;
  let v;
  try { v = JSON.parse(r.out); } catch (_) { return; }
  const raw = area.querySelector('.result');
  if (!raw) return;
  const det = el('details', { class: 'raw' }, el('summary', {}, 'Output as printed'));
  raw.replaceWith(draw(v));
  det.append(raw);
  area.append(det);
}

// deviceChoices offers the device ids poo grind devices printed, plus
// auto, to every Device field in the view. A field still takes any text:
// poo decides what is valid.
function deviceChoices(root, out) {
  let devs;
  try { devs = JSON.parse(out).devices || []; } catch (_) { return; }
  let dl = document.getElementById('grind-devices');
  if (!dl) { dl = el('datalist', { id: 'grind-devices' }); document.body.append(dl); }
  dl.textContent = '';
  for (const d of [{ id: 'auto', name: 'pick for each hash' }, ...devs]) dl.append(el('option', { value: d.id }, d.name || d.id));
  for (const i of root.querySelectorAll('input[name=device]')) i.setAttribute('list', 'grind-devices');
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

// liveJob finds the poo grind run this GUI is running, if any, the way the
// Node view finds poo node.
async function liveJob() {
  const js = await api.jobs();
  return js.reverse().find(j => !j.done && j.args[0] === 'grind' && j.args[1] === 'run');
}
let liveHooks = { attach: () => {}, detach: () => {} };

function buildLive(ctx) {
  const fields = jobFields();
  const target = el('input', { name: 'target', placeholder: 'tip, pending, tx:TID or an entry hash', spellcheck: 'false', value: 'tip' });
  const inputs = {};
  const f = el('form', { class: 'fields', autocomplete: 'off' },
    el('label', {}, el('span', {}, 'Target'), target,
      el('small', {}, 'tip: layers on each pool\'s selected tip. pending: entries from pending transactions. tx:TID: one payment, until from reaches the weight below. An entry hash: layers on that entry. The series says which hash to grind.')));
  const labels = {};
  for (const fd of fields) {
    inputs[fd.name] = el('input', { name: fd.name, placeholder: fd.placeholder, spellcheck: 'false', inputmode: fd.type === 'number' ? 'numeric' : undefined });
    f.append(labels[fd.name] = el('label', {}, el('span', {}, fd.label), inputs[fd.name]));
  }
  // The same rule as form's only: a field hidden for this target is not sent.
  const shown = fd => !fd.only || !fd.only.not.includes(target.value.trim());
  const showOnly = () => { for (const fd of fields) labels[fd.name].hidden = !shown(fd); };
  showOnly();
  f.addEventListener('input', showOnly);
  const values = () => Object.fromEntries(fields.map(fd => [fd.name, shown(fd) ? inputs[fd.name].value.trim() : '']));
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
  // A job prints one start (its spec) and a plan for each pool's hash;
  // the view keeps the newest plan of each pool.
  let newFound = [], newPlan = false, newState = null, chartDirty = false, spec = null, plans = new Map();
  const draw = paced(() => {
    if (newState !== null) { state.textContent = newState; newState = null; }
    if (newPlan) {
      plan.textContent = '';
      if (spec) plan.append(el('h3', {}, 'Job'), render(spec));
      if (plans.size) plan.append(el('h3', {}, 'Plan'), render([...plans.values()]));
      newPlan = false;
    }
    if (chartDirty) { if (!svg.isConnected) chart.append(svg); svg.set(rates); chartDirty = false; }
    if (newFound.length) {
      const frag = document.createDocumentFragment();
      for (let i = newFound.length - 1; i >= 0; i--) {
        const ev = newFound[i], h = ev.layer || ev.entry;
        frag.append(el('tr', {}, el('td', {}, ev.layer ? 'layer' : 'entry'),
          el('td', {}, h ? render(h) : ''), el('td', { class: 'num' }, String(ev.strength ?? '')), el('td', {}, ev.from ?? '')));
      }
      newFound = [];
      found.prepend(frag);
      for (let n = found.children.length; n > MAX_EVENTS; n--) found.lastChild.remove();
    }
    stats.textContent = [tries ? `${tries} tries` : '', rates.length ? `rate ${rateText(rates[rates.length - 1])}` : '', `${nFound} found`].filter(Boolean).join(' · ');
  });
  const add = ev => {
    if (ev.state || ev.reason || ev.hint) newState = [ev.kind, ev.state, ev.reason, ev.hint].filter(Boolean).join(': ');
    if (ev.kind === 'start' && ev.spec) { spec = ev.spec; newPlan = true; }
    if (ev.plan) {
      plans.set(`${ev.series ?? ''}/${ev.pool ?? ''}`, { series: ev.series, pool: ev.pool, ...ev.plan });
      if (plans.size > MAX_EVENTS) plans.delete(plans.keys().next().value);
      newPlan = true;
    }
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
    newFound = []; newPlan = false; newState = null; chartDirty = false; spec = null; plans = new Map();
    try { job = await api.start(a); } catch (err) { state.textContent = err.message; return; }
    setRunning(true);
    state.textContent = `running: ${job.command}`;
    follow();
  });
  // Stop is the job's interrupt: poo grind run stops the node's job and
  // exits 0, as Ctrl-C does in a shell.
  stopBtn.onclick = () => { if (job) api.cancel(job.id).catch(() => {}); };
  // A grind run this GUI is already running, started before the page was
  // reloaded, is taken up again: shown from its start, with its Stop, so
  // Run cannot start a second one beside it (GUI-010). Run is off until
  // the job list has answered, so a Run pressed in that moment cannot
  // start one either (GUI-011's notes).
  runBtn.disabled = true;
  liveJob().catch(() => null).then(j => {
    if (!j) { runBtn.disabled = false; return; }
    job = j;
    setRunning(true);
    state.textContent = `running: ${job.command}`;
    // Followed only while the view shows; otherwise attach follows it
    // when the view is opened (ORANGE-010).
    if (stats.isConnected) follow();
  });
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
    // The jobs list is drawn as a table with a Stop for each job still
    // going; the output as printed stays under a fold.
    const jobsPanel = panel(ctx, { title: 'Jobs', about: 'The node\'s grind jobs, running and recently ended: yours and the ones node.json schedules.', args: () => ['grind', 'jobs', ...where.flags()], refresh: 5000,
      onResult: (r, area) => tableResult(r, area, list => jobsTable(list, ctx, () => jobsPanel.el.querySelector('.panel-head button').click())) });
    // The schedule is node.json's grind section, edited only through poo
    // grind schedule (GREEN-026): the GUI never writes node.json itself.
    // One schedule change runs at a time, and the next waits until the
    // table shows its result: poo grind schedule reads the section, edits
    // it and writes it whole, so two at once would lose one (GUI-014).
    const edits = gate();
    const reloadSchedule = () => schedulePanel.reload();
    const schedulePanel = panel(ctx, { title: 'Schedule', about: 'The jobs the node runs by itself, from node.json\'s grind section, and the defaults every job, build and work takes. A running node applies a change at once and keeps your own jobs running.', args: () => ['grind', 'schedule', ...where.flags()],
      onResult: (r, area) => tableResult(r, area, v => { nowDefaults(v); return scheduleTable(v, edits, reloadSchedule); }) });
    // The defaults form shows what is set now, as each field's placeholder:
    // a field left empty keeps it (GREEN-033).
    const nowDefaults = v => {
      const g = (v && v.grind) || {};
      defaultsForm.hint('device', `now ${g.device || 'auto'}`);
      defaultsForm.hint('threads', `now ${g.threads || '0: every core but one'}`);
      defaultsForm.hint('floor', `now ${g.layer_floor || '0: only the strongest of each search'} (0 to 256)`);
      defaultsForm.hint('rate', `now ${g.layer_rate || '0: the default'} layers a second at most`);
    };
    const afterSchedule = () => reloadSchedule();
    const defaultsForm = form(ctx, { title: 'Schedule defaults', about: 'The device and threads every job takes unless it names its own, and which layers grinding keeps: poo grind schedule set.', fields: [{ name: 'device', label: 'Device', placeholder: 'auto, cpu, gpu:0' }, { name: 'threads', label: 'Threads', type: 'number', placeholder: '0: every core but one' },
        { name: 'floor', label: 'Layer floor', type: 'number', placeholder: '0 to 256; 0: only the strongest of each search' }, { name: 'rate', label: 'Layer rate', type: 'number', placeholder: 'most layers a second the floor adds (0: default)' }],
        args: v => ['grind', 'schedule', 'set', ...where.flags(), ...flag('device', v.device), ...flag('threads', v.threads), ...flag('layer-floor', v.floor), ...flag('layer-rate', v.rate)], submit: 'Set', onResult: afterSchedule, gate: edits });
    return mountAll(root, 'Work', 'The node\'s hash grinding: which devices it has, how fast each is, and the jobs it runs. Each part is a poo grind command; node.json\'s grind section sets the defaults and the jobs the node runs by itself.', [
      panel(ctx, { title: 'Devices', about: 'Each device, what it grinds fast, and which one auto picks for each hash.', args: () => ['grind', 'devices', ...where.flags()],
        onResult: r => { if (r.code === 0) deviceChoices(root, r.out); } }),
      jobsPanel,
      schedulePanel,
      form(ctx, { title: 'Schedule a job', about: 'Adds a job to node.json\'s grind section with poo grind schedule add. It has no For: it runs whenever its hours allow.', fields: [{ name: 'target', label: 'Target', value: 'tip', placeholder: 'tip, pending, tx:TID or an entry hash' }, ...jobFields().filter(fd => fd.name !== 'for')],
        args: v => ['grind', 'schedule', 'add', ...where.flags(), v.target || 'tip', ...jobFlags(v)], submit: 'Add', onResult: afterSchedule, gate: edits }),
      defaultsForm,
      { el: livePanel, stop: () => { liveHooks.detach(); livePanel.remove(); } },
      form(ctx, { title: 'Start a job in the background', about: 'The same job, left to the node: it shows under Jobs.', fields: [{ name: 'target', label: 'Target', value: 'tip', placeholder: 'tip, pending, tx:TID or an entry hash' }, ...jobFields()],
        args: v => ['grind', 'start', ...where.flags(), ...jobFlags(v), v.target || 'tip'], submit: 'Start' }),
      form(ctx, { title: 'Stop a job', fields: [{ name: 'id', label: 'Job id', type: 'number' }, { name: 'all', label: 'Every job', type: 'check' }],
        args: v => ['grind', 'stop', ...where.flags(), ...(v.all ? ['-all'] : v.id ? [v.id] : [])], submit: 'Stop', danger: true }),
      form(ctx, { title: 'Benchmark', about: 'How fast each device grinds a series\' hash, on this machine. Runs without a node.', fields: [{ name: 'series', label: 'Series id', placeholder: 'default: every hash' }, { name: 'device', label: 'Device', placeholder: 'auto, cpu, gpu:0' }, { name: 'threads', label: 'Threads', type: 'number' }, { name: 'seconds', label: 'Seconds', type: 'number', placeholder: 'per hash' }],
        args: v => ['grind', 'bench', ...where.flags(), ...flag('series', v.series), ...flag('device', v.device), ...flag('threads', v.threads), ...flag('seconds', v.seconds)], submit: 'Benchmark' }),
    ]);
  },
};
