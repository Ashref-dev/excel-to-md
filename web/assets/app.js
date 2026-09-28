const MAX_MB = 50;
const MAX_BYTES = MAX_MB * 1024 * 1024;
const EXTENSIONS = [".xlsx", ".xlsm", ".xls", ".csv", ".tsv"];
const PARALLEL_UPLOADS = 3;
const THEME_PREFERENCE_KEY = "theme-preference";

const $ = (id) => document.getElementById(id);

const root = document.documentElement;
const landing = $("landing");
const workspace = $("workspace");
const fileInput = $("fileInput");
const openBtn = $("openBtn");
const openLabel = openBtn.querySelector(".pill-label");
const sampleBtn = $("sampleBtn");
const statusEl = $("status");
const cell = $("cell");
const addBtn = $("addBtn");

const fileList = $("fileList");
const filesEmpty = $("filesEmpty");
const sheetFilter = $("sheetFilter");
const totalsEl = $("totals");
const crumbKind = $("crumbKind");
const crumbFile = $("crumbFile");
const sheetTitle = $("sheetTitle");
const sheetMeta = $("sheetMeta");
const sheetNotes = $("sheetNotes");
const stage = $("stage");
const view = $("view");
const layerTable = $("layerTable");
const layerMd = $("layerMd");
const beam = $("beam");
const summaryEl = $("summary");
const lens = $("lens");
const lensOptions = [...lens.querySelectorAll(".lens-opt")];
const copyAllBtn = $("copyAllBtn");
const copyAllCount = $("copyAllCount");
const downloadAllBtn = $("downloadAllBtn");
const copySheetBtn = $("copySheetBtn");
const downloadSheetBtn = $("downloadSheetBtn");
const toast = $("toast");

const darkModeMedia = window.matchMedia("(prefers-color-scheme: dark)");
const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)");
const isMac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);

const state = {
  files: [],
  active: null, // { fileId, index }
  mode: "table",
  busy: false,
};
let nextFileId = 1;

/* ───────── helpers ───────── */

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

function icon(name, extraClass = "") {
  const ns = "http://www.w3.org/2000/svg";
  const svg = document.createElementNS(ns, "svg");
  svg.setAttribute("class", `icon ${extraClass}`.trim());
  svg.setAttribute("aria-hidden", "true");
  const use = document.createElementNS(ns, "use");
  use.setAttribute("href", `#i-${name}`);
  svg.appendChild(use);
  return svg;
}

const plural = (n, word) => `${n} ${word}${n === 1 ? "" : "s"}`;
const pad2 = (n) => String(n).padStart(2, "0");
const extOf = (name) => (name.match(/\.[^.]+$/)?.[0] || "").toLowerCase();
const baseName = (name) => name.replace(/\.[^.]+$/, "");
const motionOK = () => !reducedMotion.matches;

