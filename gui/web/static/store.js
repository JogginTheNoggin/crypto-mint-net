// The names of everything the GUI keeps in the browser's storage, in one
// place, so a rename of the GUI touches one file (GUI-037). Each name
// starts with PREFIX.
//
// There is no migration from earlier names, and no list of them: nothing
// has been released, so the rename to poon (D204) started the browser's
// storage fresh, by jerry's decision (R40). A test profile that ran a
// build from before the rename keeps its old entries until the browser's
// site data for poon-gui's address is cleared.
export const PREFIX = 'poon-gui-';

export const HISTORY = PREFIX + 'history';             // terminal.js: saved terminal lines
export const WHERE = PREFIX + 'where';                 // where.js: Settings' -root and -policy
export const THEME = PREFIX + 'theme';                 // where.js: light or dark
export const WALLET = PREFIX + 'wallet';               // views/wallet.js: the wallet file
export const WALLET_SERIES = PREFIX + 'wallet-series'; // views/wallet.js: the wallet's series
