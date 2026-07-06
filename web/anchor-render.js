// anchor-render.js — maps between a block's NORMALIZED rune-offset space (the
// space the server anchors in) and the live DOM, and paints highlights.
//
// The server sends placements as rune offsets into a block's normalized text.
// The DOM text is NOT normalized (raw whitespace, inline <code>/<a>/<em>…), so
// we replay the exact same whitespace rule as pkg/render.normalizeText —
// collapse every run of whitespace to a single space, trim the ends — while
// tracking, per Unicode code point, the DOM (textNode, utf16 offset) it came
// from. That gives a rune-offset ⇄ DOM-position bridge used by both the
// highlighter and the new-comment selection path.

// Whitespace test approximating Go's unicode.IsSpace (covers the common cases:
// ASCII spaces, NBSP, NEL, and the JS \s set).
const WS = /[\s ]/u;

// buildCharMap walks the text nodes of blockEl and returns, per emitted concept:
//   entries: one record per DOM code point {node, offset, len, norm}
//            norm = the normalized rune index it maps to, or -1 if trimmed
//   text:    the block's normalized text (runes)
//   len:     number of runes in text
export function buildCharMap(blockEl) {
  const walker = document.createTreeWalker(blockEl, NodeFilter.SHOW_TEXT, null);
  const raw = [];
  let n;
  while ((n = walker.nextNode())) {
    let off = 0;
    for (const ch of n.data) {
      raw.push({ node: n, offset: off, len: ch.length, ch });
      off += ch.length;
    }
  }

  const out = [];
  let text = "";
  let norm = 0;
  let pendingSpace = false;
  let started = false;
  let pending = []; // out-indices of the current whitespace run

  for (let i = 0; i < raw.length; i++) {
    const r = raw[i];
    if (WS.test(r.ch)) {
      pendingSpace = true;
      pending.push(out.length);
      out.push({ node: r.node, offset: r.offset, len: r.len, norm: -1 });
      continue;
    }
    if (pendingSpace && started) {
      // One collapsed space is emitted at index `norm`; attribute the whole
      // whitespace run to it so a highlight covering it includes the run.
      for (const pi of pending) out[pi].norm = norm;
      text += " ";
      norm += 1;
    }
    pending = [];
    pendingSpace = false;
    out.push({ node: r.node, offset: r.offset, len: r.len, norm });
    text += r.ch;
    norm += 1;
    started = true;
  }
  // A trailing whitespace run stays norm=-1 (trimmed).
  return { entries: out, text, len: norm };
}

// sliceRunes returns text[start:end) counted in runes (not UTF-16 units).
export function sliceRunes(text, start, end) {
  return Array.from(text).slice(start, end).join("");
}

// highlight wraps the DOM range covering normalized runes [start,end) inside
// blockEl in <mark> elements (one per intersected text node). Returns the first
// mark created (for bubble alignment), or null.
export function highlight(blockEl, start, end, opts = {}) {
  const { entries } = buildCharMap(blockEl);
  // Per text node, the in-range code points are contiguous (norm is monotonic
  // in DOM order), so one [lo,hi) UTF-16 span per node suffices.
  const segs = new Map();
  const order = [];
  for (const e of entries) {
    if (e.norm >= start && e.norm < end) {
      const cur = segs.get(e.node);
      if (!cur) {
        segs.set(e.node, { lo: e.offset, hi: e.offset + e.len });
        order.push(e.node);
      } else {
        cur.hi = e.offset + e.len;
      }
    }
  }
  let first = null;
  for (const node of order) {
    const { lo, hi } = segs.get(node);
    const mark = wrapRange(node, lo, hi, opts);
    if (mark && !first) first = mark;
  }
  return first;
}

function wrapRange(node, lo, hi, opts) {
  const range = document.createRange();
  try {
    range.setStart(node, lo);
    range.setEnd(node, hi);
  } catch (e) {
    return null;
  }
  const mark = document.createElement("mark");
  mark.className = "qbl-mark" + (opts.fuzzy ? " qbl-mark-fuzzy" : "");
  if (opts.threadId) mark.dataset.thread = opts.threadId;
  if (opts.fuzzy && opts.confidence != null) {
    mark.title = "fuzzy match · " + Math.round(opts.confidence * 100) + "% confidence";
  }
  try {
    range.surroundContents(mark);
  } catch (e) {
    return null; // range straddled an element boundary; skip this segment
  }
  return mark;
}

// selectionToOffsets maps a DOM selection Range that lies within blockEl to
// normalized rune offsets plus the exact normalized quote. Returns null if the
// selection is empty after trimming whitespace.
export function selectionToOffsets(blockEl, range) {
  const { entries, text } = buildCharMap(blockEl);
  const sel = [];
  for (const e of entries) {
    let cmp;
    try {
      cmp = range.comparePoint(e.node, e.offset);
    } catch (err) {
      continue;
    }
    if (cmp === 0) sel.push(e); // point lies within [start,end] (inclusive)
  }
  // The end boundary is exclusive: drop a code point that starts exactly there.
  while (
    sel.length &&
    sel[sel.length - 1].node === range.endContainer &&
    sel[sel.length - 1].offset === range.endOffset
  ) {
    sel.pop();
  }
  // Trim leading/trailing trimmed-whitespace entries.
  let a = 0;
  let b = sel.length - 1;
  while (a <= b && sel[a].norm < 0) a++;
  while (b >= a && sel[b].norm < 0) b--;
  if (a > b) return null;
  const start = sel[a].norm;
  const end = sel[b].norm + 1;
  if (end <= start) return null;
  return { start, end, quote: sliceRunes(text, start, end) };
}

// closestBlock returns the nearest ancestor (or self) carrying [data-qbl].
export function closestBlock(node) {
  let el = node.nodeType === Node.TEXT_NODE ? node.parentElement : node;
  while (el && !(el.dataset && el.dataset.qbl != null)) {
    el = el.parentElement;
  }
  return el;
}
