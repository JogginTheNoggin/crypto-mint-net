// The Series and keys view. Each view is one file here, listed in index.js (GUI-GREEN-044).
import * as api from '../api.js';
import { el, flag, form, mountAll, panel, pickFile, runJob } from '../views.js';
import { saveAs } from './policies.js';

// cryptoChoices offers the hashes and key algorithms poo crypto printed as
// suggestions in the fields that name one (a datalist suggests, poo still
// decides). The lists stay in the page for the Wallet view's key field too.
function cryptoChoices(out) {
  let c;
  try { c = JSON.parse(out); } catch (_) { return; }
  for (const [id, list] of [['crypto-hashes', c.hashes], ['crypto-algs', c.key_algs]]) {
    let dl = document.getElementById(id);
    if (!dl) { dl = el('datalist', { id }); document.body.append(dl); }
    dl.textContent = '';
    for (const x of list || []) dl.append(el('option', { value: x.name }));
  }
}

// A key field opened before Series and keys asks poo crypto once, the
// first time it is focused, so its suggestions are poo's own. One ask is
// in flight at a time (ORANGE-011): focus moving in and out of key fields
// before poo answers would otherwise run poo crypto for each focus, each
// a job in the Jobs list.
let askingCrypto = null;
document.addEventListener('focusin', e => {
  const l = e.target.getAttribute && e.target.getAttribute('list');
  if ((l === 'crypto-algs' || l === 'crypto-hashes') && !document.getElementById(l) && !askingCrypto) {
    askingCrypto = api.run(['crypto']).then(r => { if (r.code === 0) cryptoChoices(r.out); }).catch(() => {})
      .finally(() => { askingCrypto = null; });
  }
});

