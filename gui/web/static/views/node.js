// The Node view. Each view is one file here, listed in index.js (GUI-GREEN-044).
import * as api from '../api.js';
import * as where from '../where.js';
import { cmdLine, el, flag, flags, form, mountAll, panel } from '../views.js';
import { scrollback } from '../scroll.js';

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
      el('strong', {}, c.name), ' ', el('span', {}, c.detail || ''), fix));
  }
  return el('div', { class: 'result' }, rep.dir ? el('p', { class: 'hint' }, `Data directory: ${rep.dir}`) : null, list);
}

export const nodeView = {
  id: 'node',
  title: 'Node',
  mount(root, ctx) {
    const runArea = el('div', { class: 'area' });
    const startBtn = el('button', { type: 'button', class: 'primary' }, 'Start node');
    const runLine = cmdLine(['node', ...where.flags()], ctx);
    const runSec = el('section', { class: 'panel' },
      el('div', { class: 'panel-head' }, el('h2', {}, 'Run the node'), startBtn),
      el('p', { class: 'hint' }, 'Runs poo node inside the GUI until you stop it or close the GUI. A node already running elsewhere (for example as a service) is used as it is: the views below talk to whichever node serves this node policy.'),
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
      const a = ['node', ...where.flags()];
      runLine.set(a);
      try { await api.start(a); } catch (e) { runArea.textContent = e.message; }
      setTimeout(showRunning, 200);
    };
    showRunning();
    // poo status says when node.json changed since the node loaded it and
    // the change waits for a reload (Source GREEN-037). The panel says so
    // above what it shows, with the Reload form's own button (GREEN-038).
    const status = panel(ctx, { title: 'Followed pools', about: 'What the running node follows, and where each pool stands.', args: () => ['status', ...where.flags()], refresh: 5000,
      onResult: (r, area) => {
        let st;
        try { st = r.code === 0 && JSON.parse(r.out); } catch (_) { return; }
        if (!st || !st.policy_changed) return;
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
        { name: 'force', label: 'Replace the settings of an existing policy (its data is kept)', type: 'check' },
      ],
      args: v => ['init', ...where.flags(), ...flag('example', v.example), ...flags('series', v.series), ...flag('listen', v.listen),
        ...flag('p2p', v.p2p), ...flags('peer', v.peer), ...flags('registry', v.registry), ...flag('advertise', v.advertise), ...flag('force', v.force)],
      submit: 'Write policy',
    });
    const followFields = [
      { name: 'issuer', label: 'Issuer id', placeholder: 'every series of an issuer' },
      { name: 'series', label: 'Series id' },
      { name: 'pool', label: 'Pools', type: 'list', placeholder: 'K …' },
      { name: 'mint', label: 'Mints', type: 'list', placeholder: 'serial …' },
    ];
    const followArgs = (cmd, v) => [cmd, ...where.flags(), ...flag('issuer', v.issuer), ...flag('series', v.series), ...flags('pool', v.pool), ...flags('mint', v.mint), ...flag('replace', v.replace), ...flag('no-reload', v.noReload)];
    const follows = panel(ctx, { title: 'What the node follows', about: 'The follow list of the node policy.', args: () => ['follow', ...where.flags()] });
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
        try { o = r.code === 0 && JSON.parse(r.out); } catch (_) { return; }
        const all = o && Array.isArray(o.series) ? o.series : null;
        if (!all) return;
        const raw = area.querySelector('.result');
        const vs = all.filter(x => x && x.variable);
        const body = vs.length ? el('div', { class: 'table-wrap' }, el('table', { class: 'data variable-series' },
          el('thead', {}, el('tr', {}, ...['series', 'parts released', 'next part', 'mints', 'units', 'newest release'].map(h => el('th', {}, h)))),
          el('tbody', {}, ...vs.map(x => el('tr', {},
            el('td', {}, el('code', { class: 'hex', title: x.id }, String(x.id).slice(0, 16) + '…')),
            ...[x.variable.released, x.variable.next_part ?? 'none (the cap is reached)', x.variable.mints, x.variable.units, x.variable.release ? `part ${x.variable.release.part}` : 'none: part 1 only'].map(c => el('td', {}, String(c)))))))) :
          el('p', { class: 'hint empty' }, `None of the ${all.length} followed series is variable.`);
        const det = el('details', { class: 'raw' }, el('summary', {}, 'Output as printed'));
        if (raw) { raw.replaceWith(body); det.append(raw); area.append(det); }
      } });
    // A release made by the issuer reaches a node as a file (D166): poo
    // checks it, keeps it in node.json and the node reloads to follow the
    // new parts' mints. A file picked here goes to poo as its input (-).
    let picked = null;
    const addRelease = form(ctx, { title: 'Add a release', about: 'Gives the node a release of a variable series it follows. poo checks it against the series, saves it in node.json, and the node then follows the new parts\' mints. A release can never be taken back.',
      fields: [{ name: 'file', label: 'Release file', placeholder: 'a path on this machine, or pick one below', required: true }],
      args: v => ['series', 'add-release', ...where.flags(), ...(v.file ? [v.file] : [])], submit: 'Add release', danger: true,
      onStart: j => { if (picked && addRelease.inputs.file.value.trim() === '-') api.inputFile(j.id, picked).catch(() => {}); },
      onResult: r => { if (r.code === 0) { variable.reload(); status.reload(); } } });
    {
      const fileIn = addRelease.inputs.file;
      const pick = el('input', { type: 'file', accept: '.json,application/json', name: 'pick' });
      pick.onchange = () => {
        const file = pick.files && pick.files[0];
        if (!file) return;
        picked = file;
        fileIn.value = '-';
        fileIn.dispatchEvent(new Event('input', { bubbles: true }));
      };
      fileIn.addEventListener('input', () => { if (fileIn.value.trim() !== '-') { picked = null; pick.value = ''; } });
      fileIn.closest('label').after(el('label', {}, el('span', {}, 'Or pick a file'), pick, el('small', {}, 'Sent to poo as its input (FILE is -); nothing is stored by the GUI')));
    }
    const t = setInterval(showRunning, 3000);
    // poo doctor checks the policy, files, series, RPC and P2P in one go.
    // Its report is drawn as a checklist; the raw output stays one click away.
    const health = panel(ctx, { title: 'Health check', about: 'What poo doctor finds for this node policy, with the fix it suggests.',
      args: () => ['doctor', '-o', 'json', ...where.flags()],
      onResult: (r, area) => {
        let rep;
        try { rep = JSON.parse(r.out); } catch (_) { return; }
        const raw = area.querySelector('.result');
        const det = el('details', { class: 'raw' }, el('summary', {}, 'Output as printed'));
        if (raw) { raw.replaceWith(doctorReport(rep, ctx)); det.append(raw); area.append(det); }
      } });
    const stop = mountAll(root, 'Node', 'Run the full node and see what it follows.', [health, { el: runSec, stop: () => { clearInterval(t); gone = true; if (unfollowLog) unfollowLog(); } }, status, follows, variable, addRelease, follow, unfollow, reload, init]);
    return stop;
  },
};
