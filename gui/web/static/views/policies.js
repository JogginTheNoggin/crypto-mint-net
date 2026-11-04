// The Node policies view. Each view is one file here, listed in index.js (GUI-GREEN-044).
import * as api from '../api.js';
import * as where from '../where.js';
import { el, flag, flags, form, mountAll, panel, runJob } from '../views.js';

export const policiesView = {
  id: 'policies',
  title: 'Node policies',
  mount(root, ctx) {
    const rootFlag = () => where.get().root ? ['-root', where.get().root] : [];
    const name = { name: 'name', label: 'Policy name' };
    return mountAll(root, 'Node policies', 'Saved and archived node policies; * marks the current one.', [
      panel(ctx, { title: 'Policies', args: () => ['policy', 'list', ...rootFlag()], empty: 'No saved node policies yet. Write one on the Node view.' }),
      form(ctx, { title: 'Use, archive, unarchive or delete', fields: [
        { name: 'sub', label: 'Action', type: 'select', options: ['use', 'archive', 'unarchive', 'delete'] }, name,
        { name: 'fresh', label: 'Fresh (drop stored data)', type: 'check', only: { field: 'sub', values: ['unarchive'] } }],
      args: v => ['policy', v.sub, ...rootFlag(), ...flag('fresh', v.fresh), ...(v.name ? [v.name] : [])], submit: 'Apply', danger: true }),
      // Export and import share node policies as one file (D149). A file
      // of every saved policy names each policy itself, so poo takes only
      // -root with it; the other scopes use Settings' -root and -policy.
      form(ctx, { title: 'Export node policies', about: 'Writes this node policy, one series, or every saved policy to one file others can import. Without a file, it is printed here, with Save as file.', fields: [
        { name: 'scope', label: 'What', type: 'select', options: [{ value: 'policy', label: 'this node policy' }, { value: 'all', label: 'every saved node policy' }, { value: 'series', label: 'one series' }] },
        { name: 'series', label: 'Series', placeholder: 'hex id', only: { field: 'scope', values: ['series'] } },
        { name: 'full', label: 'Keep machine-local settings (rpc, p2p addresses, grind device): for a backup or a clone of this machine', type: 'check' },
        { name: 'out', label: 'To a file', placeholder: 'a path on this machine (default: print it)' }],
      args: v => ['policy', 'export', ...(v.scope === 'all' ? ['-all', ...rootFlag()] : where.flags()), ...flag('series', v.series), ...flag('full', v.full), ...flag('out', v.out)], submit: 'Export',
      onResult: (r, area) => { if (r.code === 0 && r.out.trim().startsWith('{')) area.append(saveAs(r.out, 'node-policies.json')); } }),
      importForm(ctx, POLICY_IMPORT(rootFlag)),
      // Settings travel the same way, without what the node follows
      // (D155): a backup, or a second machine run like this one.
      form(ctx, { title: 'Export node settings', about: 'Writes this node policy\'s settings (peers, registries, grind jobs, tracking, p2p switches and limits), without what it follows, to one file. Without a file, it is printed here, with Save as file.', fields: [
        { name: 'full', label: 'Keep machine-local settings (rpc, p2p addresses, grind device and threads): for a backup, or a new machine that should run the same way', type: 'check' },
        { name: 'out', label: 'To a file', placeholder: 'a path on this machine (default: print it)' }],
      args: v => ['settings', 'export', ...where.flags(), ...flag('full', v.full), ...flag('out', v.out)], submit: 'Export',
      onResult: (r, area) => { if (r.code === 0 && r.out.trim().startsWith('{')) area.append(saveAs(r.out, 'node-settings.json')); } }),
      importForm(ctx, SETTINGS_IMPORT),
      ...limitsParts(ctx),
    ]);
  },
};

