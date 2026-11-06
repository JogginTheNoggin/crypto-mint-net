// The Node view. Each view is one file here, listed in index.js (GUI-GREEN-044).
import * as api from '../api.js';
import * as where from '../where.js';
import { appliedNote, cmdLine, el, flag, flags, form, mountAll, panel, pickFile, rawFold, rowsTable } from '../views.js';
import { scrollback } from '../scroll.js';
import { parseJSON, render } from '../render.js';

// node finds the node job this GUI is running, if any.
async function nodeJob() {
  const js = await api.jobs();
  return js.reverse().find(j => !j.done && j.args[0] === 'node');
}

// doctorReport lays out poo doctor -o json: one row per check, as printed.
// A check whose fix is a poo command line (fix_args, D110) gets a button
// that types it into the terminal exactly as poo printed it: poo already
// names the checked policy's -dir when Settings chose -root or -policy
// (GREEN-036), so the view adds nothing. Any other fix is shown as the
// text poo printed.
const RUNNABLE_FIX = /^poo [a-z][\w./:=-]*( [\w./:=-]+)*$/;
// DETAIL_CHARS is how much of a doctor check's detail shows at first. A
// detail names what it found, and one can name every followed series: at
// 4,096 series with no witness ceiling, the ceiling check's detail was
// 455,000 characters, and the Health check, the Node view's first panel,
// 52,000 px tall (ORANGE-025). The rest is one press away, as printed.
const DETAIL_CHARS = 1000;

// shortDetail shows the first DETAIL_CHARS characters of text, and all
// of it while "Show all" is open.
export function shortDetail(text) {
  if (text.length <= DETAIL_CHARS) return el('span', {}, text);
  const cut = `${text.slice(0, DETAIL_CHARS)}…`;
  const shown = document.createTextNode(cut);
  const more = el('details', { class: 'doctor-more' }, el('summary', {}, `Show all ${text.length} characters`));
  more.addEventListener('toggle', () => { shown.data = more.open ? text : cut; });
  return el('span', { class: 'doctor-detail-cut' }, shown, more);
}

export function doctorReport(rep, ctx) {
  const list = el('ul', { class: 'doctor' });
  for (const c of rep.checks || []) {
    let run = null;
    if (Array.isArray(c.fix_args) && c.fix_args.length) run = c.fix_args;
    else if (c.fix && RUNNABLE_FIX.test(c.fix)) run = c.fix.replace(/^poo /, '').split(' ');
    const fix = c.fix ? (run
      ? el('div', { class: 'doctor-fix' }, 'fix: ', el('code', {}, api.quote(run)), ' ',
        el('button', { type: 'button', class: 'small', onclick: () => ctx.openInTerminal(api.quote(run).replace(/^poo /, '')) }, 'Run in terminal'))
      : el('div', { class: 'doctor-fix' }, `fix: ${c.fix}`)) : null;
    list.append(el('li', { class: `doctor-${c.result}` },
      el('span', { class: `pill pill-${c.result}` }, c.result),
      el('strong', {}, c.name), ' ', shortDetail(String(c.detail || '')), fix));
  }
  return el('div', { class: 'result' }, rep.dir ? el('p', { class: 'hint' }, `Data directory: ${rep.dir}`) : null, list);
}

