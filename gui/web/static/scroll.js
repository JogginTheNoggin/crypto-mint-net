// Scrollback for the panes that show a command's output as it arrives:
// the terminal, a running node's log and a running command (ORANGE-003).
// Like a terminal's scrollback it keeps the newest max characters, so a
// command that prints for hours cannot fill the tab, and it scrolls to
// the end once per frame rather than once per write. Text is shown
// exactly as printed; only the oldest is let go.
export const MAX_CHARS = 1 << 20;
// Output goes into blocks of whole lines, about BLOCK characters each.
// A browser lays out a block of text as a whole, so writing to the last
// small block costs the same however much is above it; one long run of
// text would be laid out again, all of it, on every write.
const BLOCK = 4 << 10;

// A pane being written to often is drawn at most every DRAW_EVERY ms
// (ORANGE-007): the first write after a quiet spell shows at once, as
// typing does in a terminal, and writes that follow it within that time
// are drawn together, so a chatty node log is ten layouts a second, not
// one per line. flush draws what waits at once.
const DRAW_EVERY = 100;

export function scrollback(pre, { max = MAX_CHARS, follow = false } = {}) {
  let blocks = [], size = 0, frame = 0;
  let queue = [], queued = 0, timer = 0, lastDraw = -Infinity;
  const end = () => { frame = 0; pre.scrollTop = pre.scrollHeight; };
  const put = (text, cls) => {
    let b = blocks[blocks.length - 1];
    if (!b || (b.n >= BLOCK && b.nl)) {
      b = { el: document.createElement('div'), n: 0, nl: false, span: null, cls: null };
      b.el.className = 'blk';
      pre.append(b.el);
      blocks.push(b);
    }
    if (b.span && b.cls === cls) b.span.firstChild.appendData(text);
    else {
      b.span = document.createElement('span');
      if (cls) b.span.className = cls;
      b.span.textContent = text;
      b.cls = cls;
      b.el.append(b.span);
    }
    b.n += text.length;
    b.nl = text.endsWith('\n');
    size += text.length;
  };
  const flush = () => {
    clearTimeout(timer);
    timer = 0;
    lastDraw = performance.now();
    if (!queue.length) return;
    for (const [text, cls] of queue) put(text, cls);
    queue = [];
    queued = 0;
    while (size > max && blocks.length > 1) {
      const old = blocks.shift();
      size -= old.n;
      old.el.remove();
    }
    if (follow && !frame) frame = requestAnimationFrame(end);
  };
  return {
    write(text, cls = '') {
      queue.push([text, cls]);
      queued += text.length;
      // What waits is bounded too, for a tab whose timers are slowed.
      while (queued > max && queue.length > 1) queued -= queue.shift()[0].length;
      if (timer) return;
      const wait = lastDraw + DRAW_EVERY - performance.now();
      if (wait <= 0) flush();
      else timer = setTimeout(flush, wait);
    },
    flush,
    clear() { clearTimeout(timer); timer = 0; queue = []; queued = 0; pre.textContent = ''; blocks = []; size = 0; },
  };
}