export const cryptoView = {
  id: 'crypto',
  title: 'Series and keys',
  mount(root, ctx) {
    return mountAll(root, 'Series and keys', 'Offline tools: the choices a series can make, keys, and series parameters.', [
      panel(ctx, { title: 'Hashes, key algorithms and policy types', args: () => ['crypto'], onResult: r => { if (r.code === 0) cryptoChoices(r.out); } }),
      form(ctx, { title: 'Make a key', about: 'A key and its 1-of-1 address, from a random seed unless you give your own; the output includes the seed.',
        fields: [{ name: 'alg', label: 'Key algorithm', placeholder: 'ed25519', list: 'crypto-algs' }, { name: 'hash', label: 'Hash', placeholder: 'sha256', list: 'crypto-hashes' }, { name: 'own', label: 'Use my own seed', type: 'check', help: 'poo asks for it in a masked field after you press Make key (-seed-file -), so it is never on the command line (D98).' }],
        args: v => ['keygen', ...flag('alg', v.alg), ...flag('hash', v.hash), ...(v.own ? ['-seed-file', '-'] : [])], submit: 'Make key', secretOutput: true }),
      // A series is fixed unless Variable is ticked (D163): then the issuer
      // releases more parts later. poo checks the definition; the GUI only
      // passes the fields on and offers what poo printed as a file.
      form(ctx, { title: 'New series parameters', about: 'Prints new series parameters, checked by poo, with Save as file. More options are in the terminal (poo series new -h).',
        fields: [{ name: 'hash', label: 'Hash', placeholder: 'sha256', list: 'crypto-hashes' }, { name: 'algs', label: 'Key algorithms', placeholder: 'comma list: ' + 'see the list above' }, { name: 'types', label: 'Policy types', placeholder: 'comma list: see the list above' }, { name: 'mints', label: 'Mints', type: 'number', placeholder: 'default 8' },
          // N, S and P together: a variable series needs N and M to be
          // multiples of P, so P is here beside them (GUI-GREEN-051).
          { name: 'supply', label: 'Units in every mint', type: 'number', placeholder: 'default 100' },
          { name: 'poolSize', label: 'Pool size', type: 'number', placeholder: 'mints per pool, default 4' },
          { name: 'maxWitness', label: 'Witness ceiling', type: 'number', placeholder: 'largest witness in bytes, default 131072' },
          { name: 'anyCeiling', label: 'Allow a witness ceiling of 0 (not set) or above 4 MiB (-any-ceiling)', type: 'check' },
          { name: 'issuer', label: 'Issuer address', placeholder: 'hex: who signs the series (needed for a variable series)' },
          { name: 'variable', label: 'Variable supply: the issuer can release more parts later, one at a time and in order; a release can never be taken back', type: 'check' },
          { name: 'partSize', label: 'Part size', type: 'number', placeholder: 'mints in every part after the first', only: { field: 'variable', values: [true] } },
          { name: 'maxParts', label: 'Most parts ever', type: 'number', placeholder: 'the first part included (empty: no cap)', only: { field: 'variable', values: [true] } }],
        args: v => ['series', 'new', ...flag('hash', v.hash), ...flag('algs', v.algs), ...flag('types', v.types), ...flag('mints', v.mints), ...flag('supply', v.supply), ...flag('pool-size', v.poolSize), ...flag('max-witness', v.maxWitness), ...flag('any-ceiling', v.anyCeiling), ...flag('issuer', v.issuer), ...flag('variable', v.variable), ...flag('part-size', v.partSize), ...flag('max-parts', v.maxParts)], submit: 'Make series',
        onResult: (r, area) => { if (r.code === 0 && r.out.trim().startsWith('{')) area.append(saveAs(r.out, 'series.json')); } }),
      form(ctx, { title: 'Series file info', fields: [{ name: 'file', label: 'Series file', placeholder: 'series.json' }],
        args: v => ['series', 'info', ...(v.file ? [v.file] : [])], submit: 'Show' }),
      form(ctx, { title: 'Sign a series', about: 'Signs a series file as its issuer and prints the issuer witness for node.json. poo asks for the issuer key\'s seed in a masked field.',
        fields: [{ name: 'file', label: 'Series file', placeholder: 'series.json', required: true }, { name: 'alg', label: 'Key algorithm', placeholder: 'ed25519', list: 'crypto-algs', help: 'The issuer key\'s algorithm, from the list above (-alg).' }],
        args: v => ['series', 'sign', ...(v.file ? [v.file] : []), ...flag('alg', v.alg), '-seed-file', '-'], submit: 'Sign',
        onResult: (r, area) => { if (r.code === 0 && r.out.trim().startsWith('{')) area.append(saveAs(r.out, 'series-witness.json')); } }),
      // Parts of a variable series, as poo prints them: ranges, never the
      // serials one by one (ORANGE-018); run when asked, not on a timer.
      form(ctx, { title: 'Parts of a variable series', about: 'Each part\'s serials, pools and part id, worked out by poo from the series file. Up to 1,000 parts at a time.',
        fields: [{ name: 'file', label: 'Series file', placeholder: 'series.json', required: true }, { name: 'from', label: 'From part', type: 'number', placeholder: 'default 1' }, { name: 'to', label: 'To part', type: 'number', placeholder: 'from + 9, or the most parts' }],
        args: v => ['series', 'parts', ...(v.file ? [v.file] : []), ...flag('from', v.from), ...flag('to', v.to)], submit: 'Show',
        ask: v => farPart(v.from, v.to) }),
      releaseForm(ctx),
      checkReleaseForm(ctx),
      // The last two commands poo help lists that no view ran (GREEN-040).
      form(ctx, { title: 'The worked example\'s series', about: 'The series poo init -example follows, as JSON: a starting point for a series file.', fields: [],
        args: () => ['series', 'example'], submit: 'Show' }),
      form(ctx, { title: 'Simulate the network', about: 'The deterministic network simulator, on the worked example\'s series, in this process: it touches no files and no node. The same seed gives the same result. More flags are in the terminal (poo sim load -h).',
        fields: [{ name: 'scenario', label: 'Scenario', type: 'select', options: ['load', 'partition', 'attack', 'sizing'] },
          { name: 'seed', label: 'Seed', type: 'number', placeholder: 'default 1' }, { name: 'runs', label: 'Runs', type: 'number', placeholder: 'default 1, seeds from Seed up' },
          { name: 'nodes', label: 'Nodes', type: 'number', placeholder: 'default 16' }, { name: 'duration', label: 'Duration (ms)', type: 'number', placeholder: 'default 60000' },
          { name: 'sizes', label: 'Pool sizes', placeholder: 'default 1,2,4,8', only: { field: 'scenario', values: ['sizing'] } }],
        args: v => ['sim', v.scenario || 'load', ...flag('seed', v.seed), ...flag('runs', v.runs), ...flag('nodes', v.nodes), ...flag('duration', v.duration), ...flag('sizes', v.sizes)], submit: 'Simulate' }),
    ]);
  },
};

// FAR_PART is the part from which working out a part's id takes long
// enough to ask first. poo hashes every part id up to K the first time a
// process needs part K, at a few million a second: part 10^8 takes some
// 30 s, part 5x10^10 hours. Stop cannot end that until poo checks for it
// (GUI-028, routed to Source), and the job holds one of poo-gui's job
// slots while it runs. The GUI knows K, so it asks without a poo call.
const FAR_PART = 100000000n;
function farPart(...parts) {
  let far = 0n;
  for (const p of parts) if (/^\d+$/.test(p || '')) { const k = BigInt(p); if (k > far) far = k; }
  if (far < FAR_PART) return null;
  return `Part ${far} takes poo about ${far} hashes to work out, some 30 s for each 10^8, so a far part takes hours. Once started, Stop cannot end it, and it holds one of the GUI's job slots until it ends. Run it?`;
}