export const nodeView = {
  id: 'node',
  title: 'Node',
  mount(root, ctx) {
    const runArea = el('div', { class: 'area' });
    const startBtn = el('button', { type: 'button', class: 'primary' }, 'Start node');
    // -reverify (D178): this start ignores the pool snapshots and replays
    // every log whole, then poo status says whether the replay matched.
    const reverify = el('input', { type: 'checkbox', name: 'reverify' });
    const nodeArgs = () => ['node', ...where.flags(), ...flag('reverify', reverify.checked)];
    const runLine = cmdLine(nodeArgs(), ctx);
    reverify.onchange = () => runLine.set(nodeArgs());
    const runSec = el('section', { class: 'panel' },
      el('div', { class: 'panel-head' }, el('h2', {}, 'Run the node'), startBtn),
      el('p', { class: 'hint' }, 'Runs poo node inside the GUI until you stop it or close the GUI. A node already running elsewhere (for example as a service) is used as it is: the views below talk to whichever node serves this node policy.'),
      el('label', { class: 'check reverify' }, reverify, ' Check every record again on this start: ignore the pool snapshots and replay each pool log whole (slower; Followed pools then says whether the replay matched the snapshots)'),
      runLine, runArea);
    let following = null, unfollowLog = null, gone = false;
    const showRunning = async () => {
      if (document.hidden && following) return;
      const j = await nodeJob().catch(() => null);
      if (gone) return;
      startBtn.disabled = !!j;
      if (j && following !== j.id) {
        following = j.id;
        runArea.textContent = '';
        const log = el('pre', { class: 'live-out' });
        const logOut = scrollback(log, { follow: true });
        const stop = el('button', { type: 'button', class: 'danger' }, 'Stop node');
        stop.onclick = () => api.cancel(j.id);
        runArea.append(el('div', { class: 'run-bar' }, el('span', { class: 'run-status live' }, `running: ${j.command}`), stop), log);
        if (unfollowLog) unfollowLog();
        unfollowLog = api.follow(j.id, {
          chunk: c => logOut.write(c.data, c.stream === 'err' ? 'err' : ''),
          exit: e => { following = null; unfollowLog = null; startBtn.disabled = false; runArea.prepend(el('div', { class: 'run-status ' + (e.code ? 'bad' : 'ok') }, `node stopped (exit ${e.code})`)); stop.remove(); },
        });
      }
    };
    startBtn.onclick = async () => {
      startBtn.disabled = true;
      const a = nodeArgs();
      runLine.set(a);
      try { await api.start(a); } catch (e) { runArea.textContent = e.message; }
      setTimeout(showRunning, 200);
    };
    showRunning();
    // poo status says when node.json changed since the node loaded it and
    // the change waits for a reload (Source GREEN-037). The panel says so
    // above what it shows, with the Reload form's own button (GREEN-038).
    // The 5 s refresh asks for poo's totals per series (-counts, D177), not a
    // row per pool: on the reserve series that was 352 KB and a 150 ms pause
    // every 5 s (ORANGE-021). poo adds them up; the GUI totals nothing.
    // Each pool's row is in Each pool below, which runs only when asked.
    const status = panel(ctx, { title: 'Followed pools', about: 'What the running node follows: totals for each series (pools, entries, branches, pending, waiting, stored records), with the entry cache and file limit notes. Refreshes every 5 seconds.', args: () => ['status', '-counts', ...where.flags()], refresh: 5000,
      onResult: (r, area) => {
        let st;
        try { st = r.code === 0 && JSON.parse(r.out); } catch (_) { return; }
        if (!st) return;
        // Where entry bodies are (D172) and the open file limit
        // (ORANGE-037), as poo status gives them; the GUI judges nothing.
        const notes = statusNotes(st);
        if (notes.length) area.prepend(...notes);
        // The totals poo printed, one row per series, naming a series built
        // into poo by its builtin field (D179). The notes above already say
        // what bodies, file_limit and snapshots hold, so the rest stays
        // folded under "Output as printed".
        const raw = area.querySelector('.result');
        if (raw && Array.isArray(st.series)) { raw.replaceWith(totalsTable(st.series)); area.append(rawFold(raw)); }
        if (!st.policy_changed) return;
        area.prepend(el('p', { class: 'next-step policy-changed', role: 'status' },
          'The node policy file has changed since the node loaded it, and the change waits for a reload. ',
          el('button', { type: 'button', class: 'small', onclick: () => reload.run({}) }, 'Reload now')));
      } });
    const init = form(ctx, {
      title: 'Write a node policy',
      about: 'Creates the node policy (settings, series to follow) and makes it current.',
      fields: [
        { name: 'example', label: 'Follow the worked example series', type: 'check' },
        { name: 'series', label: 'Series files', type: 'list', placeholder: 'path/to/series.json …', help: 'Series parameter files (poo series new) to know and follow.' },
        { name: 'listen', label: 'RPC address', placeholder: 'default; "off" to disable' },
        { name: 'p2p', label: 'P2P address', placeholder: 'default; "off" to accept none' },
        { name: 'peer', label: 'Peers', type: 'list', placeholder: 'host:port …' },
        { name: 'registry', label: 'Address registries', type: 'list', placeholder: 'URL or file …' },
        { name: 'advertise', label: 'Advertise as', placeholder: 'host:port' },
        // With no series, poo follows the built-in reserve series (D170).
        { name: 'reserve', label: 'Also follow the built-in default series (what happens anyway when no series is given)', type: 'check' },
        { name: 'noSeries', label: 'Follow no series, not even the built-in default series', type: 'check' },
        // Opt-in port mapping (D182): poo asks the router to forward the
        // peer port. Off unless chosen; the help says what peers learn.
        { name: 'portMap', label: 'Ask the router to forward the peer port', type: 'select', options: [{ value: '', label: 'no (default off)' }, 'auto', 'pcp', 'natpmp', 'upnp'], help: 'Lets peers reach a node at home behind a router. The cost: once a dial reaches this node, peers learn this network\'s public IP address. Followed pools shows how the mapping went.' },
        { name: 'anyCeiling', label: 'Accept a series file whose witness ceiling is not set or above 4 MiB', type: 'check' },
        { name: 'force', label: 'Replace the settings of an existing policy (its data is kept)', type: 'check' },
      ],
      args: v => ['init', ...where.flags(), ...flag('example', v.example), ...flags('series', v.series), ...flag('listen', v.listen),
        ...flag('p2p', v.p2p), ...flags('peer', v.peer), ...flags('registry', v.registry), ...flag('advertise', v.advertise), ...flag('port-map', v.portMap), ...flag('default-series', v.reserve), ...flag('no-series', v.noSeries), ...flag('any-ceiling', v.anyCeiling), ...flag('force', v.force)],
      submit: 'Write policy',
    });
    const followFields = [
      { name: 'issuer', label: 'Issuer id', placeholder: 'every series of an issuer' },
      { name: 'series', label: 'Series id' },
      { name: 'pool', label: 'Pools', type: 'list', placeholder: 'K …' },
      { name: 'mint', label: 'Mints', type: 'list', placeholder: 'serial …' },
    ];
    const followArgs = (cmd, v) => [cmd, ...where.flags(), ...flag('issuer', v.issuer), ...flag('series', v.series), ...flags('pool', v.pool), ...flags('mint', v.mint), ...flag('replace', v.replace), ...flag('no-reload', v.noReload)];
    // Each pool and the follow list print a row for every pool: on the
    // reserve series 350 KB and 410 KB, over 1,000 rows and 23,000 page
    // elements each time the Node view opened. Each pool now runs when
    // Show is pressed; the follow list shows its rules at once and draws
    // its pools only when they are opened (GUI-GREEN-056).
    const eachPool = panel(ctx, { title: 'Each pool', about: 'Where each followed pool stands, one row per pool, as poo status prints it. It runs when you press Show, and does not refresh by itself.', args: () => ['status', ...where.flags()], start: false });
    const follows = panel(ctx, { title: 'What the node follows', about: 'The follow list of the node policy, and the pools it covers.', args: () => ['follow', ...where.flags()],
      onResult: (r, area) => {
        let o;
        try { o = r.code === 0 && parseJSON(r.out); } catch (_) { return; }
        const raw = area.querySelector('.result');
        if (!o || !raw || !Array.isArray(o.pools)) return;
        raw.replaceWith(followView(o));
        area.append(rawFold(raw));
      } });
    // Replace (GREEN-038, Source) is poo follow -replace: with a series, the
    // pools and mints given become all it follows of that series, in one
    // change. It shows only once a series is named, as poo requires.
    const follow = form(ctx, { title: 'Follow more', about: 'Adds to the node policy; a running node applies it at once.',
      fields: [...followFields, { name: 'replace', label: 'Replace what the series follows with these', type: 'check', only: { field: 'series', not: [''] } }, { name: 'noReload', label: 'Only write the policy (no reload)', type: 'check' }], args: v => followArgs('follow', v), submit: 'Follow' });
    const unfollow = form(ctx, { title: 'Follow less', fields: [...followFields, { name: 'noReload', label: 'Only write the policy (no reload)', type: 'check' }],
      args: v => followArgs('unfollow', v), submit: 'Unfollow', danger: true });
    const reload = form(ctx, { title: 'Reload the node policy', about: 'Makes the running node apply its node policy file as it now stands.', fields: [],
      args: () => ['reload', ...where.flags()], submit: 'Reload', onResult: r => { if (r.code === 0) status.reload(); } });
    // Variable supply series the running node follows (D166): what poo
    // says it has released, the next part, the mints and units so far and
    // the newest release it holds. The GUI only lays out poo's answer.
    const variable = panel(ctx, { title: 'Variable series', about: 'Each followed series that can grow: the parts released so far, the next part, the mints and units that exist now, and the newest release the node holds.', args: () => ['series', 'followed', ...where.flags()],
      onResult: (r, area) => {
        let o;
        try { o = r.code === 0 && parseJSON(r.out); } catch (_) { return; }
        const all = o && Array.isArray(o.series) ? o.series : null;
        if (!all) return;
        const raw = area.querySelector('.result');
        const vs = all.filter(x => x && x.variable);
        const cells = x => [el('code', { class: 'hex', title: x.id }, String(x.id).slice(0, 16) + '…'),
          ...[x.variable.released, x.variable.next_part ?? 'none (the cap is reached)', x.variable.mints, x.variable.units, x.variable.release ? `part ${x.variable.release.part}` : 'none: part 1 only'].map(String)];
        const body = vs.length ? variableTable(vs, cells) :
          el('p', { class: 'hint empty' }, `None of the ${all.length} followed series is variable.`);
        if (raw) { raw.replaceWith(body); area.append(rawFold(raw)); }
      } });
    // Every series the running node follows, one row each, as poo series
    // followed gives them: a newcomer on the default sees "the built-in
    // reserve series" by name (poo's builtin, D170), not only a hash and
    // 1,024 pool rows. The GUI only lays out poo's answer
    // (GUI-GREEN-054).
    const followed = panel(ctx, { title: 'Followed series', about: 'Each series the running node follows: its name when it is built into poo, its id, whether its supply is fixed or can grow, and its pools, mints, units per mint and floors.', args: () => ['series', 'followed', ...where.flags()],
      onResult: (r, area) => {
        let o;
        try { o = r.code === 0 && parseJSON(r.out); } catch (_) { return; }
        const all = o && Array.isArray(o.series) ? o.series : null;
        const raw = area.querySelector('.result');
        if (!all || !raw) return;
        const body = all.length ? seriesTable(all) : el('p', { class: 'hint empty' }, 'The node follows no series. Write a node policy below, or Follow more.');
        raw.replaceWith(body); area.append(rawFold(raw));
      } });
    // A release made by the issuer reaches a node as a file (D166): poo
    // checks it, keeps it in node.json and the node reloads to follow the
    // new parts' mints. A file picked here goes to poo as its input (-).
    let picker = null;
    const addRelease = form(ctx, { title: 'Add a release', about: 'Gives the node a release of a variable series it follows. poo checks it against the series, saves it in node.json, and the node then follows the new parts\' mints. A release can never be taken back.',
      fields: [{ name: 'file', label: 'Release file', placeholder: 'a path on this machine, or pick one below', required: true }],
      args: v => ['series', 'add-release', ...where.flags(), ...(v.file ? [v.file] : [])], submit: 'Add release', danger: true,
      onStart: j => picker.feed(j),
      onResult: (r, area) => { appliedNote(r, area); if (r.code === 0) { variable.reload(); status.reload(); } } });
    picker = pickFile(addRelease);
    const t = setInterval(showRunning, 3000);
    // poo doctor checks the policy, files, series, RPC and P2P in one go.
    // Its report is drawn as a checklist; the raw output stays one click away.
    const health = panel(ctx, { title: 'Health check', about: 'What poo doctor finds for this node policy, with the fix it suggests.',
      args: () => ['doctor', '-o', 'json', ...where.flags()],
      onResult: (r, area) => {
        let rep;
        try { rep = JSON.parse(r.out); } catch (_) { return; }
        const raw = area.querySelector('.result');
        if (raw) { raw.replaceWith(doctorReport(rep, ctx)); area.append(rawFold(raw)); }
      } });
    const stop = mountAll(root, 'Node', 'Run the full node and see what it follows.', [health, { el: runSec, stop: () => { clearInterval(t); gone = true; if (unfollowLog) unfollowLog(); } }, followed, status, eachPool, follows, variable, addRelease, follow, unfollow, reload, init]);
    return stop;
  },
};