// limitsParts is the settable limits (D159, D160, D161): poo policy limits
// lists each one's range, default, the value node.json sets and the one in
// effect, and -set changes them, checked by poo against the range before
// anything is saved. The list comes from poo, so a limit Source adds shows
// here without a change to the GUI.
//
// Set limits draws a field for each limit poo listed, named for it and
// showing its value in effect, range and default, so no name has to be
// remembered or typed (GUI-GREEN-049). A field left empty sends nothing.
// poo still checks every value; the GUI only shows what poo printed.
function limitsParts(ctx) {
  let typed = [];
  // saved is set by a successful save, so the redraw after it starts empty;
  // any other redraw (Refresh) keeps what was typed and not yet saved.
  let saved = false;
  const set = form(ctx, { title: 'Set limits', about: 'Saves the limits to node.json; a running node takes them at once. Fill only the ones to change. Each value must lie in its range, or nothing is saved; 0 puts a limit back to its default.', fields: [
    { name: 'set', label: 'Other limits (NAME=VALUE)', type: 'list', placeholder: 'any poo policy limits names that are not listed above' }],
  args: v => ['policy', 'limits', ...where.flags(), ...flags('set', [...typed.filter(i => i.value.trim()).map(i => `${i.name}=${i.value.trim()}`), ...v.set])], submit: 'Save', danger: true,
  onResult: r => { if (r.code === 0) { saved = true; limits.reload(); } } });
  const grid = el('div', { class: 'limit-fields' });
  set.form.prepend(grid);
  const limits = panel(ctx, { title: 'Settable limits', about: 'Each limit this node policy can set: its range, its default, what node.json sets (0: the default) and the value in effect.', args: () => ['policy', 'limits', ...where.flags()],
    onResult: r => {
      let list;
      try { list = r.code === 0 ? JSON.parse(r.out) : null; } catch (_) { list = null; }
      if (!Array.isArray(list)) return;
      const named = list.filter(l => l && typeof l.name === 'string');
      const before = saved ? {} : Object.fromEntries(typed.map(i => [i.name, i.value]));
      saved = false;
      typed = named.map(l => el('input', { name: l.name, type: 'text', inputmode: 'numeric', placeholder: String(l.used ?? ''), spellcheck: 'false', 'data-limit': l.name, value: before[l.name] || '' }));
      grid.replaceChildren(...named.map((l, n) => {
        return el('label', {}, el('span', {}, l.name), typed[n], el('small', {}, `${l.min} to ${l.max}, default ${l.default}${l.set ? `; node.json sets ${l.set}` : ''}`));
      }));
      set.form.dispatchEvent(new Event('input', { bubbles: true }));
    } });
  return [limits, set];
}

// saveAs offers printed output as a file to download, as it was printed.
export function saveAs(text, name) {
  const b = el('button', { type: 'button', class: 'small save-as' }, 'Save as file');
  b.onclick = () => {
    const a = el('a', { download: name, hidden: true });
    a.href = URL.createObjectURL(new Blob([text], { type: 'application/json' }));
    document.body.append(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(a.href), 1000);
  };
  return el('div', { class: 'actions' }, b);
}

// mapField is -map-path OLD=NEW, for registry file paths in a file made
// on another machine (D155); poo reads the paths, the GUI only sends them.
const mapField = { name: 'map', label: 'Registry paths from the other machine (OLD=NEW)', type: 'list', placeholder: '/home/a/.poo=/home/b/.poo' };

