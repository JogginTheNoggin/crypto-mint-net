// Getting started: the first steps with poo as a list of command lines.
// Each step only types its line into the terminal; the terminal runs it
// exactly as poo would, so the guide teaches the headless tool too. Steps
// marked policy get the -root and -policy from Settings, so they act on
// the same node policy as the views; with none set, the line is unchanged.
import { el } from './views.js';
import * as api from './api.js';
import * as where from './where.js';

const steps = [
  { title: 'Check the tool', line: 'version',
    text: 'Prints the version of the headless code this GUI runs.' },
  { title: 'Watch a network at work', line: 'demo -quiet',
    text: 'Runs a few full nodes in this process through a payment, a double spend and its resolution. It touches no files.' },
  { title: 'See the cryptographic choices', line: 'crypto',
    text: 'Lists the hashes, key algorithms and policy types a series can choose from.' },
  { title: 'Write a node policy', line: 'init -example', policy: true,
    text: 'Writes a node policy that follows the worked example\'s series and makes it current. Add -p2p and -peer to join other nodes.' },
  { title: 'Run the node', line: 'node', policy: true,
    text: 'Starts the full node. Use "Run in background" in the terminal to keep it going while you do the next steps.' },
  { title: 'Look at the node', line: 'status', policy: true, view: 'node',
    text: 'Every pool the running node follows. "peers" and "chases" show the rest of its view.' },
  { title: 'Check its health', line: 'doctor', policy: true, view: 'node',
    text: 'Checks the node policy, files, series, RPC and P2P, and says what to fix. The Node view\'s Health check is the same command.' },
  { title: 'Watch it work', line: 'watch', policy: true, view: 'watch',
    text: 'What the running node is doing, one line per event: transactions arriving, entries and layers kept. It keeps going until you stop it; the Watch view shows the same events as they come.' },
  { title: 'See what can grind', line: 'grind devices', policy: true, view: 'work',
    text: 'Each device on this machine, what it grinds fast, and which one auto picks for each hash.' },
  { title: 'Help secure the ledger', line: 'grind start tip', policy: true, view: 'work',
    text: 'Starts a job in the running node that adds layers on each pool\'s selected tip. "grind jobs" lists it and "grind stop -all" ends it; the Work view does both, and schedules jobs the node runs by itself.' },
  { title: 'Make a wallet', line: 'wallet create', policy: true, view: 'wallet',
    text: 'Makes a wallet from a new seed, sealed under a passphrase you type into the masked field.' },
  { title: 'Get an address', line: 'wallet address', policy: true, view: 'wallet',
    text: 'Gives out a fresh address to be paid at.' },
];

const TITLES = { node: 'Node', watch: 'Watch', work: 'Work', wallet: 'Wallet' };
const viewTitle = id => TITLES[id] || id;

export const startView = {
  id: 'start',
  title: 'Getting started',
  mount(root, ctx) {
    const list = el('ol', { class: 'steps' });
    const flags = api.quote(where.flags()).replace(/^poo ?/, '');
    for (const st of steps) {
      const s = { ...st, line: st.policy && flags ? `${st.line} ${flags}` : st.line };
      list.append(el('li', {},
        el('h2', {}, s.title),
        el('p', {}, s.text),
        el('div', { class: 'step-cmd' },
          el('code', {}, `poo ${s.line}`),
          el('button', { type: 'button', class: 'primary', onclick: () => ctx.openInTerminal(s.line) }, 'Run in terminal'),
          // The view that runs the same command with forms (GREEN-036).
          s.view ? el('a', { href: `#/${s.view}`, class: 'step-view' }, `Open the ${viewTitle(s.view)} view`) : null)));
    }
    root.append(el('h1', {}, 'Getting started'),
      el('p', { class: 'hint' }, 'Each step is one poo command. Running it here is the same as typing it after poo in a shell.'),
      list);
  },
};
