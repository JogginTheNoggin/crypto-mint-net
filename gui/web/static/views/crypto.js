// The Series and keys view. Each view is one file here, listed in index.js (GUI-GREEN-044).
import * as api from '../api.js';
import { el, flag, form, mountAll, panel } from '../views.js';

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
      form(ctx, { title: 'New series parameters', about: 'Prints new series parameters, checked. More options are in the terminal (poo series new -h).',
        fields: [{ name: 'hash', label: 'Hash', placeholder: 'sha256', list: 'crypto-hashes' }, { name: 'algs', label: 'Key algorithms', placeholder: 'comma list: ' + 'see the list above' }, { name: 'types', label: 'Policy types', placeholder: 'comma list: see the list above' }, { name: 'mints', label: 'Mints', type: 'number' }],
        args: v => ['series', 'new', ...flag('hash', v.hash), ...flag('algs', v.algs), ...flag('types', v.types), ...flag('mints', v.mints)], submit: 'Make series' }),
      form(ctx, { title: 'Series file info', fields: [{ name: 'file', label: 'Series file', placeholder: 'series.json' }],
        args: v => ['series', 'info', ...(v.file ? [v.file] : [])], submit: 'Show' }),
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
