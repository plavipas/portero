"use strict";
// Portero UI. Everything that comes from the API is put on the page with textContent / createElement only.

const SVGNS = "http://www.w3.org/2000/svg";
const $ = (id) => document.getElementById(id);

function el(tag, props, ...kids) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(props || {})) {
    if (k === "class") e.className = v;
    else if (k === "text") e.textContent = v;
    else e.setAttribute(k, v);
  }
  for (const k of kids) e.append(k);
  return e;
}

function svg(tag, attrs, text) {
  const e = document.createElementNS(SVGNS, tag);
  for (const [k, v] of Object.entries(attrs || {})) e.setAttribute(k, String(v));
  if (text !== undefined) e.textContent = text;
  return e;
}

async function api(path, body) {
  const opt = body === undefined ? {} : {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  };
  const res = await fetch(path, opt);
  let data = null;
  try { data = await res.json(); } catch (e) { data = null; }
  return { status: res.status, data };
}

// ---- theme ----
const root = document.documentElement;
function setTheme(t) {
  root.setAttribute("data-theme", t);
  try { localStorage.setItem("portero-theme", t); } catch (e) { /* private mode */ }
}
(function initTheme() {
  let t = null;
  try { t = localStorage.getItem("portero-theme"); } catch (e) { t = null; }
  if (t !== "dark" && t !== "light") {
    t = window.matchMedia && window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
  }
  root.setAttribute("data-theme", t);
  $("theme").addEventListener("click", () => setTheme(root.getAttribute("data-theme") === "dark" ? "light" : "dark"));
})();

// ---- state ----
const S = {
  hash: "",          // hash of the file the editor started from
  text: "",          // its text
  logOffset: 0,
  decisions: [],
  graph: null,
  valid: false,      // last validation of the editor text: no error
  stale: false,
  checking: 0,
};

// ---- diagram ----
const COL_X = [8, 318, 628], NODE_W = 212, NODE_H = 30, ROW = 42, TOP = 34, VIEW_W = 848;
const COL_TITLES = ["Opened by", "Rule", "Opens in"];

function clip(s, n) { return s.length > n ? s.slice(0, n - 1) + "…" : s; }

function drawGraph(g) {
  S.graph = g;
  const root = $("diagram");
  root.replaceChildren();
  const cols = [g.openers || [], g.rules || [], g.targets || []];
  const rows = Math.max(1, ...cols.map((c) => c.length));
  const height = TOP + rows * ROW;
  root.setAttribute("viewBox", "0 0 " + VIEW_W + " " + height);
  const pos = {};
  cols.forEach((nodes, ci) => nodes.forEach((n, i) => { pos[n.id] = { x: COL_X[ci], y: TOP + i * ROW }; }));

  COL_TITLES.forEach((t, i) => root.append(svg("text", { x: COL_X[i], y: 18, class: "col-title" }, t)));

  const edges = svg("g", {});
  for (const e of g.edges || []) {
    const a = pos[e.from], b = pos[e.to];
    if (!a || !b) continue;
    const x1 = a.x + NODE_W, y1 = a.y + NODE_H / 2, x2 = b.x, y2 = b.y + NODE_H / 2, mx = (x1 + x2) / 2;
    const p = svg("path", {
      d: "M" + x1 + " " + y1 + " C" + mx + " " + y1 + " " + mx + " " + y2 + " " + x2 + " " + y2,
      class: "edge" + (e.count === 0 ? " static" : ""),
      "stroke-width": e.count === 0 ? 1 : Math.min(10, 1.5 + Math.log2(e.count + 1) * 1.6),
    });
    p.append(svg("title", {}, e.count + " link(s)"));
    p.dataset.from = e.from;
    p.dataset.to = e.to;
    edges.append(p);
  }
  root.append(edges);

  cols.forEach((nodes) => nodes.forEach((n) => {
    const p = pos[n.id];
    let cls = "node kind-" + n.kind;
    if (n.stale) cls += " stale";
    if (n.kind === "target" && !n.installed) cls += " off";
    const grp = svg("g", { class: cls, transform: "translate(" + p.x + " " + p.y + ")" });
    grp.dataset.id = n.id;
    grp.append(svg("rect", { width: NODE_W, height: NODE_H, rx: 6 }));
    grp.append(svg("text", { x: 10, y: NODE_H / 2 + 4 }, clip(n.label, 30)));
    let tip = n.label;
    if (n.stale) tip += " (not in the rules file any more)";
    if (n.kind === "target" && !n.installed) tip += " (not installed on this Mac)";
    grp.append(svg("title", {}, tip));
    grp.addEventListener("mouseenter", () => hoverNode(n.id));
    grp.addEventListener("mouseleave", clearHover);
    root.append(grp);
  }));
  $("no-decisions").hidden = (g.total || 0) > 0;
  const last = S.decisions[S.decisions.length - 1];
  if (last) highlightPath(last.path, false);
}

