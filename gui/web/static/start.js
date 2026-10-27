// Getting started: the first steps with poo as a list of command lines.
// Each step only types its line into the terminal; the terminal runs it
// exactly as poo would, so the guide teaches the headless tool too.
import { el } from './views.js';

const steps = [
  { title: 'Check the tool', line: 'version',
    text: 'Prints the version of the headless code this GUI runs.' },
  { title: 'Watch a network at work', line: 'demo -quiet',
    text: 'Runs a few full nodes in this process through a payment, a double spend and its resolution. It touches no files.' },
  { title: 'See the cryptographic choices', line: 'crypto',
    text: 'Lists the hashes, key algorithms and policy types a series can choose from.' },
  { title: 'Write a node policy', line: 'init -example',
    text: 'Writes a node policy that follows the worked example\'s series and makes it current. Add -p2p and -peer to join other nodes.' },
  { title: 'Run the node', line: 'node',
    text: 'Starts the full node. Use "Run in background" in the terminal to keep it going while you do the next steps.' },
  { title: 'Look at the node', line: 'status',
    text: 'Every pool the running node follows. "peers" and "chases" show the rest of its view.' },
  { title: 'Make a wallet', line: 'wallet create',
    text: 'Makes a wallet from a new seed, sealed under a passphrase you type into the masked field.' },
  { title: 'Get an address', line: 'wallet address',
    text: 'Gives out a fresh address to be paid at.' },
];

export const startView = {
  id: 'start',
  title: 'Getting started',
  mount(root, ctx) {
    const list = el('ol', { class: 'steps' });
    for (const s of steps) {
      list.append(el('li', {},
        el('h2', {}, s.title),
        el('p', {}, s.text),
        el('div', { class: 'step-cmd' },
          el('code', {}, `poo ${s.line}`),
          el('button', { type: 'button', class: 'primary', onclick: () => ctx.openInTerminal(s.line) }, 'Run in terminal'))));
    }
    root.append(el('h1', {}, 'Getting started'),
      el('p', { class: 'hint' }, 'Each step is one poo command. Running it here is the same as typing it after poo in a shell.'),
      list);
  },
};