// The two imports: one form, different commands and options. Each
// spec gives the form's title and fields and builds the preview line.
const POLICY_IMPORT = rootFlag => ({
  title: 'Import node policies', about: 'Shows what a node policy file would change here: what is new, changed or in conflict. Nothing changes until you press Apply.',
  fields: [
    { name: 'every', label: 'The file holds every saved policy (exported with "every saved node policy"): each goes under its own name', type: 'check' },
    { name: 'prefer', label: 'Where both set a value differently, take the file\'s (default: keep this node\'s)', type: 'check' },
    { name: 'replace', label: 'Replace this node policy with the file\'s instead of merging (machine-local settings stay)', type: 'check' },
    { name: 'settings', label: 'Take the file\'s node settings too (peers, registries, grind jobs, tracking, p2p limits); by default only what to follow is taken', type: 'check' },
    { name: 'full', label: 'Take every setting, machine-local ones too (rpc, p2p addresses, grind device, registry files): restoring a backup on this machine', type: 'check' },
    mapField,
    { name: 'noReload', label: 'Only write the policy (no reload)', type: 'check' }],
  args: v => ['policy', 'import', ...(v.every ? rootFlag() : where.flags()), ...flag('prefer-file', v.prefer), ...flag('replace', v.replace), ...flag('settings', v.settings), ...flag('full', v.full), ...flags('map-path', v.map), ...flag('no-reload', v.noReload), '-json', ...(v.file ? [v.file] : [])],
});
const SETTINGS_IMPORT = {
  title: 'Import node settings', about: 'Shows what a settings file would change in this node policy (what it follows stays as it is), with what may not fit this machine. Nothing changes until you press Apply.',
  fields: [
    { name: 'prefer', label: 'Where both set a value differently, take the file\'s (default: keep this node\'s)', type: 'check' },
    { name: 'replace', label: 'Replace this node\'s settings with the file\'s instead of merging (what it follows and machine-local settings stay)', type: 'check' },
    { name: 'full', label: 'Take the machine-local settings too (rpc, p2p addresses, grind device and threads, registry files): moving to a new machine or restoring a backup', type: 'check' },
    mapField,
    { name: 'noReload', label: 'Only write the policy (no reload)', type: 'check' }],
  args: v => ['settings', 'import', ...where.flags(), ...flag('prefer-file', v.prefer), ...flag('replace', v.replace), ...flag('full', v.full), ...flags('map-path', v.map), ...flag('no-reload', v.noReload), '-json', ...(v.file ? [v.file] : [])],
};

// importForm previews poo policy import or poo settings import (neither
// changes anything without -apply), draws the plan poo printed, and
// offers Apply with the same flags. poo makes the plan; the GUI only
// lays it out.
export function importForm(ctx, spec) {
  const importArgs = spec.args;
  // A spec may draw its own plan and find its digest (poo peers import
  // prints one object, not a list of node policies).
  const show = spec.show || showPlan;
  const digestOf = spec.digest || (o => (o[0] || {}).digest);
  // A file picked in the browser goes to poo on its standard input:
  // FILE is -, and the job is sent the file's text, then end of input,
  // as typing it and Ctrl-D would. poo reads and checks it (D149).
  // The file is read as poo reads it, not whole first (ORANGE-014), and
  // Apply sends the digest of the file previewed (-if-digest, D151), so
  // poo refuses anything else, the file changed on disk included (GUI-023).
  let picked = null; // {name, file} of the file picked now
  const feed = (args, j, p) => {
    if (args[args.length - 1] !== '-' || p === null) return;
    api.inputFile(j.id, p.file).catch(() => {});
  };
  // Apply runs what was previewed, and only that (GUI-023): it keeps the
  // preview's args and the very text sent to it, never what is picked
  // now. Any change to the form after a preview takes its Apply away, so
  // the plan on show is always the plan Apply carries out.
  let last = null, previewing = null, dropApply = () => {};
  const f = form(ctx, { title: spec.title, about: spec.about, fields: [
    { name: 'file', label: 'File', placeholder: 'a path on this machine, or pick one below', required: true },
    ...spec.fields],
  args: v => { last = importArgs(v); return last; }, submit: 'Preview',
  onStart: j => { dropApply(); previewing = { args: last, file: picked }; feed(last, j, picked); },
  onResult: (r, area) => {
    const { args, file } = previewing || { args: last, file: picked };
    show(r, area);
    if (r.code !== 0) return;
    let digest = '';
    try { digest = digestOf(JSON.parse(r.out)) || ''; } catch (_) { /* no plan */ }
    // Apply is pinned to the previewed file, or not offered: without a
    // digest from the preview it would run unpinned (GUI/Red R20B).
    if (!/^[0-9a-f]{64}$/.test(digest)) {
      area.append(el('p', { class: 'run-status bad no-digest', role: 'alert' }, 'Apply is not offered: the preview printed no digest to pin it to the file previewed.'));
      return;
    }
    const apply = el('button', { type: 'button', class: 'danger' }, 'Apply');
    const out = el('div', { class: 'area' });
    const actions = el('div', { class: 'actions' }, apply);
    apply.onclick = () => {
      const a = [...args.slice(0, -1), '-apply', ...flag('if-digest', digest), args[args.length - 1]];
      const what = file && args[args.length - 1] === '-' ? ` with the previewed file ${file.name}` : '';
      if (!confirm(`Run ${api.quote(a)}${what}?`)) return;
      apply.disabled = true;
      dropApply = () => {};
      runJob(a, out, { onStart: j => feed(a, j, file) }).then(r2 => show(r2, out, true));
    };
    dropApply = () => { actions.remove(); dropApply = () => {}; };
    area.append(actions, out);
  } });
  f.form.addEventListener('input', () => dropApply());
  f.form.addEventListener('change', () => dropApply());
  const fileIn = f.inputs.file;
  const pick = el('input', { type: 'file', accept: '.json,application/json', name: 'pick' });
  pick.onchange = () => {
    const file = pick.files && pick.files[0];
    if (!file) return;
    picked = { name: file.name, file };
    dropApply();
    fileIn.value = '-';
    fileIn.dispatchEvent(new Event('input', { bubbles: true }));
  };
  // Typing a path again drops the picked file.
  fileIn.addEventListener('input', () => { if (fileIn.value.trim() !== '-') { picked = null; pick.value = ''; } });
  fileIn.closest('label').after(el('label', {}, el('span', {}, 'Or pick a file'), pick, el('small', {}, 'Sent to poo as its input (FILE is -); nothing is stored by the GUI')));
  return f;
}