function hoverNode(id) {
  const root = $("diagram");
  root.classList.add("dim");
  const near = new Set([id]);
  root.querySelectorAll(".edge").forEach((e) => {
    if (e.dataset.from === id || e.dataset.to === id) {
      e.classList.add("hl");
      near.add(e.dataset.from);
      near.add(e.dataset.to);
    }
  });
  root.querySelectorAll(".node").forEach((n) => { if (near.has(n.dataset.id)) n.classList.add("hl"); });
}

function clearHover() {
  const root = $("diagram");
  root.classList.remove("dim");
  root.querySelectorAll(".hl").forEach((n) => n.classList.remove("hl"));
}

// highlightPath animates the edges of [opener, rule, target] and outlines its nodes.
function highlightPath(path, restart) {
  const root = $("diagram");
  root.querySelectorAll(".flow").forEach((e) => e.classList.remove("flow"));
  root.querySelectorAll(".node.last").forEach((n) => n.classList.remove("last"));
  if (!path) return;
  if (restart) root.getBoundingClientRect(); // reflow so the animation starts again
  const pairs = [[path[0], path[1]], [path[1], path[2]]];
  root.querySelectorAll(".edge").forEach((e) => {
    if (pairs.some((p) => p[0] === e.dataset.from && p[1] === e.dataset.to)) e.classList.add("flow");
  });
  root.querySelectorAll(".node").forEach((n) => { if (path.includes(n.dataset.id)) n.classList.add("last"); });
}

// ---- decisions ----
function fmtTime(t) { return t.replace("T", " ").replace("Z", ""); }

function drawDecisions() {
  const body = $("decisions-body");
  body.replaceChildren();
  const last = S.decisions.slice(-20).reverse();
  for (const d of last) {
    const replay = el("button", { type: "button", class: "ghost", text: "replay" });
    replay.addEventListener("click", () => replayDecision(d));
    const target = d.kind === "error" ? "ERROR: " + d.target : d.target;
    const tr = el("tr", { class: d.kind === "error" ? "err" : "" },
      el("td", { text: fmtTime(d.time) }),
      el("td", { text: d.opener === "-" ? "(none)" : d.opener }),
      el("td", { text: d.rule }),
      el("td", { text: target, title: target }),
      el("td", { text: d.url, title: d.url }),
      el("td", {}, replay));
    body.append(tr);
  }
}

function replayDecision(d) {
  $("m-url").value = d.url;
  $("m-opener").value = d.opener === "-" ? "" : d.opener;
  highlightPath(d.path, true);
}

async function poll() {
  try {
    const { status, data } = await api("/api/decisions?offset=" + S.logOffset + "&limit=200");
    if (status === 200 && data) {
      if (data.truncated) S.decisions = [];
      if (data.truncated || (data.decisions && data.decisions.length)) {
        S.decisions = S.decisions.concat(data.decisions || []).slice(-200);
        S.logOffset = data.offset;
        drawDecisions();
        const g = await api("/api/graph");
        if (g.status === 200) drawGraph(g.data);
      } else {
        S.logOffset = data.offset;
      }
    }
  } catch (e) { /* the server is gone or restarting; keep trying */ }
  setTimeout(poll, 2000);
}

// ---- editor ----
const editor = $("editor"), gutter = $("gutter");
let errLine = 0, debounce = 0;

function drawGutter() {
  const n = editor.value.split("\n").length;
  const frag = document.createDocumentFragment();
  for (let i = 1; i <= n; i++) frag.append(el("span", { class: i === errLine ? "err" : "", text: String(i) }));
  gutter.replaceChildren(frag);
  gutter.scrollTop = editor.scrollTop;
}
editor.addEventListener("scroll", () => { gutter.scrollTop = editor.scrollTop; });
editor.addEventListener("input", () => {
  errLine = 0;
  drawGutter();
  setApply(false);
  clearTimeout(debounce);
  debounce = setTimeout(validate, 400);
});

function setApply(ok) {
  const changed = editor.value !== S.text;
  $("btn-apply").disabled = !(ok && changed && S.valid && !S.stale);
  $("btn-reload").hidden = !S.stale && !changed;
  $("state-line").textContent = S.stale ? "The file changed on disk." : changed ? "Unsaved changes." : "";
}

function selectLine(n) {
  const lines = editor.value.split("\n");
  let start = 0;
  for (let i = 0; i < n - 1 && i < lines.length; i++) start += lines[i].length + 1;
  editor.focus();
  editor.setSelectionRange(start, start + (lines[n - 1] || "").length);
}

function showProblems(data) {
  const box = $("problems");
  box.replaceChildren();
  if (!data) return;
  if (data.error) {
    const msg = data.error.message + (data.error.line ? " (line " + data.error.line + ")" : "");
    const d = el("div", { class: "error", text: msg });
    if (data.error.line) d.addEventListener("click", () => selectLine(data.error.line));
    box.append(d);
  } else if (data.ok) {
    box.append(el("div", { class: "ok", text: "The rules file is valid." }));
  }
  for (const w of data.warnings || []) box.append(el("div", { class: "warn", text: w }));
  if (data.stale) box.append(el("div", { class: "error", text: "The rules file changed on disk since you loaded it. Reload before applying." }));
}