function escapeHtml(text) {
  return text.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

function formatBytes(bytes) {
  if (bytes === 0) return "0 B";
  const index = Math.min(3, Math.floor(Math.log(bytes) / Math.log(1024)));
  return `${(bytes / 1024 ** index).toFixed(index ? 1 : 0)} ${["B", "KB", "MB", "GB"][index]}`;
}

function withTransition(update) {
  if (document.startViewTransition && motionOK()) {
    document.startViewTransition(update);
  } else {
    update();
  }
}

function showToast(message, isError = false) {
  toast.textContent = message;
  toast.classList.toggle("is-error", isError);
  toast.classList.add("show");
  clearTimeout(showToast.timer);
  showToast.timer = setTimeout(() => toast.classList.remove("show"), isError ? 4200 : 2200);
}

function setStatus(message, isError = false) {
  statusEl.textContent = message;
  statusEl.classList.toggle("is-error", isError);
}

function report(message) {
  if (workspace.hidden) setStatus(message, true);
  else showToast(message, true);
}

/* ───────── conversion ───────── */

function validate(file) {
  if (!EXTENSIONS.includes(extOf(file.name))) {
    return "only .xlsx, .xls and .csv files are supported.";
  }
  if (file.size > MAX_BYTES) {
    return `it's ${formatBytes(file.size)}; the limit is ${MAX_MB} MB.`;
  }
  return "";
}

async function request(file) {
  const body = new FormData();
  body.append("file", file);
  let response;
  try {
    response = await fetch("/api/convert", { method: "POST", body });
  } catch (_) {
    throw new Error("couldn't reach the server; check your connection and try again.");
  }
  const payload = await response.json().catch(() => ({}));
  if (!response.ok || !payload.ok) {
    throw new Error(payload.error || "conversion failed");
  }
  if (!payload.sheets.length) {
    throw new Error("no visible sheets to convert");
  }
  return payload;
}

async function mapLimit(items, limit, worker) {
  const results = new Array(items.length);
  let cursor = 0;
  const run = async () => {
    while (cursor < items.length) {
      const index = cursor++;
      results[index] = await worker(items[index]);
    }
  };
  await Promise.all(Array.from({ length: Math.min(limit, items.length) }, run));
  return results;
}

function setBusy(busy, done = 0, total = 0) {
  state.busy = busy;
  [openBtn, sampleBtn, addBtn].forEach((button) => (button.disabled = busy));
  openBtn.classList.toggle("is-busy", busy);
  addBtn.classList.toggle("is-busy", busy);
  openBtn.setAttribute("aria-busy", String(busy));
  cell.classList.toggle("is-copying", busy);
  openLabel.textContent = !busy ? "Open files" : total > 1 ? `Converting ${done}/${total}…` : "Converting…";
}

function upsertFile(file, payload) {
  const kind = payload.meta?.format || extOf(file.name).slice(1);
  const entry = { id: nextFileId++, name: file.name, size: file.size, kind, sheets: payload.sheets };
  const existing = state.files.findIndex((f) => f.name === file.name && f.size === file.size);
  if (existing >= 0) {
    state.files[existing] = entry;
  } else {
    state.files.push(entry);
  }
  return entry;
}

async function addFiles(list) {
  const files = [...(list || [])];
  if (!files.length || state.busy) return;

  const wasOpen = !workspace.hidden;
  const problems = [];
  const valid = files.filter((file) => {
    const message = validate(file);
    if (message) problems.push({ name: file.name, message });
    return !message;
  });

  let added = [];
  if (valid.length) {
    let done = 0;
    setStatus("");
    setBusy(true, 0, valid.length);
    const results = await mapLimit(valid, PARALLEL_UPLOADS, async (file) => {
      try {
        return { file, payload: await request(file) };
      } catch (error) {
        problems.push({ name: file.name, message: error.message });
        return null;
      } finally {
        done += 1;
        setBusy(true, done, valid.length);
      }
    });
    added = results.filter(Boolean).map(({ file, payload }) => upsertFile(file, payload));
    setBusy(false);
  }

  const problemText = describeProblems(problems);
  if (!added.length) {
    if (problemText) report(problemText);
    return;
  }

  const first = added[0];
  state.active = { fileId: first.id, index: firstReadableSheet(first) };
  sheetFilter.value = "";

  if (workspace.hidden) {
    withTransition(() => {
      landing.hidden = true;
      workspace.hidden = false;
      renderWorkspace(false);
      placeLensThumb(false);
    });
  } else {
    renderWorkspace(true);
  }

  if (problemText) {
    showToast(problemText, true);
  } else if (wasOpen) {
    const sheets = added.reduce((n, f) => n + f.sheets.length, 0);
    showToast(`Added ${plural(added.length, "file")} · ${plural(sheets, "sheet")}`);
  }
}

function describeProblems(problems) {
  if (!problems.length) return "";
  if (problems.length === 1) return `${problems[0].name}: ${problems[0].message}`;
  return `${problems.length} files couldn't be converted: ${problems.map((p) => p.name).join(", ")}`;
}

function firstReadableSheet(file) {
  return Math.max(0, file.sheets.findIndex((sheet) => !sheet.error));
}

async function openSamples() {
  if (state.busy) return;
  try {
    const names = ["sample-sales.xlsx", "sample-support.csv"];
    const files = await Promise.all(
      names.map(async (name) => {
        const response = await fetch(`/assets/${name}`);
        if (!response.ok) throw new Error("The sample files are unavailable right now.");
        return new File([await response.blob()], name);
      }),
    );
    await addFiles(files);
  } catch (error) {
    report(error.message);
  }
}

function openPicker() {
  if (!state.busy) fileInput.click();
}

function removeFile(id) {
  const index = state.files.findIndex((f) => f.id === id);
  if (index < 0) return;
  const [removed] = state.files.splice(index, 1);

  if (!state.files.length) {
    state.active = null;
    setStatus("");
    withTransition(() => {
      workspace.hidden = true;
      landing.hidden = false;
      fileList.replaceChildren();
      openBtn.focus({ preventScroll: true });
    });
    return;
  }

  if (state.active.fileId === id) {
    const neighbour = state.files[Math.min(index, state.files.length - 1)];
    state.active = { fileId: neighbour.id, index: firstReadableSheet(neighbour) };
  }
  renderWorkspace(true);
  fileList.querySelector(".sheet-item.is-active")?.focus({ preventScroll: true });
  showToast(`Closed ${removed.name}`);
}

/* ───────── workspace ───────── */

const allSheets = () => state.files.flatMap((file) => file.sheets.map((sheet) => ({ file, sheet })));
const activeFile = () => state.files.find((f) => f.id === state.active?.fileId);
const activeSheet = () => activeFile()?.sheets[state.active.index];

function renderWorkspace(animate) {
  const sheets = allSheets();
  const flagged = sheets.filter(({ sheet }) => sheet.error || sheet.warnings?.length).length;

  totalsEl.textContent = `${plural(state.files.length, "file")} · ${plural(sheets.length, "sheet")}`;
  summaryEl.textContent =
    `${plural(sheets.length, "sheet")} from ${plural(state.files.length, "file")}` +
    (flagged ? ` · ${flagged} with notes` : "");
  copyAllCount.textContent = String(sheets.length);
  copyAllBtn.setAttribute("aria-label", `Copy all ${plural(sheets.length, "sheet")} as Markdown`);

  renderFileList();
  applyFilter(sheetFilter.value);
  selectSheet(state.active.fileId, state.active.index, animate);
}

function renderFileList() {
  fileList.replaceChildren();

  state.files.forEach((file) => {
    const group = el("li", "file-group");
    group.dataset.file = String(file.id);
    group.setAttribute("aria-label", file.name);

    const row = el("div", "file-row");
    const close = el("button", "file-x");
    close.type = "button";
    close.setAttribute("aria-label", `Close ${file.name}`);
    close.title = "Close file";
    close.appendChild(icon("x"));
    close.addEventListener("click", () => removeFile(file.id));
    row.append(el("span", "kind", file.kind), el("span", "file-name", file.name), close);
    row.querySelector(".file-name").title = file.name;

    const list = el("ul", "sheet-list");
    list.setAttribute("role", "list");
    file.sheets.forEach((sheet, index) => {
      const li = el("li");
      li.dataset.name = `${sheet.name} ${file.name}`.toLowerCase();

      const button = el("button", "sheet-item");
      button.type = "button";
      button.dataset.file = String(file.id);
      button.dataset.index = String(index);

      const name = el("span", "name");
      name.appendChild(el("span", "", sheet.name || "Untitled"));
      if (sheet.error) name.appendChild(icon("warn", "is-fail"));
      else if (sheet.warnings?.length) name.appendChild(icon("warn"));

      button.append(
        el("span", "idx", pad2(index + 1)),
        name,
        el("span", "dims", `${sheet.row_count || 0}×${sheet.col_count || 0}`),
      );
      button.addEventListener("click", () => selectSheet(file.id, index));
      li.appendChild(button);
      list.appendChild(li);
    });

    group.append(row, list);
    fileList.appendChild(group);
  });
}

function selectSheet(fileId, index, animate = true) {
  const file = state.files.find((f) => f.id === fileId);
  const sheet = file?.sheets[index];
  if (!sheet) return;
  state.active = { fileId, index };

  fileList.querySelectorAll(".sheet-item").forEach((button) => {
    const on = Number(button.dataset.file) === fileId && Number(button.dataset.index) === index;
    button.classList.toggle("is-active", on);
    if (on) {
      button.setAttribute("aria-current", "true");
      button.scrollIntoView({ block: "nearest", inline: "nearest" });
    } else {
      button.removeAttribute("aria-current");
    }
  });

  crumbKind.textContent = file.kind;
  crumbFile.textContent = file.name;
  sheetTitle.textContent = sheet.name || "Untitled";
  sheetTitle.title = sheet.name || "Untitled";
  sheetMeta.textContent = `${plural(sheet.row_count || 0, "row")} · ${plural(sheet.col_count || 0, "column")}`;

  sheetNotes.replaceChildren();
  (sheet.warnings || []).forEach((text) => {
    const li = el("li");
    li.append(icon("warn"), el("span", "", text));
    sheetNotes.appendChild(li);
  });
  if (sheet.error) {
    const li = el("li", "is-fail");
    li.append(icon("warn"), el("span", "", sheet.error));
    sheetNotes.appendChild(li);
  }
  sheetNotes.hidden = !sheetNotes.children.length;

  const hasMarkdown = Boolean(sheet.markdown);
  copySheetBtn.disabled = !hasMarkdown;
  downloadSheetBtn.disabled = !hasMarkdown;

  renderLayers(sheet);
  if (animate && motionOK()) {
    view.classList.remove("is-entering");
    void view.offsetWidth;
    view.classList.add("is-entering");
  }
}

function highlightMarkdown(markdown) {
  return markdown
    .split("\n")
    .map((line) => {
      const escaped = escapeHtml(line);
      if (/^\s*\|?(\s*:?-{3,}:?\s*\|)+\s*$/.test(line)) {
        return `<span class="rule-row">${escaped}</span>`;
      }
      return escaped.replace(/(^|[^\\])\|/g, '$1<span class="pipe">|</span>');
    })
    .join("\n");
}

function renderLayers(sheet) {
  view.scrollTop = 0;
  view.scrollLeft = 0;
  if (sheet.error) {
    layerTable.innerHTML = layerMd.innerHTML = "<p>This sheet couldn’t be converted.</p>";
  } else {
    layerTable.innerHTML = DOMPurify.sanitize(marked.parse(sheet.markdown || ""));
    layerMd.innerHTML = `<pre><code>${highlightMarkdown(sheet.markdown || "")}</code></pre>`;
  }
  showLayer(state.mode);
}

function showLayer(mode) {
  layerTable.hidden = mode !== "table";
  layerMd.hidden = mode !== "markdown";
}

/* ───────── the lens: Table ⇄ Markdown ───────── */

function placeLensThumb(animate = true) {
  const option = lensOptions.find((o) => o.dataset.view === state.mode);
  if (!option || !option.offsetWidth) return;
  const thumb = lens.querySelector(".lens-thumb");
  if (!animate) thumb.style.transition = "none";
  lens.style.setProperty("--x", `${option.offsetLeft}px`);
  lens.style.setProperty("--w", `${option.offsetWidth}px`);
  if (!animate) {
    void thumb.offsetWidth;
    thumb.style.transition = "";
  }
}

let morphing = null;

function setMode(mode) {
  if (mode === state.mode) return;
  state.mode = mode;
  lensOptions.forEach((option) => option.setAttribute("aria-pressed", String(option.dataset.view === mode)));
  placeLensThumb();
  if (!state.active) return;

  const incoming = mode === "table" ? layerTable : layerMd;
  const outgoing = mode === "table" ? layerMd : layerTable;
  morphing?.cancel();
  view.scrollTop = 0;
  view.scrollLeft = 0;

  if (!motionOK()) {
    showLayer(mode);
    return;
  }

  // A scan line sweeps down the panel, rewriting the sheet in the new form as it passes.
  outgoing.hidden = false;
  incoming.hidden = false;
  outgoing.style.zIndex = "1";
  incoming.style.zIndex = "2";
  const height = incoming.offsetHeight;
  const reach = Math.min(view.clientHeight, height);
  const timing = { duration: 560, easing: "cubic-bezier(0.65, 0, 0.35, 1)" };

  const sweep = incoming.animate(
    { clipPath: [`inset(0 0 ${height}px 0)`, `inset(0 0 ${height - reach}px 0)`] },
    timing,
  );
  morphing = sweep;
  beam.animate(
    { transform: ["translateY(0)", `translateY(${reach}px)`], opacity: [1, 1, 0], offset: [0, 0.85, 1] },
    timing,
  );

  const settle = () => {
    if (morphing !== sweep) return;
    showLayer(state.mode);
    outgoing.style.zIndex = incoming.style.zIndex = "";
    morphing = null;
  };
  sweep.finished.then(settle, settle);
}

/* ───────── filter + keyboard navigation ───────── */

function applyFilter(value) {
  const term = value.trim().toLowerCase();
  let visible = 0;
  fileList.querySelectorAll(".file-group").forEach((group) => {
    let groupVisible = 0;
    group.querySelectorAll("li[data-name]").forEach((li) => {
      const match = !term || li.dataset.name.includes(term);
      li.hidden = !match;
      if (match) groupVisible += 1;
    });
    group.hidden = groupVisible === 0;
    visible += groupVisible;
  });
  filesEmpty.hidden = visible > 0;
}

function visibleSheetButtons() {
  return [...fileList.querySelectorAll(".file-group:not([hidden]) li:not([hidden]) .sheet-item")];
}

function stepSheet(delta) {
  const buttons = visibleSheetButtons();
  if (!buttons.length) return;
  const position = buttons.findIndex((b) => b.classList.contains("is-active"));
  const next = buttons[Math.min(buttons.length - 1, Math.max(0, position + delta))];
  selectSheet(Number(next.dataset.file), Number(next.dataset.index));
}

/* ───────── copy / download ───────── */

function combinedMarkdown() {
  return (
    state.files
      .map((file) => {
        const sheets = file.sheets.map((sheet) => {
          const body = sheet.error ? `> Could not convert: ${sheet.error}` : sheet.markdown;
          return `## ${sheet.name}\n\n${body}`;
        });
        return `# ${file.name}\n\n${sheets.join("\n\n")}`;
      })
      .join("\n\n") + "\n"
  );
}

async function writeClipboard(text) {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch (_) {
    const area = document.createElement("textarea");
    area.value = text;
    area.setAttribute("readonly", "");
    area.style.position = "fixed";
    area.style.opacity = "0";
    document.body.appendChild(area);
    area.select();
    const copied = document.execCommand("copy");
    area.remove();
    return copied;
  }
}

const COPY_FAILED = "Couldn't copy. Use Download instead, or allow clipboard access.";

function flashDone(button, label) {
  const text = button.querySelector(".act-label, .pill-label");
  const original = button.dataset.label || text?.textContent;
  button.dataset.label = original;
  button.classList.add("is-done");
  if (text && label) text.textContent = label;
  clearTimeout(button.doneTimer);
  button.doneTimer = setTimeout(() => {
    button.classList.remove("is-done");
    if (text) text.textContent = original;
  }, 1600);
}

function marchAnts() {
  if (!motionOK()) return;
  stage.classList.add("is-copied");
  clearTimeout(marchAnts.timer);
  marchAnts.timer = setTimeout(() => stage.classList.remove("is-copied"), 1600);
}

async function copySheet() {
  const sheet = activeSheet();
  if (!sheet?.markdown) return;
  if (!(await writeClipboard(sheet.markdown))) {
    showToast(COPY_FAILED, true);
    return;
  }
  flashDone(copySheetBtn, "Copied");
  marchAnts();
  showToast(`Copied “${sheet.name}”`);
}

async function copyAll() {
  if (!state.files.length) return;
  if (!(await writeClipboard(combinedMarkdown()))) {
    showToast(COPY_FAILED, true);
    return;
  }
  flashDone(copyAllBtn, "Copied");
  marchAnts();
  const count = allSheets().length;
  showToast(`Copied ${plural(count, "sheet")} from ${plural(state.files.length, "file")}`);
}

function downloadText(filename, text) {
  const url = URL.createObjectURL(new Blob([text], { type: "text/markdown;charset=utf-8" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
  showToast(`Saved ${filename}`);
}

copySheetBtn.addEventListener("click", copySheet);
copyAllBtn.addEventListener("click", copyAll);
downloadSheetBtn.addEventListener("click", () => {
  const sheet = activeSheet();
  if (sheet?.markdown) downloadText(`${sheet.name || "sheet"}.md`, sheet.markdown);
});
downloadAllBtn.addEventListener("click", () => {
  const name = state.files.length === 1 ? `${baseName(state.files[0].name)}.md` : "excel-tables.md";
  downloadText(name, combinedMarkdown());
});

/* ───────── drag anywhere ───────── */

function initDrop() {
  let depth = 0;
  const hasFiles = (event) => [...(event.dataTransfer?.types || [])].includes("Files");
  const end = () => {
    depth = 0;
    document.body.classList.remove("is-dragging");
    if (!state.busy) cell.classList.remove("is-copying");
  };

  window.addEventListener("dragenter", (event) => {
    if (!hasFiles(event)) return;
    event.preventDefault();
    depth += 1;
    document.body.classList.add("is-dragging");
    cell.classList.add("is-copying");
  });
  window.addEventListener("dragover", (event) => {
    if (hasFiles(event)) event.preventDefault();
  });
  window.addEventListener("dragleave", (event) => {
    if (!hasFiles(event)) return;
    depth -= 1;
    if (depth <= 0) end();
  });
  window.addEventListener("drop", (event) => {
    if (!hasFiles(event)) return;
    event.preventDefault();
    end();
    addFiles(event.dataTransfer.files);
  });
}

/* ───────── keyboard ───────── */

function initKeys() {
  $("openKbd").textContent = isMac ? "⌘ O" : "Ctrl O";
  addBtn.title = `Add files (${isMac ? "⌘O" : "Ctrl+O"})`;

  document.addEventListener("keydown", (event) => {
    const mod = isMac ? event.metaKey : event.ctrlKey;
    if (mod && event.key.toLowerCase() === "o") {
      event.preventDefault();
      openPicker();
      return;
    }
    if (workspace.hidden || event.metaKey || event.ctrlKey || event.altKey) return;

    if (event.target === sheetFilter) {
      if (event.key === "Escape") {
        sheetFilter.value = "";
        applyFilter("");
        sheetFilter.blur();
      } else if (event.key === "Enter") {
        const first = visibleSheetButtons()[0];
        if (first) selectSheet(Number(first.dataset.file), Number(first.dataset.index));
        sheetFilter.blur();
      }
      return;
    }
    if (event.target instanceof HTMLInputElement) return;

    const key = event.key.toLowerCase();
    if (event.key === "/") {
      event.preventDefault();
      sheetFilter.focus();
    } else if (event.key === "ArrowDown" || key === "j") {
      event.preventDefault();
      stepSheet(1);
    } else if (event.key === "ArrowUp" || key === "k") {
      event.preventDefault();
      stepSheet(-1);
    } else if (key === "m") {
      setMode(state.mode === "table" ? "markdown" : "table");
    } else if (key === "c") {
      copySheet();
    }
  });
}

/* ───────── theme: the new theme grows out of the toggle ───────── */

function readThemePreference() {
  try {
    const saved = window.localStorage.getItem(THEME_PREFERENCE_KEY);
    if (saved === "light" || saved === "dark") return saved;
  } catch (_) {
    /* storage blocked: follow the system */
  }
  return darkModeMedia.matches ? "dark" : "light";
}

function saveThemePreference(theme) {
  try {
    window.localStorage.setItem(THEME_PREFERENCE_KEY, theme);
  } catch (_) {
    /* storage blocked: the choice lasts for this page only */
  }
}

function applyTheme(theme) {
  root.setAttribute("data-theme", theme);
  const next = theme === "dark" ? "light" : "dark";
  document.querySelectorAll("[data-theme-toggle]").forEach((button) => {
    button.querySelector("use").setAttribute("href", theme === "dark" ? "#i-moon" : "#i-sun");
    button.setAttribute("aria-label", `Switch to ${next} theme`);
    button.title = `Switch to ${next} theme`;
  });
  document.querySelectorAll('meta[name="theme-color"]').forEach((meta) => {
    meta.setAttribute("content", theme === "dark" ? "#101010" : "#f4f3ef");
  });
}

function toggleTheme(event) {
  const next = root.getAttribute("data-theme") === "dark" ? "light" : "dark";
  saveThemePreference(next);

  if (!document.startViewTransition || !motionOK()) {
    applyTheme(next);
    return;
  }

  const rect = event.currentTarget.getBoundingClientRect();
  const x = rect.left + rect.width / 2;
  const y = rect.top + rect.height / 2;
  const radius = Math.hypot(Math.max(x, innerWidth - x), Math.max(y, innerHeight - y));

  root.classList.add("theme-shift");
  const transition = document.startViewTransition(() => applyTheme(next));
  transition.ready.then(() => {
    root.animate(
      { clipPath: [`circle(0px at ${x}px ${y}px)`, `circle(${radius}px at ${x}px ${y}px)`] },
      { duration: 720, easing: "cubic-bezier(0.7, 0, 0.2, 1)", pseudoElement: "::view-transition-new(root)" },
    );
  });
  transition.finished.finally(() => root.classList.remove("theme-shift"));
}

function initTheme() {
  applyTheme(readThemePreference());
  document.querySelectorAll("[data-theme-toggle]").forEach((button) => button.addEventListener("click", toggleTheme));

  const onSystemChange = () => {
    let saved = null;
    try {
      saved = window.localStorage.getItem(THEME_PREFERENCE_KEY);
    } catch (_) {
      /* storage blocked */
    }
    if (!saved) applyTheme(darkModeMedia.matches ? "dark" : "light");
  };
  darkModeMedia.addEventListener?.("change", onSystemChange);
}

/* ───────── wire up ───────── */

openBtn.addEventListener("click", openPicker);
addBtn.addEventListener("click", openPicker);
sampleBtn.addEventListener("click", openSamples);
fileInput.addEventListener("change", () => {
  const files = [...fileInput.files];
  fileInput.value = "";
  addFiles(files);
});
sheetFilter.addEventListener("input", () => applyFilter(sheetFilter.value));
lensOptions.forEach((option) => option.addEventListener("click", () => setMode(option.dataset.view)));
window.addEventListener("resize", () => placeLensThumb(false));
document.fonts?.ready.then(() => placeLensThumb(false));

initTheme();
initDrop();
initKeys();
