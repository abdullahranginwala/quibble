// comments.js — the comment-layer entry module for `quibble serve`.
//
// Fetches placements from the server (it never runs anchoring locally), paints
// highlights + margin bubbles, drives the thread slide-over panel, turns text
// selections into new comments, and live-updates over SSE. Zero dependencies.

import {
  buildCharMap,
  highlight,
  selectionToOffsets,
  closestBlock,
} from "./anchor-render.js";

const slug = decodeURIComponent(location.pathname.replace(/^\/d\//, ""));
const params = new URLSearchParams(location.search);
const includeResolved = params.get("include") === "resolved";

const state = {
  docVersion: null,
  relPath: null,
  threads: [],
  panelThreadId: null, // open thread panel id, or "__new__" for a draft
  draft: null, // {blockId, start, end, quote} pending new comment
};

let article, main, bubbles;

boot();

function boot() {
  injectStyles();
  article = document.querySelector(".qbl-prose");
  main = document.querySelector(".qbl-main");
  if (!article || !main) return;
  bubbles = document.createElement("div");
  bubbles.className = "qbl-bubbles";
  main.appendChild(bubbles);

  loadComments().then(() => {
    connectSSE();
    window.addEventListener("resize", positionBubbles);
    document.addEventListener("mouseup", onSelect);
    document.addEventListener("keyup", onSelect);
  });
}

function injectStyles() {
  if (document.querySelector('link[data-qbl-ui]')) return;
  const link = document.createElement("link");
  link.rel = "stylesheet";
  link.href = "/qbl/ui.css";
  link.setAttribute("data-qbl-ui", "1");
  document.head.appendChild(link);
}

// --- data ---

async function loadComments() {
  const q = includeResolved ? "?include=resolved" : "";
  const res = await fetch(`/api/docs/${encodeURIComponent(slug)}/comments${q}`);
  if (!res.ok) return;
  const data = await res.json();
  state.docVersion = data.docVersion;
  state.relPath = data.relPath;
  state.threads = data.threads || [];
  renderAll(data);
}

function renderAll(data) {
  clearMarks();
  renderHeader(data);
  const orphans = [];
  for (const t of state.threads) {
    if (t.status === "resolved") continue;
    if (t.placement) {
      paintThread(t);
    } else {
      orphans.push(t);
    }
  }
  renderOrphans(orphans);
  positionBubbles();
  if (includeResolved) renderResolvedList();
}

function paintThread(t) {
  const p = t.placement;
  const block = article.querySelector(`[data-qbl="${cssEscape(p.blockId)}"]`);
  if (!block) return;
  const mark = highlight(block, p.start, p.end, {
    fuzzy: p.method === "fuzzy",
    confidence: p.confidence,
    threadId: t.id,
  });
  if (mark) {
    mark.addEventListener("click", (e) => {
      e.stopPropagation();
      openThread(t.id);
    });
  }
}

// --- highlights / bubbles ---

function clearMarks() {
  article.querySelectorAll("mark.qbl-mark").forEach((m) => {
    const parent = m.parentNode;
    while (m.firstChild) parent.insertBefore(m.firstChild, m);
    parent.removeChild(m);
    parent.normalize();
  });
  bubbles.innerHTML = "";
}

function positionBubbles() {
  bubbles.innerHTML = "";
  const mainRect = main.getBoundingClientRect();
  const seen = new Set();
  for (const t of state.threads) {
    if (t.status === "resolved" || !t.placement) continue;
    if (seen.has(t.id)) continue;
    seen.add(t.id);
    const mark = article.querySelector(`mark.qbl-mark[data-thread="${cssEscape(t.id)}"]`);
    if (!mark) continue;
    const r = mark.getBoundingClientRect();
    const b = document.createElement("div");
    b.className = "qbl-bubble";
    b.dataset.status = t.status;
    b.title = `${t.author}: ${(t.body || "").slice(0, 80)}`;
    b.textContent = t.status === "addressed" ? "✓" : "💬";
    b.style.top = `${r.top - mainRect.top}px`;
    b.addEventListener("click", () => openThread(t.id));
    bubbles.appendChild(b);
  }
}

// --- orphan panel ---

function renderOrphans(orphans) {
  const existing = document.querySelector(".qbl-orphans");
  if (existing) existing.remove();
  if (!orphans.length) return;
  const box = document.createElement("div");
  box.className = "qbl-orphans";
  const title = document.createElement("div");
  title.className = "qbl-orphans-title";
  title.textContent = `Unanchored comments (${orphans.length})`;
  box.appendChild(title);
  for (const t of orphans) {
    const item = document.createElement("div");
    item.className = "qbl-orphan-item";
    item.textContent = `${t.author}: ${(t.body || "").slice(0, 90) || "(no text)"}`;
    item.addEventListener("click", () => openThread(t.id));
    box.appendChild(item);
  }
  article.insertBefore(box, article.firstChild);
}

// --- header ---

function renderHeader(data) {
  const inner = document.querySelector(".qbl-header-inner");
  if (!inner) return;
  let el = inner.querySelector(".qbl-counts");
  if (!el) {
    el = document.createElement("span");
    el.className = "qbl-counts";
    inner.appendChild(el);
  }
  const link = includeResolved
    ? `<a href="${location.pathname}">← back to open</a>`
    : `<a href="${location.pathname}?include=resolved">resolved…</a>`;
  el.innerHTML =
    `<span class="qbl-badge">${data.open} open</span>` +
    `<span class="qbl-badge">${data.addressed} addressed</span>` +
    link +
    (includeResolved ? `<span class="qbl-read-only">read-only</span>` : "");
}

function renderResolvedList() {
  // In the resolved view, list resolved threads inline at the top (read-only).
  const resolved = state.threads.filter((t) => t.status === "resolved");
  const box = document.createElement("div");
  box.className = "qbl-orphans";
  const title = document.createElement("div");
  title.className = "qbl-orphans-title";
  title.textContent = `Resolved comments (${resolved.length}) — read-only`;
  box.appendChild(title);
  for (const t of resolved) {
    const item = document.createElement("div");
    item.className = "qbl-orphan-item";
    item.textContent = `${t.author}: ${(t.body || "").slice(0, 90)}`;
    item.addEventListener("click", () => openThread(t.id));
    box.appendChild(item);
  }
  article.insertBefore(box, article.firstChild);
}

// --- selection → new comment ---

function onSelect(e) {
  // A click on the float button fires document-level mouseup first; removing
  // and re-creating the button here would detach it before its click event is
  // delivered, so the panel would never open.
  if (e && e.target instanceof Element && e.target.closest(".qbl-float")) return;
  removeFloat();
  const sel = window.getSelection();
  if (!sel || sel.isCollapsed || sel.rangeCount === 0) return;
  const range = sel.getRangeAt(0);
  const startBlock = closestBlock(range.startContainer);
  const endBlock = closestBlock(range.endContainer);
  // v0.1 limitation: single-block anchors only. A cross-block selection offers
  // no comment button.
  if (!startBlock || startBlock !== endBlock) return;
  const offsets = selectionToOffsets(startBlock, range);
  if (!offsets) return;

  const btn = document.createElement("button");
  btn.className = "qbl-float";
  btn.textContent = "💬 Comment";
  const r = range.getBoundingClientRect();
  btn.style.top = `${window.scrollY + r.bottom + 6}px`;
  btn.style.left = `${window.scrollX + r.left}px`;
  // Open on pointerdown, not click: a click's own mouseup bubbles to the
  // document-level onSelect listener, and any code path that removes the
  // button there detaches it before the browser would dispatch its click —
  // the panel would silently never open (the v0.1.0 bug). pointerdown fires
  // before mouseup, so the panel is open before any selection handling runs;
  // preventDefault also keeps the text selection alive.
  const open = (e) => {
    e.preventDefault();
    e.stopPropagation();
    removeFloat();
    state.draft = { blockId: startBlock.dataset.qbl, ...offsets };
    openNewCommentPanel();
  };
  btn.addEventListener("pointerdown", open);
  btn.addEventListener("click", open); // keyboard activation fallback
  document.body.appendChild(btn);
}

function removeFloat() {
  document.querySelectorAll(".qbl-float").forEach((b) => b.remove());
}

// --- panel ---

function panelEl() {
  let p = document.getElementById("qbl-panel");
  if (!p) {
    p = document.createElement("aside");
    p.id = "qbl-panel";
    p.className = "qbl-panel";
    document.body.appendChild(p);
  }
  return p;
}

export function panelIsOpen() {
  return !!state.panelThreadId;
}

function closePanel() {
  const p = document.getElementById("qbl-panel");
  if (p) p.classList.remove("qbl-open");
  state.panelThreadId = null;
  state.draft = null;
}

function draftHasText() {
  const box = document.querySelector("#qbl-panel .qbl-reply-box");
  return !!(box && box.value.trim());
}

function openThread(id) {
  const t = state.threads.find((x) => x.id === id);
  if (!t) return;
  state.panelThreadId = id;
  const p = panelEl();
  const canReopen = t.status === "addressed" || t.status === "resolved";
  const canResolve = t.status !== "resolved";
  const replies = (t.replies || [])
    .map((r) => msgHTML(r.author, r.time, r.body))
    .join("");
  p.innerHTML = `
    <div class="qbl-panel-head">
      <span class="qbl-panel-title">Comment <span class="qbl-status-tag">${t.status}</span></span>
      <button class="qbl-panel-close" title="Close">×</button>
    </div>
    <div class="qbl-panel-body">
      <div class="qbl-quote">${escapeHTML(t.anchor.exact)}</div>
      ${msgHTML(t.author, t.created, t.body)}
      ${replies}
      <textarea class="qbl-reply-box" placeholder="Reply…"></textarea>
    </div>
    <div class="qbl-actions">
      <button class="qbl-btn qbl-btn-primary" data-act="reply">Reply</button>
      ${canResolve ? `<button class="qbl-btn" data-act="resolve">Resolve</button>` : ""}
      ${canReopen ? `<button class="qbl-btn" data-act="reopen">Reopen</button>` : ""}
    </div>`;
  wirePanel(p, id);
  p.classList.add("qbl-open");
}

function openNewCommentPanel() {
  state.panelThreadId = "__new__";
  const p = panelEl();
  p.innerHTML = `
    <div class="qbl-panel-head">
      <span class="qbl-panel-title">New comment</span>
      <button class="qbl-panel-close" title="Close">×</button>
    </div>
    <div class="qbl-panel-body">
      <div class="qbl-quote">${escapeHTML(state.draft.quote)}</div>
      <textarea class="qbl-reply-box" placeholder="Write a comment…"></textarea>
    </div>
    <div class="qbl-actions">
      <button class="qbl-btn qbl-btn-primary" data-act="create">Comment</button>
    </div>`;
  p.querySelector(".qbl-panel-close").addEventListener("click", closePanel);
  p.querySelector('[data-act="create"]').addEventListener("click", () => submitNew(p));
  p.classList.add("qbl-open");
  const box = p.querySelector(".qbl-reply-box");
  if (box) box.focus();
}

function wirePanel(p, id) {
  p.querySelector(".qbl-panel-close").addEventListener("click", closePanel);
  const box = p.querySelector(".qbl-reply-box");
  const act = (name, fn) => {
    const btn = p.querySelector(`[data-act="${name}"]`);
    if (btn) btn.addEventListener("click", fn);
  };
  act("reply", async () => {
    const body = box.value.trim();
    if (!body) return;
    await postJSON(`/api/comments/${id}/reply`, { body });
    await loadComments();
    openThread(id);
  });
  act("resolve", async () => {
    await postJSON(`/api/comments/${id}/status`, { status: "resolved" });
    closePanel();
    await loadComments();
  });
  act("reopen", async () => {
    await postJSON(`/api/comments/${id}/status`, { status: "open" });
    await loadComments();
    openThread(id);
  });
}

async function submitNew(p) {
  const box = p.querySelector(".qbl-reply-box");
  const body = box ? box.value.trim() : "";
  const d = state.draft;
  const res = await postJSON("/api/comments", {
    doc: state.relPath,
    blockId: d.blockId,
    quoteStart: d.start,
    quoteEnd: d.end,
    quote: d.quote,
    body,
    docVersion: state.docVersion,
  });
  if (res && res.status === 409) {
    alert("The document changed since you selected. Reloading.");
    closePanel();
    await loadComments();
    return;
  }
  if (!res || !res.ok) {
    const err = res ? await res.json().catch(() => ({})) : {};
    alert("Could not save comment: " + (err.error || "unknown error"));
    return;
  }
  closePanel();
  await loadComments();
}

// --- SSE ---

function connectSSE() {
  const es = new EventSource("/api/events");
  es.addEventListener("doc-changed", (e) => onServerChange("doc", e));
  es.addEventListener("comments-changed", (e) => onServerChange("comments", e));
}

function onServerChange(kind, e) {
  let data = {};
  try {
    data = JSON.parse(e.data);
  } catch (_) {}
  if (data.slug && data.slug !== slug) return;
  // Never yank content out from under an open panel or an in-progress draft.
  if (panelIsOpen() || draftHasText()) {
    showToast(kind);
    return;
  }
  if (kind === "comments") {
    loadComments();
  } else {
    // A doc edit changes the DOM; a full reload is the safe, simple path.
    location.reload();
  }
}

function showToast(kind) {
  if (document.querySelector(".qbl-toast")) return;
  const t = document.createElement("div");
  t.className = "qbl-toast";
  t.innerHTML = `<span>${kind === "doc" ? "Document" : "Comments"} updated on disk.</span>`;
  const btn = document.createElement("button");
  btn.textContent = "Reload";
  btn.addEventListener("click", () => location.reload());
  t.appendChild(btn);
  document.body.appendChild(t);
}

// --- helpers ---

async function postJSON(url, body) {
  try {
    return await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Qbl": "1" },
      body: JSON.stringify(body),
    });
  } catch (e) {
    return null;
  }
}

function msgHTML(author, time, body) {
  return (
    `<div class="qbl-msg"><div class="qbl-msg-meta">${escapeHTML(author)} · ${escapeHTML(shortTime(time))}</div>` +
    `<div class="qbl-msg-body">${escapeHTML(body || "")}</div></div>`
  );
}

function shortTime(iso) {
  const d = new Date(iso);
  return isNaN(d) ? iso : d.toLocaleString();
}

function escapeHTML(s) {
  const div = document.createElement("div");
  div.textContent = s == null ? "" : String(s);
  return div.innerHTML;
}

function cssEscape(s) {
  if (window.CSS && CSS.escape) return CSS.escape(s);
  return String(s).replace(/["\\]/g, "\\$&");
}

// buildCharMap is imported for potential debugging parity with the server; keep
// a reference so tree-shakers/linters don't flag the import as unused.
void buildCharMap;
