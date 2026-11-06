// The GUI's shell: navigation between views. Every view is a way to build
// and run poon command lines (views/); the terminal runs typed ones.
import * as api from './api.js';
import * as terminal from './terminal.js';
import { views } from './views/index.js';
import { startView } from './start.js';
import * as where from './where.js';

where.applyTheme();
const whereEl = document.getElementById('where');
const showWhere = () => {
  const f = where.flags();
  whereEl.textContent = f.length ? f.join(' ') : 'default node policy';
};
showWhere();
where.onChange(showWhere);

const nav = document.getElementById('nav');
const main = document.getElementById('main');

// The terminal stays mounted so a running command and its scrollback
// survive switching views.
const termPane = document.createElement('div');
termPane.className = 'pane';
const term = terminal.mount(termPane);
main.append(termPane);

const viewPane = document.createElement('div');
viewPane.className = 'pane';
main.append(viewPane);

const all = [startView, ...views, { id: 'terminal', title: 'Terminal' }];
for (const v of all) {
  const a = document.createElement('a');
  a.href = `#/${v.id}`;
  a.textContent = v.title;
  a.dataset.id = v.id;
  nav.append(a);
}

// On a narrow window the views are a menu instead of a row of tabs that
// scrolls out of sight. Both are the same links; CSS shows one of them.
const menu = document.createElement('select');
menu.id = 'nav-menu';
menu.setAttribute('aria-label', 'View');
for (const v of all) menu.append(new Option(v.title, v.id));
menu.addEventListener('change', () => { location.hash = `#/${menu.value}`; });
nav.before(menu);

let stopView = null;

// openInTerminal shows the terminal with a command line typed and run, so
// a view's "Run in terminal" is exactly typing it.
export function openInTerminal(text) {
  location.hash = '#/terminal';
  term.run(text);
}

function show() {
  // A view may take parameters after "?", for example #/explore?tid=HEX,
  // which a link elsewhere uses to open it with a field filled in.
  const [id, query] = location.hash.replace(/^#\//, '').split('?');
  const v = all.find(x => x.id === id) || all[0];
  const params = new URLSearchParams(query || '');
  for (const a of nav.children) {
    a.classList.toggle('active', a.dataset.id === v.id);
    if (a.dataset.id === v.id) a.setAttribute('aria-current', 'page'); else a.removeAttribute('aria-current');
  }
  document.title = `${v.title} · poon GUI`;
  menu.value = v.id;
  if (stopView) { stopView(); stopView = null; }
  const isTerm = v.id === 'terminal';
  termPane.hidden = !isTerm;
  viewPane.hidden = isTerm;
  if (isTerm) { term.focus(); return; }
  viewPane.textContent = '';
  stopView = v.mount(viewPane, { openInTerminal, params }) || null;
}

// The skip link moves focus without touching the hash the views use.
document.querySelector('.skip').addEventListener('click', ev => {
  ev.preventDefault();
  main.focus();
});

window.addEventListener('hashchange', show);
api.loadInfo().then(() => {
  document.getElementById('view-only').hidden = !api.viewOnly;
  // The release this GUI runs, as poon version prints it (GUI White G4,
  // GUI-GREEN-058): for a bug report, and to compare with a node's.
  const ver = document.getElementById('version');
  if (typeof api.version === 'string' && api.version) { ver.textContent = `v${api.version}`; ver.hidden = false; }
  show();
});