// PLAN_ROWS is how many lines of a plan are drawn at first, and then on
// each press of "Show more". A file following tens of thousands of
// issuers makes a plan as long: drawn whole, 40,000 lines took 4.6 s and
// held the page for up to 0.9 s at a time, and 100,000 never showed, as
// one call with that many rows overflowed the stack (ORANGE-015). Like
// the other tables (render.js), 500 show at first; the person can draw
// more, and Apply always carries out the whole plan.
const PLAN_ROWS = 500;

// planTable draws [kind, line] pairs; cols names the table's columns and
// cells gives a line's cells after its kind (a node policy's by default).
const settingCells = c => [c.field, c.local || '', c.file || '', c.took ? `took ${c.took}'s` : ''];
export function planTable(lines, cols = ['setting', 'this node', 'the file', ''], cells = settingCells) {
  const body = el('tbody');
  const row = ([kind, c]) => el('tr', { class: `plan-${kind}` }, el('td', {}, kind), ...cells(c).map(x => el('td', {}, x)));
  let shown = 0;
  const more = () => {
    const part = document.createDocumentFragment();
    for (const end = Math.min(lines.length, shown + PLAN_ROWS); shown < end; shown++) part.append(row(lines[shown]));
    body.append(part);
  };
  more();
  const wrap = el('div', { class: 'table-wrap' }, el('table', { class: 'data' },
    el('thead', {}, el('tr', {}, ...['', ...cols].map(h => el('th', {}, h)))), body));
  if (shown < lines.length) {
    const text = document.createTextNode('');
    const btn = el('button', { type: 'button' }, 'Show more');
    const note = el('p', { class: 'hint plan-more' }, text, btn);
    const label = () => { text.textContent = `Showing ${shown} of ${lines.length} lines; Apply makes every one, and "Output as printed" below has them all. `; };
    label();
    btn.onclick = () => { more(); if (shown < lines.length) label(); else note.remove(); };
    wrap.append(note);
  }
  return wrap;
}

