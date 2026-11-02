// The GUI's shell: navigation between views. Every view is a way to build
// and run poo command lines (views.js); the terminal runs typed ones.
import * as terminal from './terminal.js';
import { views } from './views.js';
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

const all = [...views, { id: 'terminal', title: 'Terminal' }];
for (const v of all) {
  const a = document.createElement('a');
  a.href = `#/${v.id}`;
  a.textContent = v.title;
  a.dataset.id = v.id;
  nav.append(a);
}

let stopView = null;

// openInTerminal shows the terminal with a command line typed and run, so
// a view's "Run in terminal" is exactly typing it.
export function openInTerminal(text) {
  location.hash = '#/terminal';
  term.run(text);
}

function show() {
  const id = location.hash.replace(/^#\//, '') || all[0].id;
  const v = all.find(x => x.id === id) || all[0];
  for (const a of nav.children) a.classList.toggle('active', a.dataset.id === v.id);
  if (stopView) { stopView(); stopView = null; }
  const isTerm = v.id === 'terminal';
  termPane.hidden = !isTerm;
  viewPane.hidden = isTerm;
  if (isTerm) { term.focus(); return; }
  viewPane.textContent = '';
  stopView = v.mount(viewPane, { openInTerminal }) || null;
}

window.addEventListener('hashchange', show);
show();