export function variableTable(vs, cells) {
  return rowsTable(vs, cells, { headers: ['series', 'parts released', 'next part', 'mints', 'units', 'newest release'], cls: 'variable-series', more: 'variable-more', noun: 'variable series' });
}

// followView draws poo follow's JSON: the follow rules as printed, and
// its pools folded under a count, drawn only when opened.
export function followView(o) {
  const rest = { ...o };
  delete rest.pools;
  const pools = el('details', { class: 'follow-pools' }, el('summary', {}, `${o.pools.length} ${o.pools.length === 1 ? 'pool' : 'pools'} followed`));
  pools.addEventListener('toggle', () => { if (pools.open && pools.children.length === 1) pools.append(render(o.pools)); });
  return el('div', { class: 'result follow-view' }, render(rest), pools);
}

// seriesName is what poo's builtin field names a series by: the default
// reserve series (D170) or another series built into poo.
export const seriesName = x => x.builtin === 'default' ? 'the built-in default series' : x.builtin ? `built in: ${x.builtin}` : '';

// seriesTable is the Followed series panel's table, from poo series
// followed's JSON.
export function seriesTable(all) {
  const p = x => x.params || {};
  // poo's warning on a series (a witness ceiling not set, or above 4 MiB:
  // D171) goes in its own column, as printed.
  const cells = x => [seriesName(x), el('code', { class: 'hex', title: String(x.id) }, String(x.id).slice(0, 16) + '…'),
    ...[x.variable ? 'variable' : 'fixed', x.pools, x.variable ? x.variable.mints : p(x).mints, p(x).supply, `${p(x).min_entry_strength ?? 0} / ${p(x).min_layer_strength ?? 0}`].map(v => String(v ?? '')),
    x.warning ? el('span', { class: 'series-warning' }, String(x.warning)) : ''];
  return rowsTable(all, cells, { headers: ['name', 'series', 'supply', 'pools', 'mints', 'units per mint', 'entry / layer floor', 'warning'], cls: 'followed-series', more: 'followed-more', noun: 'series' });
}

