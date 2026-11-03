// The Node policies view. Each view is one file here, listed in index.js (GUI-GREEN-044).
import * as api from '../api.js';
import * as where from '../where.js';
import { el, flag, form, mountAll, panel, runJob } from '../views.js';

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
      importForm(ctx, rootFlag),
    ]);
  },
};

// saveAs offers printed output as a file to download, as it was printed.
function saveAs(text, name) {
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

// importForm previews poo policy import (it changes nothing without
// -apply), draws the plan poo printed, and offers Apply with the same
// flags. poo makes the plan; the GUI only lays it out.
function importForm(ctx, rootFlag) {
  const importArgs = v => ['policy', 'import', ...(v.every ? rootFlag() : where.flags()), ...flag('prefer-file', v.prefer), ...flag('replace', v.replace), ...flag('settings', v.settings), ...flag('full', v.full), ...flag('no-reload', v.noReload), '-json', ...(v.file ? [v.file] : [])];
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
  const f = form(ctx, { title: 'Import node policies', about: 'Shows what a node policy file would change here: what is new, changed or in conflict. Nothing changes until you press Apply.', fields: [
    { name: 'file', label: 'File', placeholder: 'a path on this machine, or pick one below', required: true },
    { name: 'every', label: 'The file holds every saved policy (exported with "every saved node policy"): each goes under its own name', type: 'check' },
    { name: 'prefer', label: 'Where both set a value differently, take the file\'s (default: keep this node\'s)', type: 'check' },
    { name: 'replace', label: 'Replace this node policy with the file\'s instead of merging (machine-local settings stay)', type: 'check' },
    { name: 'settings', label: 'Take the file\'s node settings too (peers, registries, grind jobs, tracking, p2p limits); by default only what to follow is taken', type: 'check' },
    { name: 'full', label: 'Take every setting, machine-local ones too (rpc, p2p addresses, grind device, registry files): restoring a backup on this machine', type: 'check' },
    { name: 'noReload', label: 'Only write the policy (no reload)', type: 'check' }],
  args: v => { last = importArgs(v); return last; }, submit: 'Preview',
  onStart: j => { dropApply(); previewing = { args: last, file: picked }; feed(last, j, picked); },
  onResult: (r, area) => {
    const { args, file } = previewing || { args: last, file: picked };
    showPlan(r, area);
    if (r.code !== 0) return;
    let digest = '';
    try { digest = (JSON.parse(r.out)[0] || {}).digest || ''; } catch (_) { /* no plan */ }
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
      runJob(a, out, { onStart: j => feed(a, j, file) }).then(r2 => showPlan(r2, out, true));
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
    const rows = [];
    for (const [kind, xs] of [['added', p.added], ['removed', p.removed], ['changed', p.changed], ['conflict', p.conflicts]]) {
      for (const c of xs || []) rows.push(el('tr', { class: `plan-${kind}` }, el('td', {}, kind), el('td', {}, c.field), el('td', {}, c.local || ''), el('td', {}, c.file || ''), el('td', {}, c.took ? `took ${c.took}'s` : '')));
    }
    const notes = [
      p.new ? 'A new node policy.' : null,
      (p.skipped_settings || []).length ? `Node settings left alone (Take the file's node settings takes them): ${p.skipped_settings.join(', ')}.` : null,
      `${p.same || 0} already the same.`,
      (p.skipped || []).length ? `Machine-local settings left alone: ${p.skipped.join(', ')}.` : null,
      (p.refused || []).length ? `Not accepted here: ${p.refused.join('; ')}.` : null,
      t.saved ? (t.reloading ? 'Saved; the running node is reloading it.' : 'Saved.') : applied ? 'Nothing needed changing.' : 'Nothing changed yet: press Apply to make this change.',
    ].filter(Boolean).join(' ');
    // Read first: lines that change a defence of this node or where it
    // reaches out (D150), as poo prints them first.
    const defences = (p.defences || []).length ? el('p', { class: 'next-step plan-defences', role: 'alert' },
      `Read first: these change a defence of this node or where it reaches out: ${p.defences.join(', ')}`) : null;
    return el('div', { class: 'import-plan' },
      el('h3', {}, `${t.name || t.dir} (${t.mode}${t.running ? ', running' : ''})`), defences,
      rows.length ? el('div', { class: 'table-wrap' }, el('table', { class: 'data' },
        el('thead', {}, el('tr', {}, ...['', 'setting', 'this node', 'the file', ''].map(h => el('th', {}, h)))), el('tbody', {}, ...rows)))
        : el('p', {}, 'No settings change.'),
      el('p', { class: 'hint' }, notes));
  });
  const det = el('details', { class: 'raw' }, el('summary', {}, 'Output as printed'));
  raw.replaceWith(el('div', {}, ...blocks));
  det.append(raw);
  area.append(det);
}
