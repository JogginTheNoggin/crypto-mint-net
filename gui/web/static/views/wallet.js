// The Wallet view. Each view is one file here, listed in index.js (GUI-GREEN-044).
import * as api from '../api.js';
import * as store from '../store.js';
import * as where from '../where.js';
import { el, flag, flags, form, mountAll } from '../views.js';

// The Wallet view's wallet file, kept per browser like the Settings flags.
const WALLET_KEY = store.WALLET;
export const walletFile = () => { try { return localStorage.getItem(WALLET_KEY) || ''; } catch (_) { return ''; } };
const setWalletFile = v => { try { localStorage.setItem(WALLET_KEY, v); } catch (_) { /* storage off */ } };
const walletSeries = () => { try { return localStorage.getItem(store.WALLET_SERIES) || ''; } catch (_) { return ''; } };
// signNote is the line over poon sign's reply (D193): which key signed,
// how many signatures it added and how many are still needed, read from
// poon's own fields. null when poon printed no reply.
export function signNote(r) {
  let o = null;
  try { o = JSON.parse(r.out); } catch (_) { return null; }
  if (!o || typeof o.address !== 'string' || !Array.isArray(o.missing)) return null;
  const left = o.missing.flat().reduce((a, n) => a + (Number(n) || 0), 0);
  const legs = Array.isArray(o.legs) ? o.legs.length : 0;
  const pays = `${legs} ${legs === 1 ? 'transaction' : 'transactions'}`;
  const still = left ? `${left} ${left === 1 ? 'signature is' : 'signatures are'} still needed` : 'no more signatures are needed: Submit a co-signed payment sends it';
  const text = o.shown
    ? `Shown only, nothing was signed: the payment has ${pays}, and this key's address is ${o.address}; ${still}.`
    : `This key (address ${o.address}) added ${o.added} ${o.added === 1 ? 'signature' : 'signatures'} to ${pays}${o.wrote ? `, written to ${o.wrote}` : ''}; ${still}.`;
  return el('p', { class: 'hint sign-note', role: 'status' }, text);
}
const setWalletSeries = v => { try { localStorage.setItem(store.WALLET_SERIES, v); } catch (_) { /* storage off */ } };

// payQuestion is Pay's one confirm. With no co-signing file it says what
// a payment that needs a signature this wallet cannot give (a watch-only
// wallet, a shared policy) needs: the file is what the key signs, and poon
// refuses such a payment without one, before recording anything (Source
// #291, D194; GUI-GREEN-061). The GUI cannot tell which wallet it is; poon
// can.
export function payQuestion(line, out) {
  return out ? `Run ${line}?` : `Run ${line}?\n\nNo co-signing file is filled in. If this wallet is watch-only, or pays from a shared policy, fill one in first: it is the file the key signs, and poon refuses such a payment without one.`;
}

// COLD_STEPS is a key kept off this machine from start to end (D193):
// where each step runs, and the form or view that runs it.
const COLD_STEPS = [
  ['Where the key is', 'Make the key, or bring one made by another tool (-raw). Note its algorithm and public key; the key itself never comes here.', { view: 'crypto', label: 'Make a key (Series and keys)' }],
  ['Here', 'Make a wallet that holds no private key.', { form: 'Create or restore a wallet', label: 'Create, with Watch-only ticked' }],
  ['Here', 'Record the key: 1 signature needed, key ALG:PUBLIC (for example ed25519:d404…). Its address is where the coins go.', { form: 'Record a shared policy', label: 'Record a shared policy' }],
  ['Here', 'Watch the mints the key holds, and claim a mint to the key: Claim a mint with To a wallet policy and a claim file, which the key signs like a payment.', { form: 'Claim a mint', label: 'Claim a mint' }],
  ['Here', 'Pay with a co-signing file, every time: it is what the key signs.', { form: 'Pay', label: 'Pay' }],
  ['Where the key is', 'Sign the file. Show only first, to read what it pays. On an offline machine, copy the line and run it there.', { form: 'Sign with a key kept elsewhere', label: 'Sign with a key kept elsewhere' }],
  ['Here', 'Send the signed file.', { form: 'Submit a co-signed payment', label: 'Submit a co-signed payment' }],
];
function coldSteps(root) {
  const go = title => {
    const h = [...root.querySelectorAll('section.panel h2')].find(x => x.textContent === title);
    if (!h) return;
    h.closest('section').scrollIntoView({ block: 'start' });
    h.tabIndex = -1;
    h.focus({ preventScroll: true });
  };
  const ol = el('ol', { class: 'cold-steps' }, ...COLD_STEPS.map(([where, text, to]) => el('li', {}, el('strong', {}, `${where}: `), text, ' ',
    to.view ? el('a', { href: `#/${to.view}` }, to.label) : el('button', { type: 'button', class: 'small', onclick: () => go(to.form) }, to.label))));
  const det = el('details', { class: 'cold' }, el('summary', {}, 'Seven steps, from making the key to a payment sent'), ol);
  const sec = el('section', { class: 'panel cold-key' }, el('div', { class: 'panel-head' }, el('h2', {}, 'Keep the key off this machine')),
    el('p', { class: 'hint' }, 'A watch-only wallet sees balances and prepares payments; the key that signs them stays on another machine, offline if you like (D193).'), det);
  return { el: sec, stop: () => {} };
}