// releaseForm is Release a part (D163) in two steps, like the imports
// (GUI-023): Preview runs poo series parts for part K and says which
// serials the release brings into being, then Sign, under the preview,
// signs that part of that series and nothing else. A release can never be
// taken back and releases every part before it, so the seed is asked for
// only once the person has seen the range (GUI-027). Any change to the
// form takes Sign away. After signing, the release poo printed is held to
// the preview: the same series and the same part.
function releaseForm(ctx) {
  let previewed = null, dropSign = () => {};
  const f = form(ctx, { title: 'Release a part', about: 'Signs part K of a variable series as its issuer and prints the release to publish. Preview first shows which serials the release brings into being; Sign, under it, then signs that part. A release is permanent: nothing can take it back, and it also releases every part before it. poo asks for the issuer key\'s seed in a masked field.',
    fields: [{ name: 'file', label: 'Series file', placeholder: 'series.json', required: true }, { name: 'part', label: 'Part', type: 'number', placeholder: 'from 2 on', required: true }, { name: 'alg', label: 'Key algorithm', placeholder: 'ed25519', list: 'crypto-algs', help: 'The issuer key\'s algorithm, from the list above (-alg).' }],
    args: v => ['series', 'parts', ...(v.file ? [v.file] : []), ...flag('from', v.part), ...flag('to', v.part)], submit: 'Preview',
    ask: v => farPart(v.part),
    onStart: () => dropSign(),
    onResult: (r, area) => {
      if (r.code !== 0) return;
      let info;
      try { info = JSON.parse(r.out); } catch (_) { return; }
      const pt = info && Array.isArray(info.parts) && info.parts.length === 1 ? info.parts[0] : null;
      if (!pt || typeof info.series_id !== 'string') return;
      // Part 1 is released by the series' own issuer witness (Sign a
      // series); poo releases parts from 2 on.
      if (pt.part < 2) { area.append(el('p', { class: 'hint' }, 'Part 1 is released by the issuer witness on the series (Sign a series above); a release is for part 2 on.')); return; }
      const file = f.inputs.file.value.trim();
      // The key algorithm is kept with what was previewed, so Sign signs
      // with the one shown when the preview ran.
      previewed = { file, part: pt.part, series: info.series_id, last: pt.last_serial, alg: f.inputs.alg.value.trim() };
      // The mints that exist once the part is released are poo's count
      // (mints_released, D168); the GUI adds nothing up.
      const range = `serials ${pt.first_serial} to ${pt.last_serial}, and with every part before it, serials 0 to ${pt.last_serial}${pt.mints_released !== undefined ? `: ${pt.mints_released} mints` : ''}`;
      const what = el('p', { class: 'next-step release-range', role: 'alert' },
        `Read first: releasing part ${pt.part} of series ${info.series_id.slice(0, 12)} brings into being ${range}. It can never be taken back.`);
      const sign = el('button', { type: 'button', class: 'danger' }, `Sign part ${pt.part}`);
      const out = el('div', { class: 'area' });
      const actions = el('div', { class: 'actions' }, sign);
      sign.onclick = () => {
        const p = previewed;
        const a = ['series', 'release', p.file, '-part', String(p.part), ...flag('alg', p.alg), '-seed-file', '-'];
        if (!confirm(`Release part ${p.part} of series ${p.series.slice(0, 12)}: ${range}? This runs ${api.quote(a)} and cannot be taken back.`)) return;
        sign.disabled = true;
        dropSign = () => {};
        runJob(a, out).then(r2 => {
          if (r2.code !== 0 || !r2.out.trim().startsWith('{')) return;
          let rel = null;
          try { rel = JSON.parse(r2.out); } catch (_) { /* shown as printed */ }
          const same = rel && rel.series === p.series && rel.part === p.part;
          // A release that is not the one previewed is not offered to
          // save: it says do not publish it (GUI/Red R26B).
          if (same) out.append(el('p', { class: 'run-status ok release-made' }, `Signed: part ${p.part} of series ${p.series.slice(0, 12)}, ${range}. poo checks it below.`), saveAs(r2.out, `release-${p.part}.json`));
          else out.append(el('p', { class: 'run-status bad release-differs', role: 'alert' }, 'This release is not the part previewed (the series file changed?): do not publish it. Preview again.'));
          // poo checks the release just printed, read from its input
          // (-check -), so what is saved is what poo says it releases.
          if (same) {
            const checked = el('div', { class: 'area release-check' });
            out.append(checked);
            runJob(['series', 'release', p.file, '-check', '-'], checked, { onStart: j => api.inputFile(j.id, new Blob([r2.out])).catch(() => {}) });
          }
        });
      };
      dropSign = () => { what.remove(); actions.remove(); previewed = null; dropSign = () => {}; };
      area.append(what, actions, out);
    } });
  f.form.addEventListener('input', () => dropSign());
  f.form.addEventListener('change', () => dropSign());
  return f;
}

// Check a release takes the release file by path, or picked in the
// browser and sent to poo as its input (-check -, D166), as Add a release
// and the imports take theirs (GUI-GREEN-052): a release made elsewhere
// can be checked before it is given to a node.
function checkReleaseForm(ctx) {
  let picker = null;
  const f = form(ctx, { title: 'Check a release', about: 'Checks a saved release file against its series and says which serials it releases: for a release made elsewhere, or after Sign above.',
    fields: [{ name: 'file', label: 'Series file', placeholder: 'series.json', required: true }, { name: 'check', label: 'Release file', placeholder: 'release.json, or pick one below', required: true }],
    args: v => ['series', 'release', ...(v.file ? [v.file] : []), ...flag('check', v.check)], submit: 'Check',
    onStart: j => picker.feed(j) });
  picker = pickFile(f, { field: 'check' });
  return f;
}
