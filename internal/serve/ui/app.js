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
  selected: null,    // pinned diagram node (click); hover is ignored while set
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
      class: "edge" + (e.count === 0 ? " static" : "") + (e.ask ? " ask" : ""),
      "stroke-width": e.count === 0 ? 1 : Math.min(10, 1.5 + Math.log2(e.count + 1) * 1.6),
    });
    p.append(svg("title", {}, e.count + " link(s)" + (e.ask ? " (asks)" : "")));
    p.dataset.from = e.from;
    p.dataset.to = e.to;
    edges.append(p);
  }
  root.append(edges);

  cols.forEach((nodes) => nodes.forEach((n) => {
    const p = pos[n.id];
    let cls = "node kind-" + n.kind;
    if (n.stale) cls += " stale";
    if (n.ask) cls += " ask";
    if (n.kind === "target" && !n.installed) cls += " off";
    const grp = svg("g", { class: cls, transform: "translate(" + p.x + " " + p.y + ")" });
    grp.dataset.id = n.id;
    grp.append(svg("rect", { width: NODE_W, height: NODE_H, rx: 6 }));
    grp.append(svg("text", { x: 10, y: NODE_H / 2 + 4 }, clip(n.label, 30)));
    let tip = n.label;
    if (n.stale) tip += " (not in the rules file any more)";
    if (n.kind === "target" && !n.installed) tip += " (not installed on this Mac)";
    grp.append(svg("title", {}, tip));
    grp.addEventListener("mouseenter", () => { if (!S.selected) hoverNode(n.id); });
    grp.addEventListener("mouseleave", () => { if (!S.selected) clearHover(); });
    grp.addEventListener("click", () => selectNode(n.id));
    root.append(grp);
  }));
  $("no-decisions").hidden = (g.total || 0) > 0;
  const last = S.decisions[S.decisions.length - 1];
  if (last) highlightPath(last.path, false);
  if (S.selected) {
    if (nodeById(S.selected)) { applySelection(); drawNodeDetails(S.selected); } else selectNode(null);
  }
}

function nodeById(id) {
  const g = S.graph;
  if (!g) return null;
  for (const col of [g.openers || [], g.rules || [], g.targets || []]) {
    const n = col.find((x) => x.id === id);
    if (n) return n;
  }
  return null;
}

function labelOf(id) { const n = nodeById(id); return n ? n.label : id; }

// selectNode pins a node: every logged path through it is highlighted and listed with its rule and target.
// Clicking the pinned node again, the close button or Escape clears it.
function selectNode(id) {
  if (!id || S.selected === id) {
    S.selected = null;
    clearHover();
    $("diagram").querySelectorAll(".node.sel").forEach((n) => n.classList.remove("sel"));
    $("node-details").hidden = true;
    const last = S.decisions[S.decisions.length - 1];
    if (last) highlightPath(last.path, false);
    return;
  }
  S.selected = id;
  applySelection();
  drawNodeDetails(id);
}

function pathsThrough(id) { return S.decisions.filter((d) => Array.isArray(d.path) && d.path.length === 3 && d.path.includes(id)); }

function applySelection() {
  const id = S.selected, root = $("diagram");
  clearHover();
  root.querySelectorAll(".flow").forEach((e) => e.classList.remove("flow"));
  root.querySelectorAll(".node.last").forEach((n) => n.classList.remove("last"));
  const through = pathsThrough(id);
  if (through.length === 0) { hoverNode(id); } else {
    root.classList.add("dim");
    const near = new Set(), pairs = new Set();
    for (const d of through) {
      d.path.forEach((x) => near.add(x));
      pairs.add(d.path[0] + ">" + d.path[1]);
      pairs.add(d.path[1] + ">" + d.path[2]);
    }
    root.querySelectorAll(".edge").forEach((e) => { if (pairs.has(e.dataset.from + ">" + e.dataset.to)) e.classList.add("hl"); });
    root.querySelectorAll(".node").forEach((n) => { if (near.has(n.dataset.id)) n.classList.add("hl"); });
  }
  root.querySelectorAll(".node.sel").forEach((n) => n.classList.remove("sel"));
  root.querySelectorAll(".node").forEach((n) => { if (n.dataset.id === id) n.classList.add("sel"); });
}

