// The terminal: a typed line goes to the server, which splits it as a
// shell would and runs it through cli.Main, the same as typing it after
// "poo" in a shell. Nothing here changes the command line.
import * as api from './api.js';
import { complete } from './complete.js';

const HISTORY_KEY = 'poo-gui-history';

// A line is saved to the browser's storage unless it may hold a secret,
// such as a seed after -restore, or starts with a space, as a shell with
// HISTCONTROL=ignorespace skips it. Up-arrow still recalls every line of
// this session (GUI-006 in security/gui/findings).
const saveable = text => !/^\s/.test(text) && !api.secretLine(text);

function loadHistory() {
  try { return (JSON.parse(localStorage.getItem(HISTORY_KEY)) || []).filter(saveable); } catch (_) { return []; }
}
function saveHistory(h) {
  try { localStorage.setItem(HISTORY_KEY, JSON.stringify(h.filter(saveable).slice(-200))); } catch (_) { /* storage off */ }
}

export function mount(root) {
  root.innerHTML = `
    <section class="terminal" aria-label="Terminal">
      <div class="term-bar">
        <span>Terminal: the same commands as <code>poo</code> in a shell. Tab completes, Ctrl-C stops a command, Ctrl-L clears.</span>
        <span class="spacer"></span>
        <button type="button" data-act="clear" title="Clear the screen (Ctrl-L)">Clear</button>
        <button type="button" data-act="forget" title="Forget the command lines this browser saved">Forget history</button>
        <button type="button" data-act="show" hidden aria-pressed="false" title="Show what you type to the running command">Show typing</button>
        <button type="button" data-act="detach" disabled title="Keep the running command going in the background">Run in background</button>
        <button type="button" data-act="stop" disabled>Stop</button>
      </div>
      <pre class="term-out" tabindex="0" aria-label="Terminal output"></pre>
      <div class="sr-only" role="status" aria-live="polite"></div>
      <form class="term-in" autocomplete="off">
        <label class="prompt" for="term-line">$ poo</label>
        <input id="term-line" name="line" spellcheck="false" autocapitalize="off" autofocus>
      </form>
    </section>`;
  const out = root.querySelector('.term-out');
  const form = root.querySelector('form');
  const line = root.querySelector('input');
  const prompt = root.querySelector('.prompt');
  const stopBtn = root.querySelector('[data-act=stop]');
  const detachBtn = root.querySelector('[data-act=detach]');
  // Screen readers hear when a command ends, not every chunk it prints.
  const status = root.querySelector('[role=status]');
  const announce = text => { status.textContent = text; };
  const clearBtn = root.querySelector('[data-act=clear]');
  const showBtn = root.querySelector('[data-act=show]');
  const history = loadHistory();
  // Lines saved by an older version may hold a secret; drop them now.
  saveHistory(history);
  let hpos = history.length;
  root.querySelector('[data-act=forget]').addEventListener('click', () => {
    history.length = 0;
    hpos = 0;
    try { localStorage.removeItem(HISTORY_KEY); } catch (_) { /* storage off */ }
    announce('History forgotten');
    line.focus();
  });
  let job = null; // {id, unfollow}
  // section.dataset.runs counts the command lines that have ended, so a
  // test or a script driving the page can tell when one is done.
  const section = root.querySelector('.terminal');
  section.dataset.runs = '0';
  const finished = () => { section.dataset.runs = String(Number(section.dataset.runs) + 1); };

  const write = (text, cls) => {
    const span = document.createElement('span');
    if (cls) span.className = cls;
    span.textContent = text;
    out.append(span);
    out.scrollTop = out.scrollHeight;
  };

  // A passphrase or seed is masked and never echoed, exactly when poo
  // reads one: the core reads every secret through cli.Stdio.Secret
  // (decision D101), and the job's state says so. Other input shows as
  // typed, as in a shell. Show typing unmasks a secret on request.
  let showTyping = false;
  let secret = false;
  const setRunning = j => {
    job = j;
    stopBtn.disabled = detachBtn.disabled = !j;
    secret = showTyping = false;
    refreshInput();
  };
  const refreshInput = () => {
    const masked = !!job && secret && !showTyping;
    line.type = masked ? 'password' : 'text';
    showBtn.hidden = !(job && secret);
    showBtn.setAttribute('aria-pressed', String(showTyping));
    prompt.textContent = job ? (secret ? (masked ? 'secret (hidden)' : 'secret') : 'input') : '$ poo';
  };

  async function runLine(text) {
    // Every line goes to poo, even "clear": clearing the screen is the
    // Clear button and Ctrl-L, never a command (GUI-002).
    write(`$ poo ${text.replace(/^\s*poo(\s+|$)/, '')}\n`, 'echo');
    let j;
    try {
      j = await api.start(text);
    } catch (e) {
      write(`poo-gui: ${e.message}\n`, 'err');
      finished();
      return;
    }
    const cur = { id: j.id };
    setRunning(cur);
    cur.unfollow = api.follow(j.id, {
      chunk: c => write(c.data, c.stream === 'err' ? 'err' : ''),
      dropped: () => write('[earlier output dropped]\n', 'note'),
      state: st => { if (job === cur && st.secret !== secret) { secret = !!st.secret; showTyping = false; refreshInput(); } },
      exit: e => {
        if (e.code !== 0) write(`[exit ${e.code}]\n`, 'note');
        announce(e.code === 0 ? 'Command finished.' : `Command failed with exit ${e.code}.`);
        if (job === cur) setRunning(null);
        finished();
        line.focus();
      },
      error: () => { if (job === cur) { write('[lost the connection to poo-gui]\n', 'err'); setRunning(null); finished(); } },
    });
  }

  form.addEventListener('submit', async ev => {
    ev.preventDefault();
    const text = line.value;
    line.value = '';
    if (job) {
      // The running command reads this line, as from a shell's input.
      // A secret is not echoed (poo ends its line itself, as the headless
      // terminal does); other input is, as a shell's terminal echoes it.
      if (!secret) write(text + '\n', 'echo');
      try { await api.input(job.id, text); } catch (e) { write(`poo-gui: ${e.message}\n`, 'err'); }
      return;
    }
    if (text.trim() === '') return;
    if (history[history.length - 1] !== text) { history.push(text); saveHistory(history); }
    hpos = history.length;
    runLine(text);
  });

  line.addEventListener('keydown', ev => {
    if (ev.ctrlKey && ev.key === 'c' && job && line.selectionStart === line.selectionEnd) {
      ev.preventDefault();
      write('^C\n', 'note');
      api.cancel(job.id).catch(() => {});
    } else if (ev.ctrlKey && ev.key === 'l') {
      ev.preventDefault();
      out.textContent = '';
    } else if (ev.key === 'Tab' && !job && !ev.shiftKey && line.selectionStart === line.value.length) {
      // Completion only reads poo's help; it never changes what runs.
      ev.preventDefault();
      const typed = line.value;
      complete(typed).then(c => {
        if (line.value !== typed) return;
        line.value = c.line;
        if (c.options.length) write(c.options.join('  ') + '\n', 'note');
      });
    } else if (ev.key === 'ArrowUp' && !job && hpos > 0) {
      ev.preventDefault();
      line.value = history[--hpos];
    } else if (ev.key === 'ArrowDown' && !job) {
      ev.preventDefault();
      hpos = Math.min(history.length, hpos + 1);
      line.value = history[hpos] || '';
    }
  });
  clearBtn.addEventListener('click', () => { out.textContent = ''; line.focus(); });
  showBtn.addEventListener('click', () => { showTyping = !showTyping; refreshInput(); line.focus(); });
  stopBtn.addEventListener('click', () => { if (job) { write('^C\n', 'note'); api.cancel(job.id).catch(() => {}); } });
  detachBtn.addEventListener('click', () => {
    if (!job) return;
    write(`[job ${job.id} runs on in the background; see Jobs]\n`, 'note');
    job.unfollow();
    setRunning(null);
    finished();
    line.focus();
  });
  out.addEventListener('click', () => { if (!getSelection().toString()) line.focus(); });
  return { run: text => { line.value = ''; runLine(text); }, focus: () => line.focus() };
}