// looksLikeKey (api.js) says a Key file value is a key rather than a path
// (GUI-032). Such a value would show on the command line, in poon's error
// and in the Jobs list, so the form never sends it: the line shows … in
// its place, and Sign refuses it and empties the field. poon-gui hides such
// a value in a job's command line as well (shell.Redact), and the terminal
// keeps such a line out of its saved history (api.secretLine, GUI-034).
export const looksLikeKey = api.looksLikeKey;
export const keyPathArg = s => !s ? '-' : looksLikeKey(s) ? '…' : s;

export const walletView = {
  id: 'wallet',
  title: 'Wallet',
  mount(root, ctx) {
    // The wallet file is chosen once for the whole view, like -root and
    // -policy in Settings, instead of in every form.
    const pick = el('input', { name: 'wallet-file', value: walletFile(), placeholder: 'wallet.json in the root', spellcheck: 'false' });
    // Every poon wallet command takes -series; it is chosen here once, with
    // the file, instead of in three forms and missing from the rest
    // (GUI-GREEN-041).
    const pickSeries = el('input', { name: 'wallet-series', value: walletSeries(), placeholder: 'only if the node follows more than one', spellcheck: 'false' });
    const wf = () => [...where.flags(), ...flag('wallet', pick.value.trim()), ...flag('series', pickSeries.value.trim())];
    const picker = el('section', { class: 'panel wallet-pick' },
      el('label', {}, el('span', {}, 'Wallet file (-wallet)'), pick),
      el('label', {}, el('span', {}, 'Series (-series)'), pickSeries),
      el('small', { class: 'hint' }, 'Every form below uses this file and series. Empty means wallet.json in the root and the only followed series, as in a shell.'));
    const changed = () => {
      setWalletFile(pick.value.trim());
      setWalletSeries(pickSeries.value.trim());
      // Each form's preview shows the new -wallet at once.
      for (const f of root.querySelectorAll('form.fields')) f.dispatchEvent(new Event('input'));
    };
    pick.addEventListener('input', changed);
    pickSeries.addEventListener('input', changed);
    const pass = 'The command asks for the passphrase when it needs it.';
    const payArgs = v => ['wallet', 'pay', ...wf(v), ...flag('to', v.to), ...flag('amount', v.amount), ...flag('request', v.request), ...flags('mint', v.mint), ...flag('memo', v.memo), ...flag('out', v.out), ...flag('change', v.change)];
    return mountAll(root, 'Wallet', 'Keys from one seed, payments and requests, all through poon wallet. Passphrases go straight to the command and are never stored.', [
      { el: picker, stop: () => {} },
      coldSteps(root),
      form(ctx, { title: 'Balance', about: 'Spendable units per watched mint. ' + pass, fields: [], args: v => ['wallet', 'balance', ...wf(v)], submit: 'Show balance' }),
      form(ctx, { title: 'Addresses', about: 'Every address, used or not, and shared ones. ' + pass, fields: [], args: v => ['wallet', 'addresses', ...wf(v)], submit: 'Show addresses' }),
      form(ctx, { title: 'New address', fields: [{ name: 'label', label: 'Label' }], args: v => ['wallet', 'address', ...wf(v), ...flag('label', v.label)], submit: 'Give out an address' }),
      form(ctx, { title: 'Pay', about: 'Pays an address, or a payment request file.', fields: [
        { name: 'to', label: 'To address' }, { name: 'amount', label: 'Amount', type: 'number' },
        { name: 'request', label: 'Or a request file' }, { name: 'mint', label: 'Only from mints', type: 'list', placeholder: 'serials …' },
        { name: 'memo', label: 'Memo' }, { name: 'out', label: 'Co-signing file', placeholder: 'for a shared policy, or a watch-only wallet' },
        { name: 'change', label: 'Change address', placeholder: 'default: a fresh address (watch-only: back where it came from)' }],
      args: payArgs,
      // Its one confirm warns when no co-signing file is filled in
      // (GUI-GREEN-061).
      danger: true, dangerText: v => payQuestion(api.quote(payArgs(v)), v.out),
      submit: 'Pay' }),
      // A shared k-of-n payment from start to end (GREEN-039): Pay with a
      // co-signing file writes what co-signers sign; each runs cosign on
      // it, and submit sends it once enough have signed.
      form(ctx, { title: 'Record a shared policy', about: 'A k-of-n address this wallet signs for with others. self is a fresh key of this wallet; give each co-signer\'s public key. ' + pass, fields: [
        { name: 'name', label: 'Name' }, { name: 'k', label: 'Signatures needed', type: 'number', placeholder: 'default 1' },
        { name: 'key', label: 'Keys', type: 'list', placeholder: 'self, ALG:HEX or HEX …' }, { name: 'label', label: 'Label' }],
        args: v => ['wallet', 'policy', ...wf(v), ...flag('name', v.name), ...flag('k', v.k), ...flags('key', v.key), ...flag('label', v.label)], submit: 'Record' }),
      form(ctx, { title: 'Co-sign a payment', about: 'Adds this wallet\'s signatures to a co-signing file another signer wrote with Pay. ' + pass, fields: [{ name: 'file', label: 'Co-signing file' }],
        args: v => ['wallet', 'cosign', ...wf(v), ...(v.file ? [v.file] : [])], submit: 'Co-sign' }),
      // Cold keys (D193): poon sign signs a co-signing file with a key kept
      // off the wallet, with no node and no wallet. An empty key file runs
      // -key-file -, so poon asks for the key in a masked field.
      form(ctx, { title: 'Sign with a key kept elsewhere', about: 'Signs a co-signing file (from Pay with a co-signing file) with a key that is not in this wallet, such as a cold key, without the node. Show only first to read what it pays. The same command runs on an offline machine: copy its line.', fields: [
        { name: 'file', label: 'Co-signing file', required: true },
        // The key is asked for in a masked field unless it is in a file;
        // the path field shows only then, so a key on the clipboard has
        // no plain field named Key to be pasted into (GUI-032).
        { name: 'inFile', label: 'The key is in a file (otherwise poon asks for it in a masked field)', type: 'check' },
        { name: 'keyFile', label: 'Key file', placeholder: 'a path on this machine', required: true, only: { field: 'inFile', values: [true] } },
        { name: 'alg', label: 'Key algorithm', placeholder: 'default ed25519', list: 'crypto-algs' },
        { name: 'raw', label: 'The key is a private key in its algorithm\'s usual form, as another tool keeps it, not a poon seed (-raw)', type: 'check' },
        { name: 'hash', label: 'Hash', placeholder: 'only if the file does not name the series\' hash', list: 'crypto-hashes' },
        { name: 'out', label: 'Write the signed payment to', placeholder: 'default: back into the co-signing file' },
        { name: 'show', label: 'Show only: say what it pays and what this key would sign, and sign nothing', type: 'check' }],
        args: v => ['sign', ...(v.file ? [v.file] : []), '-key-file', keyPathArg(v.keyFile), ...flag('alg', v.alg), ...flag('raw', v.raw), ...flag('hash', v.hash), ...flag('out', v.out), ...flag('show', v.show)],
        refuse: v => looksLikeKey(v.keyFile) && { field: 'keyFile', text: 'Key file holds what looks like a key, not a path, so it was emptied. Untick "The key is in a file" and poon asks for the key in a masked field, where it is never shown or kept.' },
        submit: 'Sign', onResult: (r, area) => { const n = signNote(r); if (n) area.prepend(n); } }),
      form(ctx, { title: 'Submit a co-signed payment', about: 'Sends a payment whose co-signing file has every signature it needs.', fields: [{ name: 'file', label: 'Co-signing file' }],
        args: v => ['wallet', 'submit', ...wf(v), ...(v.file ? [v.file] : [])], submit: 'Submit', danger: true }),
      form(ctx, { title: 'Request a payment', about: 'Asks to be paid, to a fresh address.', fields: [{ name: 'amount', label: 'Amount', type: 'number' }, { name: 'memo', label: 'What for' }, { name: 'out', label: 'Request file', placeholder: 'default: print it' }],
        args: v => ['wallet', 'request', ...wf(v), ...flag('amount', v.amount), ...flag('memo', v.memo), ...flag('out', v.out)], submit: 'Make request' }),
      // -serial N -tid HASH pairs name transactions the payer told you of;
      // poon pairs them in order and says if the counts differ (GUI-GREEN-041).
      form(ctx, { title: 'What has arrived', fields: [{ name: 'address', label: 'Requested address' }, { name: 'request', label: 'Or the request file' }, { name: 'threshold', label: 'Threshold', placeholder: 'from(e) for settled' },
        { name: 'tid', label: 'Transactions the payer named', type: 'list', placeholder: 'optional: tid …' }, { name: 'tidSerial', label: 'Their mint serials', type: 'list', placeholder: 'one per transaction, in order' }],
        args: v => ['wallet', 'receive', ...wf(v), ...flag('address', v.address), ...flag('request', v.request), ...flag('threshold', v.threshold), ...flags('serial', v.tidSerial), ...flags('tid', v.tid)], submit: 'Check' }),
      form(ctx, { title: 'Payments sent', about: 'Where every payment sent stands.', fields: [{ name: 'threshold', label: 'Threshold', placeholder: 'from(e) for settled' }],
        args: v => ['wallet', 'status', ...wf(v), ...flag('threshold', v.threshold)], submit: 'Show' }),
      form(ctx, { title: 'Send dropped legs again', about: 'Sends again the parts of payments the node dropped (Payments sent shows them). ' + pass, fields: [],
        args: v => ['wallet', 'resend', ...wf(v)], submit: 'Resend' }),
      form(ctx, { title: 'Watch mints', about: 'Adds mints or pools to look for portions in.', fields: [{ name: 'serial', label: 'Mint serials', type: 'list' }, { name: 'pool', label: 'Pools', type: 'list' }],
        args: v => ['wallet', 'watch', ...wf(v), ...flags('serial', v.serial), ...flags('pool', v.pool)], submit: 'Watch' }),
      // A watch-only wallet claims to a policy's address, unsigned, into a
      // file the key signs (GREEN-060, D194).
      form(ctx, { title: 'Claim a mint', fields: [{ name: 'serial', label: 'Mint serial', type: 'number' },
        { name: 'toPolicy', label: 'To a wallet policy', placeholder: 'optional: a policy\'s name, for a key kept elsewhere (watch-only)' },
        { name: 'out', label: 'Claim file', placeholder: 'with To a wallet policy: the file the key signs', only: { field: 'toPolicy', not: [''] } }],
        args: v => ['wallet', 'claim', ...wf(v), ...flag('serial', v.serial), ...flag('to-policy', v.toPolicy), ...flag('out', v.out)], submit: 'Claim', danger: true }),
      form(ctx, { title: 'Follow only the watched mints', about: 'Light mode: the node policy follows only the pools of the mints this wallet watches, in place of what it follows now.', fields: [],
        args: v => ['wallet', 'follow', ...wf(v)], submit: 'Follow only these', danger: true }),
      form(ctx, { title: 'Change the passphrase', about: 'Seals the seed under a new passphrase. poon asks for the old one, then the new one, each in a masked field.', fields: [],
        args: v => ['wallet', 'passwd', ...wf(v)], submit: 'Change' }),
      form(ctx, { title: 'Show the seed', about: 'Prints the seed, to back it up. Anyone with it can spend from this wallet. The output is hidden until you show it, and Jobs hides it.', fields: [],
        args: v => ['wallet', 'seed', ...wf(v)], submit: 'Show seed', danger: true, secretOutput: true }),
      // Restoring runs -restore -, so poon asks for the seed through its
      // secret prompt (D106) and the seed is typed into a masked field,
      // never onto the command line.
      // Watch-only (D193): the wallet holds no private key and needs no
      // passphrase; keys kept elsewhere are added with Record a shared
      // policy (1 signature, ALG:HEX), payments go to a co-signing file and
      // are signed with Sign with a key kept elsewhere.
      form(ctx, { title: 'Create or restore a wallet', fields: [{ name: 'alg', label: 'Key algorithm', placeholder: 'default ed25519', list: 'crypto-algs' }, { name: 'restore', label: 'Restore from a seed', type: 'check', help: 'poon asks for the seed in a masked field after you press Create.' }, { name: 'gap', label: 'Gap', type: 'number' },
        { name: 'watchOnly', label: 'Watch-only: hold no private key at all (-watch-only)', type: 'check', help: 'Add keys kept elsewhere with Record a shared policy (1 signature needed, key ALG:HEX), pay with a co-signing file, sign it with Sign with a key kept elsewhere, then Submit a co-signed payment.' }],
        args: v => ['wallet', 'create', ...wf(v), ...flag('alg', v.alg), ...(v.restore ? ['-restore', '-'] : []), ...flag('gap', v.gap), ...flag('watch-only', v.watchOnly)], submit: 'Create' }),
    ]);
  },
};