// drawNodeDetails lists, for the pinned node, the other two steps of every path through it with counts.
function drawNodeDetails(id) {
  const n = nodeById(id);
  if (!n) return;
  const box = $("node-details");
  box.hidden = false;
  $("nd-title").textContent = n.label;
  const kinds = { opener: "opened by", target: "opens in" };
  $("nd-sub").textContent = kinds[n.kind] || "rule";
  const colIdx = n.kind === "opener" ? 0 : n.kind === "target" ? 2 : 1;
  const others = [0, 1, 2].filter((i) => i !== colIdx);
  const heads = ["Opened by", "Rule", "Opened in"];
  $("nd-head-row").replaceChildren(
    ...others.map((i) => el("th", { text: heads[i] })), el("th", { text: "Links" }), el("th", { text: "Last" }), el("th", {}));
  const groups = new Map();
  for (const d of pathsThrough(id)) {
    const key = others.map((i) => d.path[i]).join("\u0000");
    const g = groups.get(key) || { ids: others.map((i) => d.path[i]), count: 0, last: d };
    g.count++;
    g.last = d;
    groups.set(key, g);
  }
  const rows = [...groups.values()].sort((a, b) => b.count - a.count);
  const body = $("nd-body");
  body.replaceChildren();
  $("nd-empty").hidden = rows.length > 0;
  for (const g of rows) {
    const replay = el("button", { type: "button", class: "ghost", text: "show", title: "Show this link's path on the diagram" });
    replay.addEventListener("click", () => replayDecision(g.last));
    body.append(el("tr", {},
      ...g.ids.map((x) => el("td", { text: labelOf(x), title: x })),
      el("td", { text: String(g.count) }),
      el("td", { text: fmtTime(g.last.time) }),
      el("td", {}, replay)));
  }
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
    const replay = el("button", { type: "button", class: "ghost", text: "show", title: "Show this link's path on the diagram" });
    replay.addEventListener("click", () => replayDecision(d));
    const target = d.kind === "error" ? "ERROR: " + d.target : d.target;
    const tr = el("tr", { class: d.kind === "error" ? "err" : "" },
      el("td", { text: fmtTime(d.time) }),
      el("td", { text: d.opener === "-" ? "(none)" : d.opener }),
      el("td", { text: d.rule }),
      el("td", { text: d.ask ? target + " (" + d.ask + ")" : target, title: d.ask ? target + " (" + d.ask + ")" : target }),
      el("td", { text: d.url, title: d.url }),
      el("td", {}, replay));
    body.append(tr);
  }
}

function replayDecision(d) {
  highlightPath(d.path, true);
}

$("nd-close").addEventListener("click", () => selectNode(null));
document.addEventListener("keydown", (e) => {
  const tag = e.target && e.target.tagName;
  if (e.key === "Escape" && S.selected && tag !== "INPUT" && tag !== "TEXTAREA") selectNode(null);
});

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
  // The editor is read-only while the request is in flight: a successful Apply reloads the saved file into
  // the editor, and an edit typed meanwhile would be lost.
  editor.readOnly = true;
  $("btn-apply").disabled = true;
  let status, data;
  try {
    ({ status, data } = await api("/api/apply", { yaml: editor.value, base_hash: S.hash }));
  } finally {
    editor.readOnly = false;
  }
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
  if (data.rules) {
    let plain = 0, grouped = 0, groups = 0;
    for (const r of data.rules.rules) {
      if (r.group) { groups++; grouped += (r.rules || []).length; } else plain++;
    }
    const counts = groups ? (plain + grouped) + " rule(s): " + plain + " plain, " + grouped + " in " + groups + " group(s)" : plain + " rule(s)";
    sum.textContent = counts + ", default " + data.rules.default;
  }
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

// ---- settings ----
const SET = { data: null, timer: 0 };

// ask shows the shared confirmation dialog and resolves true on Continue; Cancel and Esc resolve false.
function ask(title, paragraphs) {
  const dlg = $("confirm");
  $("confirm-title").textContent = title;
  $("confirm-body").replaceChildren(...paragraphs.map((t) => el("p", { text: t })));
  return new Promise((resolve) => {
    dlg.returnValue = "";
    dlg.addEventListener("close", () => resolve(dlg.returnValue === "ok"), { once: true });
    dlg.showModal();
    $("confirm-cancel").focus();
  });
}
$("confirm-ok").addEventListener("click", () => $("confirm").close("ok"));
$("confirm-cancel").addEventListener("click", () => $("confirm").close("cancel"));

function setHint(id, text, isError) {
  const h = $(id);
  h.textContent = text;
  h.classList.toggle("error", !!isError);
}