function drawDiff(lines, tooLarge) {
  const box = $("diff");
  box.replaceChildren();
  if (tooLarge) { box.append(el("div", { class: "same", text: "The file is too large to show a diff." })); return; }
  if (!lines || !lines.some((l) => l.op !== " ")) { box.append(el("div", { class: "same", text: "No changes." })); return; }
  const ctx = 2, keep = new Array(lines.length).fill(false);
  lines.forEach((l, i) => {
    if (l.op === " ") return;
    for (let j = Math.max(0, i - ctx); j <= Math.min(lines.length - 1, i + ctx); j++) keep[j] = true;
  });
  let skipped = false;
  lines.forEach((l, i) => {
    if (!keep[i]) { if (!skipped) box.append(el("div", { class: "same", text: "…" })); skipped = true; return; }
    skipped = false;
    const cls = l.op === "+" ? "add" : l.op === "-" ? "del" : "same";
    box.append(el("div", { class: cls, text: l.op + " " + l.text }));
  });
}

async function validate() {
  const ticket = ++S.checking;
  const { status, data } = await api("/api/validate", { yaml: editor.value, base_hash: S.hash });
  if (ticket !== S.checking || status !== 200 || !data) return;
  S.valid = !!data.ok;
  S.stale = !!data.stale;
  errLine = data.error && data.error.line ? data.error.line : 0;
  drawGutter();
  showProblems(data);
  drawDiff(data.diff, data.diff_too_large);
  setApply(true);
  return data;
}

async function apply() {
  const { status, data } = await api("/api/apply", { yaml: editor.value, base_hash: S.hash });
  if (status === 200) {
    await loadState();
    const box = $("problems");
    box.replaceChildren(el("div", { class: "ok", text: "Applied." + (data.backup ? " The previous file is in " + data.backup + "." : "") }));
    for (const w of data.warnings || []) box.append(el("div", { class: "warn", text: w }));
    return;
  }
  if (status === 409) {
    S.stale = true;
    showProblems({ error: data && data.error, stale: true });
  } else {
    S.valid = false;
    showProblems({ error: (data && data.error) || { message: "Apply failed (" + status + ")" } });
  }
  setApply(true);
}

// ---- match ----
async function match() {
  const out = $("match-result");
  out.replaceChildren();
  const { status, data } = await api("/api/match", {
    yaml: editor.value, url: $("m-url").value, opener: $("m-opener").value, cwd: $("m-cwd").value,
  });
  if (status !== 200) {
    const msg = data && data.error ? data.error.message : "Match failed (" + status + ")";
    out.append(el("div", { class: "error", text: msg }));
    return;
  }
  out.append(el("div", {}, el("span", { class: "k", text: "Rule: " }), el("code", { text: data.rule }),
    el("span", { class: "k", text: "  Opens in: " }), el("code", { text: data.target })));
  out.append(el("div", { class: "k", text: data.cwd_note ? "Directory: " + data.cwd_note : "Directory: " + data.cwd_used }));
  if (data.plan) out.append(el("div", {}, el("span", { class: "k", text: "Would run: " }), el("code", { text: data.plan })));
  if (data.plan_error) out.append(el("div", { class: "warn", text: data.plan_error }));
  highlightPath(data.path, true);
}

// ---- state ----
function drawBrowsers(list) {
  const ul = $("browsers");
  ul.replaceChildren();
  for (const b of list || []) {
    let text = b.name + " (" + b.kind + ")" + (b.installed ? "" : " - not installed");
    if (b.profiles && b.profiles.length) text += ": " + b.profiles.join(", ");
    ul.append(el("li", { text }));
  }
}

async function loadState() {
  const { status, data } = await api("/api/state");
  if (status !== 200 || !data) {
    $("summary").textContent = "Cannot read the state" + (data && data.error ? ": " + data.error.message : ".");
    return;
  }
  S.hash = data.hash || "";
  S.text = data.text || "";
  S.stale = false;
  S.logOffset = data.log_offset || 0;
  S.decisions = data.decisions || [];
  editor.value = S.text;
  errLine = data.rules_error && data.rules_error.line ? data.rules_error.line : 0;
  drawGutter();
  $("rules-path").textContent = data.rules_path;
  $("rules-place").textContent = data.rules_place === "icloud" ? "iCloud Drive" : "local";
  const sum = $("summary");
  if (data.rules) sum.textContent = data.rules.rules.length + " rule(s), default " + data.rules.default;
  else if (!data.exists) sum.textContent = "There is no rules file yet. Write one and press Apply.";
  else sum.textContent = "The rules file has an error; links go to Safari until it is fixed.";
  S.valid = !!data.rules;
  showProblems(data.rules_error ? { error: data.rules_error } : { warnings: data.warnings });
  drawDiff([], false);
  drawBrowsers(data.browsers);
  drawDecisions();
  drawGraph(data.graph);
  setApply(true);
}

$("btn-validate").addEventListener("click", validate);
$("btn-apply").addEventListener("click", apply);
$("btn-match").addEventListener("click", match);
$("btn-reload").addEventListener("click", () => {
  if (editor.value !== S.text && !window.confirm("Discard your changes and reload the file from disk?")) return;
  loadState();
});

loadState().then(poll);