// SOME is how many items of a plan's list are named in its line: the
// defences, the series now used, the follows. A file can make any of
// them as long as the plan: 50,000 peers taken with their settings made
// a "Read first" line of 1.5 million characters, 183,000 pixels tall,
// read out whole as an alert, with the table and Apply below it
// (ORANGE-016). The line names the first ones and how many there are;
// "Show all" lists every one.
const SOME = 20;
export const count = (xs, one, many = one + 's') => `${xs.length} ${xs.length === 1 ? one : many}`;
export const someOf = xs => xs.length > SOME ? `${xs.slice(0, SOME).join(', ')}, and ${xs.length - SOME} more.` : `${xs.join(', ')}.`;
export function allOf(xs) {
  if (xs.length <= SOME) return null;
  const d = el('details', { class: 'plan-all' }, el('summary', {}, `Show all ${xs.length}`));
  // Drawn when opened, so a closed list costs nothing.
  d.addEventListener('toggle', () => { if (d.open && d.childNodes.length === 1) d.append(el('p', {}, xs.join(', '))); });
  return d;
}

// showPlan draws poo policy import -json: one table per node policy, a row
// for each setting added, removed, changed or in conflict.
function showPlan(r, area, applied) {
  if (r.code !== 0) return;
  let list;
  try { list = JSON.parse(r.out); } catch (_) { return; }
  const raw = area.querySelector('.result');
  if (!raw || !Array.isArray(list)) return;
  const blocks = list.map(t => {
    const p = t.plan || {};
    const lines = [];
    for (const [kind, xs] of [['added', p.added], ['removed', p.removed], ['changed', p.changed], ['conflict', p.conflicts]]) {
      for (const c of xs || []) lines.push([kind, c]);
    }
    const notes = [
      p.new ? 'A new node policy.' : null,
      (p.skipped_settings || []).length ? `Node settings left alone (Take the file's node settings takes them): ${p.skipped_settings.join(', ')}.` : null,
      `${p.same || 0} already the same.`,
      (p.skipped || []).length ? `Machine-local settings left alone: ${p.skipped.join(', ')}.` : null,
      (p.refused || []).length ? `Not accepted here: ${p.refused.join('; ')}.` : null,
      (p.follows || []).length ? `Follows: ${p.follows.slice(0, SOME).map(c => `series ${c.series.slice(0, 12)}: ${c.followed} of ${c.pools} pools${c.whole ? ' (the whole series)' : ''}`).join('; ')}${p.follows.length > SOME ? `; and ${p.follows.length - SOME} more series ("Output as printed" has them all)` : ''}.` : null,
      (p.fit || []).length ? `May not fit this machine: ${p.fit.join('; ')}.` : null,
      t.saved ? (t.reloading ? 'Saved; the running node is reloading it.' : 'Saved.') : applied ? 'Nothing needed changing.' : 'Nothing changed yet: press Apply to make this change.',
    ].filter(Boolean).join(' ');
    // Read first: lines that change a defence of this node or where it
    // reaches out (D150), as poo prints them first.
    const defences = (p.defences || []).length ? el('div', { class: 'next-step plan-defences' },
      el('p', { role: 'alert' }, `Read first: ${count(p.defences, 'line')} ${p.defences.length === 1 ? 'changes' : 'change'} a defence of this node or where it reaches out: `, someOf(p.defences)), allOf(p.defences)) : null;
    // Series listed but not accepted before that the result uses: the
    // node will follow, mine into and relay them (RED-056, D156).
    const nowUsed = (p.now_used || []).length ? el('div', { class: 'next-step plan-now-used' },
      el('p', { role: 'alert' }, `Now used: ${count(p.now_used, 'series', 'series')} listed but not accepted before. The node will follow, mine into and relay ${p.now_used.length === 1 ? 'it' : 'them'}: `, someOf(p.now_used)), allOf(p.now_used)) : null;
    return el('div', { class: 'import-plan' },
      el('h3', {}, `${t.name || t.dir} (${t.mode}${t.running ? ', running' : ''})`), defences, nowUsed,
      lines.length ? planTable(lines) : el('p', {}, 'No settings change.'),
      el('p', { class: 'hint' }, notes));
  });
  const det = el('details', { class: 'raw' }, el('summary', {}, 'Output as printed'));
  raw.replaceWith(el('div', {}, ...blocks));
  det.append(raw);
  area.append(det);
}