// statusNotes draws poo status's entry body cache (D172) and open file
// limit (ORANGE-037) as one line each, from the fields poo gives.
const MIB = 1 << 20;
export function statusNotes(st, guiVersion = api.version) {
  const out = [];
  // The running node's release (W-10, D184) beside this GUI's (cli.Version,
  // GUI White S1). The versions are compared as strings only: a
  // difference is a plain note, never a judgement of what works.
  const rel = st.release;
  if (rel && typeof rel.version === 'string' && rel.version) {
    const same = !guiVersion || rel.version === guiVersion;
    out.push(el('p', same ? { class: 'hint node-release' } : { class: 'next-step node-release differs' },
      `The node runs v${rel.version}${rel.commit ? ` (${String(rel.commit).slice(0, 12)})` : ''}${same ? '.' : `; this GUI is v${guiVersion}. They are different releases, so a view may show or send what the other does not know.`}`));
  } else if (st.release !== undefined || st.series || st.pools) {
    out.push(el('p', { class: 'hint node-release' }, 'The node did not report its release (an older node).'));
  }
  const fl = st.file_limit;
  if (fl && fl.known && fl.low) out.push(el('p', { class: 'next-step file-limit-low', role: 'alert' },
    `The open file limit is ${fl.soft}, below the ${fl.need} the followed pools need. Raise it (ulimit -n, or LimitNOFILE for a service) and restart the node.`));
  // Each figure is read only if poo gave it as a number: a partial reply
  // (an older node, a field left out) shows "?" for it rather than
  // throwing, which would leave the lines after it undrawn (GUI/Red R29B).
  const b = st.bodies;
  const num = x => typeof x === 'number' && Number.isFinite(x);
  const n = x => num(x) ? String(x) : '?';
  const mib = (x, d) => num(x) ? (x / MIB).toFixed(d) : '?';
  if (b && num(b.max_bytes)) out.push(el('p', { class: 'hint entry-cache' },
    `Entry cache: ${mib(b.used_bytes, 1)} of ${mib(b.max_bytes, 0)} MiB (entry_cache_mib), ${n(b.bodies)} bodies; ${n(b.hits)} hits, ${n(b.loads)} read from disk${num(b.errors) && b.errors ? `, ${b.errors} failed reads` : ''}. ${n(b.in_memory)} of ${n(b.held)} entries are in memory${num(b.cold) ? `, and ${b.cold} older ones (outside recent_window) are read from the logs when needed` : ''}.`));
  // Pool snapshots (D178), as poo sums them up.
  const sn = st.snapshots;
  if (sn && sn.off) out.push(el('p', { class: 'hint snapshots' }, 'Pool snapshots are off (no_snapshots): each start replays every pool log whole.'));
  else if (sn && num(sn.pools)) out.push(el('p', { class: 'hint snapshots' },
    `Snapshots: ${n(sn.pools - (num(sn.missing) ? sn.missing : 0))} of ${n(sn.pools)} pools with a log have one${num(sn.age_seconds) && sn.pools > sn.missing ? `, the oldest ${age(sn.age_seconds)} old` : ''}; a start would replay at most ${n(sn.behind)} records. A snapshot is written every ${n(sn.every)} records (snapshot_every)${num(sn.due) && num(sn.every) && sn.due > sn.every ? `, or up to ${sn.due} for the largest log` : ''}, ${mib(sn.bytes, 1)} MiB in all.`));
  // The router port mapping (D182), as poo reports it; absent or off: no
  // line. A state that leaves peers unable to reach the node (failed,
  // not_public, and unconfirmed: mapped but the dial back did not arrive,
  // so poo removed it; GUI/Red R31B) is an alert; poo's own state and
  // error words are kept.
  const pm = st.port_map;
  if (pm && pm.mode && pm.mode !== 'off') {
    const bad = pm.state === 'failed' || pm.state === 'not_public' || pm.state === 'unconfirmed';
    out.push(el('p', bad ? { class: 'next-step port-map', role: 'alert' } : { class: 'hint port-map' },
      `Port mapping (${pm.mode}): ${String(pm.state || '?').replace(/_/g, ' ')}${pm.method ? ` via ${pm.method}` : ''}${pm.external ? `, the router reports ${pm.external}` : ''}${pm.advertised ? `, offered to peers as ${pm.advertised}` : ''}${pm.lease_seconds ? `, lease ${age(pm.lease_seconds)}` : ''}${pm.error ? `. ${pm.error}` : ''}.`));
  }
  // How the running node started: from snapshots, and under -reverify
  // whether the full replay matched them.
  const rp = st.replay;
  if (rp && num(rp.snapshots)) out.push(el('p', { class: 'hint replay' },
    `This start: ${n(rp.snapshots)} ${rp.snapshots === 1 ? 'pool' : 'pools'} from ${rp.snapshots === 1 ? 'its snapshot' : 'their snapshots'} (${n(rp.snapshot_records)} records not read again), ${n(rp.records)} records replayed after them${unused(rp.snapshots_unused, rp.snapshots_unused_count, 'snapshots not used')}.`));
  if (rp && num(rp.reverified) && rp.reverified) out.push(el('p', rp.reverify_differed ? { class: 'next-step reverify-differed', role: 'alert' } : { class: 'hint reverify' },
    rp.reverify_differed ? `Checked every record again: the full replay of ${rp.reverify_differed} of ${rp.reverified} pools built a different pool than its snapshot held. Run poo doctor and keep the logs for a report.`
      : `Checked every record again: the full replay of all ${rp.reverified} pools matched their snapshots.`));
  if (rp && ((rp.reverify_not_compared || []).length || rp.reverify_not_compared_count)) out.push(el('p', { class: 'hint reverify-not-compared' },
    `Not compared${unused(rp.reverify_not_compared, rp.reverify_not_compared_count, 'pools').replace(/^; /, ': ')}.`));
  return out;
}