function drawSettings(d) {
  SET.data = d;
  const def = d.default || {};
  $("set-default").hidden = !def.supported && !(def.schemes && def.schemes.length);
  const rows = $("default-rows");
  rows.replaceChildren();
  for (const sc of def.schemes || []) {
    const who = sc.portero ? "Portero" : (sc.handler || "no handler");
    rows.append(el("li", {},
      el("span", { class: "scheme", text: sc.scheme }),
      el("span", { class: sc.portero ? "yes" : "no", text: (sc.portero ? "\u2713 " : "") + who })));
  }
  const btn = $("btn-default");
  btn.hidden = !def.supported;
  const allOurs = (def.schemes || []).length > 0 && def.schemes.every((sc) => sc.portero);
  btn.disabled = (!!def.pending && !def.stale) || !def.app || allOurs;
  if (def.pending && def.stale) setHint("default-hint", "No answer from macOS for a while. Try again.", false);
  else if (def.pending) setHint("default-hint", "Waiting for macOS. Confirm the dialog it shows.", false);
  else if (!def.app) setHint("default-hint", "Run portero serve from Portero.app to change this.", false);
  else if (def.last && def.last.error) setHint("default-hint", def.last.error, true);
  else if (def.last) setHint("default-hint", "Done.", false);
  else setHint("default-hint", allOurs ? "Portero is the default browser." : "", false);

  const sy = d.sync || {};
  $("set-sync").hidden = !sy.supported;
  const icloud = sy.mode === "icloud";
  $("sync-mode").textContent = icloud ? "synced through iCloud Drive" : "local";
  $("sync-path").textContent = sy.rules_path || "";
  $("sync-dir").textContent = sy.icloud_dir || "";
  const sw = $("sync-switch");
  sw.checked = icloud;
  sw.disabled = !!sy.pinned || (!icloud && !sy.icloud_available);
  if (sy.pinned) setHint("sync-hint", "The rules file is fixed by PORTERO_RULES; unset it to change syncing.", false);
  else if (!icloud && !sy.icloud_available) setHint("sync-hint", "iCloud Drive is not available on this Mac.", false);
  else setHint("sync-hint", "", false);

  $("file-rules").textContent = sy.rules_path || "";
  $("file-log").textContent = d.log_file || "";
  $("file-config").textContent = d.config_file || "";

  clearTimeout(SET.timer);
  if (def.pending && !$("settings").hidden) SET.timer = setTimeout(loadSettings, 2000);
}

async function loadSettings() {
  const { status, data } = await api("/api/settings");
  if (status === 200 && data) drawSettings(data);
}

function showSettings(on) {
  $("settings").hidden = !on;
  $("routing").hidden = on;
  $("btn-settings").setAttribute("aria-expanded", String(on));
  $("btn-settings").textContent = on ? "Back" : "Settings";
  clearTimeout(SET.timer);
  if (on) loadSettings();
}
$("btn-settings").addEventListener("click", () => showSettings($("settings").hidden));

$("btn-default").addEventListener("click", async () => {
  const ok = await ask("Make Portero the default browser?", [
    "macOS will ask you to confirm. Portero becomes the browser for http and https links.",
  ]);
  if (!ok) return;
  $("btn-default").disabled = true;
  setHint("default-hint", "Asking macOS...", false);
  const { status, data } = await api("/api/settings/default", {});
  if (status === 202) setHint("default-hint", "Waiting for macOS. Confirm the dialog it shows.", false);
  else if (status !== 200) setHint("default-hint", data && data.error ? data.error.message : "Request failed (" + status + ").", true);
  await loadSettings();
});

$("sync-switch").addEventListener("change", async () => {
  const sw = $("sync-switch");
  const toICloud = sw.checked;
  const dirty = editor.value !== S.text;
  const d = SET.data && SET.data.sync ? SET.data.sync : {};
  const cloud = (d.icloud_dir || "iCloud Drive");
  let paras;
  if (toICloud && d.icloud_exists && d.icloud_offloaded) {
    paras = ["A synced rules file exists in " + cloud + " but is not downloaded to this Mac yet. It will be downloaded (up to 30 s) and used as is; the local file " +
      (d.rules_path || "") + " is kept only as rules.yaml.local-backup, replacing any earlier backup."];
  } else if (toICloud && d.icloud_exists) {
    paras = ["A synced rules file already exists in " + cloud + " and will be used as is. The local file " + (d.rules_path || "") +
      " is kept only as rules.yaml.local-backup, replacing any earlier backup."];
  } else if (toICloud) {
    paras = ["The rules file moves from " + (d.rules_path || "the local folder") + " to " + cloud + ". The local file is kept as rules.yaml.local-backup."];
  } else {
    paras = ["The rules file in " + cloud + " is copied to " + (d.local_path || "the local folder") + ", replacing the file there (the current local file is kept as rules.yaml.icloud-backup), and Portero stops syncing it."];
  }
  paras.push("Current file: " + (d.rules_path || ""));
  if (dirty) paras.push("Unsaved editor changes will be discarded.");
  const ok = await ask(toICloud ? "Sync rules through iCloud Drive?" : "Stop syncing rules?", paras);
  if (!ok) { sw.checked = !toICloud; return; }
  sw.disabled = true;
  setHint("sync-hint", "Working...", false);
  const { status, data } = await api("/api/settings/sync", { mode: toICloud ? "icloud" : "off" });
  if (status === 200) {
    await loadState();
    await loadSettings();
    setHint("sync-hint", data.changed ? "Done." : "Nothing to change.", false);
  } else {
    await loadSettings();
    setHint("sync-hint", data && data.error ? data.error.message : "Request failed (" + status + ").", true);
  }
});

loadState().then(poll);
