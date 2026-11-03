// The Wallet view. Each view is one file here, listed in index.js (GUI-GREEN-044).
import * as where from '../where.js';
import { el, flag, flags, form, mountAll } from '../views.js';

// The Wallet view's wallet file, kept per browser like the Settings flags.
const WALLET_KEY = 'poo-gui-wallet';
const walletFile = () => { try { return localStorage.getItem(WALLET_KEY) || ''; } catch (_) { return ''; } };
const setWalletFile = v => { try { localStorage.setItem(WALLET_KEY, v); } catch (_) { /* storage off */ } };
const walletSeries = () => { try { return localStorage.getItem(WALLET_KEY + '-series') || ''; } catch (_) { return ''; } };
const setWalletSeries = v => { try { localStorage.setItem(WALLET_KEY + '-series', v); } catch (_) { /* storage off */ } };

export const walletView = {
  id: 'wallet',
  title: 'Wallet',
  mount(root, ctx) {
    // The wallet file is chosen once for the whole view, like -root and
    // -policy in Settings, instead of in every form.
    const pick = el('input', { name: 'wallet-file', value: walletFile(), placeholder: 'wallet.json in the root', spellcheck: 'false' });
    // Every poo wallet command takes -series; it is chosen here once, with
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
    return mountAll(root, 'Wallet', 'Keys from one seed, payments and requests, all through poo wallet. Passphrases go straight to the command and are never stored.', [
      { el: picker, stop: () => {} },
      form(ctx, { title: 'Balance', about: 'Spendable units per watched mint. ' + pass, fields: [], args: v => ['wallet', 'balance', ...wf(v)], submit: 'Show balance' }),
      form(ctx, { title: 'Addresses', about: 'Every address, used or not, and shared ones. ' + pass, fields: [], args: v => ['wallet', 'addresses', ...wf(v)], submit: 'Show addresses' }),
      form(ctx, { title: 'New address', fields: [{ name: 'label', label: 'Label' }], args: v => ['wallet', 'address', ...wf(v), ...flag('label', v.label)], submit: 'Give out an address' }),
      form(ctx, { title: 'Pay', about: 'Pays an address, or a payment request file.', fields: [
        { name: 'to', label: 'To address' }, { name: 'amount', label: 'Amount', type: 'number' },
        { name: 'request', label: 'Or a request file' }, { name: 'mint', label: 'Only from mints', type: 'list', placeholder: 'serials …' },
        { name: 'memo', label: 'Memo' }, { name: 'out', label: 'Co-signing file', placeholder: 'for a shared policy' }],
      args: v => ['wallet', 'pay', ...wf(v), ...flag('to', v.to), ...flag('amount', v.amount), ...flag('request', v.request), ...flags('mint', v.mint), ...flag('memo', v.memo), ...flag('out', v.out)],
      submit: 'Pay', danger: true }),
      // A shared k-of-n payment from start to end (GREEN-039): Pay with a
      // co-signing file writes what co-signers sign; each runs cosign on
      // it, and submit sends it once enough have signed.
      form(ctx, { title: 'Record a shared policy', about: 'A k-of-n address this wallet signs for with others. self is a fresh key of this wallet; give each co-signer\'s public key. ' + pass, fields: [
        { name: 'name', label: 'Name' }, { name: 'k', label: 'Signatures needed', type: 'number', placeholder: 'default 1' },
        { name: 'key', label: 'Keys', type: 'list', placeholder: 'self, ALG:HEX or HEX …' }, { name: 'label', label: 'Label' }],
        args: v => ['wallet', 'policy', ...wf(v), ...flag('name', v.name), ...flag('k', v.k), ...flags('key', v.key), ...flag('label', v.label)], submit: 'Record' }),
      form(ctx, { title: 'Co-sign a payment', about: 'Adds this wallet\'s signatures to a co-signing file another signer wrote with Pay. ' + pass, fields: [{ name: 'file', label: 'Co-signing file' }],
        args: v => ['wallet', 'cosign', ...wf(v), ...(v.file ? [v.file] : [])], submit: 'Co-sign' }),
      form(ctx, { title: 'Submit a co-signed payment', about: 'Sends a payment whose co-signing file has every signature it needs.', fields: [{ name: 'file', label: 'Co-signing file' }],
        args: v => ['wallet', 'submit', ...wf(v), ...(v.file ? [v.file] : [])], submit: 'Submit', danger: true }),
      form(ctx, { title: 'Request a payment', about: 'Asks to be paid, to a fresh address.', fields: [{ name: 'amount', label: 'Amount', type: 'number' }, { name: 'memo', label: 'What for' }, { name: 'out', label: 'Request file', placeholder: 'default: print it' }],
        args: v => ['wallet', 'request', ...wf(v), ...flag('amount', v.amount), ...flag('memo', v.memo), ...flag('out', v.out)], submit: 'Make request' }),
      // -serial N -tid HASH pairs name transactions the payer told you of;
      // poo pairs them in order and says if the counts differ (GUI-GREEN-041).
      form(ctx, { title: 'What has arrived', fields: [{ name: 'address', label: 'Requested address' }, { name: 'request', label: 'Or the request file' }, { name: 'threshold', label: 'Threshold', placeholder: 'from(e) for settled' },
        { name: 'tid', label: 'Transactions the payer named', type: 'list', placeholder: 'optional: tid …' }, { name: 'tidSerial', label: 'Their mint serials', type: 'list', placeholder: 'one per transaction, in order' }],
        args: v => ['wallet', 'receive', ...wf(v), ...flag('address', v.address), ...flag('request', v.request), ...flag('threshold', v.threshold), ...flags('serial', v.tidSerial), ...flags('tid', v.tid)], submit: 'Check' }),
      form(ctx, { title: 'Payments sent', about: 'Where every payment sent stands.', fields: [{ name: 'threshold', label: 'Threshold', placeholder: 'from(e) for settled' }],
        args: v => ['wallet', 'status', ...wf(v), ...flag('threshold', v.threshold)], submit: 'Show' }),
      form(ctx, { title: 'Send dropped legs again', about: 'Sends again the parts of payments the node dropped (Payments sent shows them). ' + pass, fields: [],
        args: v => ['wallet', 'resend', ...wf(v)], submit: 'Resend' }),
      form(ctx, { title: 'Watch mints', about: 'Adds mints or pools to look for portions in.', fields: [{ name: 'serial', label: 'Mint serials', type: 'list' }, { name: 'pool', label: 'Pools', type: 'list' }],
        args: v => ['wallet', 'watch', ...wf(v), ...flags('serial', v.serial), ...flags('pool', v.pool)], submit: 'Watch' }),
      form(ctx, { title: 'Claim a mint', fields: [{ name: 'serial', label: 'Mint serial', type: 'number' }],
        args: v => ['wallet', 'claim', ...wf(v), ...flag('serial', v.serial)], submit: 'Claim', danger: true }),
      form(ctx, { title: 'Follow only the watched mints', about: 'Light mode: the node policy follows only the pools of the mints this wallet watches, in place of what it follows now.', fields: [],
        args: v => ['wallet', 'follow', ...wf(v)], submit: 'Follow only these', danger: true }),
      form(ctx, { title: 'Change the passphrase', about: 'Seals the seed under a new passphrase. poo asks for the old one, then the new one, each in a masked field.', fields: [],
        args: v => ['wallet', 'passwd', ...wf(v)], submit: 'Change' }),
      form(ctx, { title: 'Show the seed', about: 'Prints the seed, to back it up. Anyone with it can spend from this wallet. The output is hidden until you show it, and Jobs hides it.', fields: [],
        args: v => ['wallet', 'seed', ...wf(v)], submit: 'Show seed', danger: true, secretOutput: true }),
      // Restoring runs -restore -, so poo asks for the seed through its
      // secret prompt (D106) and the seed is typed into a masked field,
      // never onto the command line.
      form(ctx, { title: 'Create or restore a wallet', fields: [{ name: 'alg', label: 'Key algorithm', placeholder: 'default ed25519', list: 'crypto-algs' }, { name: 'restore', label: 'Restore from a seed', type: 'check', help: 'poo asks for the seed in a masked field after you press Create.' }, { name: 'gap', label: 'Gap', type: 'number' }],
        args: v => ['wallet', 'create', ...wf(v), ...flag('alg', v.alg), ...(v.restore ? ['-restore', '-'] : []), ...flag('gap', v.gap)], submit: 'Create' }),
    ]);
  },
};