// unused says how many of a list poo capped at a few reasons there are,
// from its count field when it gives one (D184, GUI White S2), with the
// reasons it printed.
function unused(list, total, noun) {
  const xs = Array.isArray(list) ? list : [];
  const k = typeof total === 'number' ? total : xs.length;
  if (!k) return '';
  return `; ${k} ${noun}${xs.length ? `: ${xs.slice(0, 5).join('; ')}${k > Math.min(xs.length, 5) ? '; …' : ''}` : ''}`;
}

// age says a number of seconds as the largest whole unit.
function age(s) {
  for (const [u, n] of [['day', 86400], ['hour', 3600], ['minute', 60]]) if (s >= n) { const k = Math.floor(s / n); return `${k} ${u}${k === 1 ? '' : 's'}`; }
  return `${s} second${s === 1 ? '' : 's'}`;
}

// totalsTable is the Followed pools panel's table, from poo status
// -counts: one row per series, with its name when it is built into poo.
export function totalsTable(rows) {
  const cells = x => [seriesName(x), el('code', { class: 'hex', title: String(x.series) }, String(x.series).slice(0, 16) + '…'),
    ...['pools', 'entries', 'branches', 'pending', 'waiting', 'stored_records'].map(k => String(x[k] ?? ''))];
  return rowsTable(rows, cells, { headers: ['name', 'series', 'pools', 'entries', 'branches', 'pending', 'waiting', 'stored records'], cls: 'series-totals', more: 'totals-more', noun: 'series' });
}

