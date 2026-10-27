// Next steps for errors a new user meets often. A hint is shown under the
// command's own error, which stays exactly as printed; it only points at
// a view or command that helps. Nothing here decides anything: a hint that
// does not match simply is not shown.
export const HINTS = [
  { match: /is the node running\?/,
    text: 'No node is answering for this node policy. Start one on the Node view, or write a node policy there first.',
    view: 'node', label: 'Go to Node' },
  { match: /node\.json: no such file or directory/,
    text: 'There is no node policy here yet. Write one on the Node view, or pick a root and policy in Settings.',
    view: 'node', label: 'Go to Node' },
  { match: /wallet\.json: no such file or directory/,
    text: 'There is no wallet file here yet. Create one on the Wallet view, or pick another file.',
    view: 'wallet', label: 'Go to Wallet' },
];

// hintFor returns the first hint whose pattern matches the error text.
export function hintFor(err) {
  return HINTS.find(h => h.match.test(err)) || null;
}
