const view = document.getElementById("view");
const shellStatus = document.getElementById("shell-status");
const navLinks = Array.from(document.querySelectorAll("[data-nav]"));

let eventsSource = null;
let terminalSource = null;
let runRefreshTimer = null;
let terminalPollTimer = null;
let terminalBuffer = "";
let autoScrollTerminal = true;
let terminalLastEventAt = 0;
let activeRunId = "";

const routes = {
  "/ui": renderOverview,
  "/ui/": renderOverview,
  "/ui/studio": renderStudio,
  "/ui/overview": renderOverview,
};

const packTemplates = [
  {
    id: "feature-slice",
    name: "Feature Slice",
    description: "Discover, implement, validate, and finalize one clear feature slice.",
    packID: "feature-slice",
    packName: "Feature Slice",
    goal: "Implement one scoped behavior change with deterministic validation and clear operator visibility.",
    tasks: [
      {
        id: "discover-context",
        title: "Discover Context",
        summary: "Inspect the relevant files and confirm the smallest safe implementation path.",
        allowed_files: "README.md",
        validation: "go test ./...",
        depends_on: "",
      },
      {
        id: "implement-core",
        title: "Implement Core",
        summary: "Apply the scoped behavior change without widening the task.",
        allowed_files: "README.md",
        validation: "go test ./...",
        depends_on: "discover-context",
      },
      {
        id: "validate-and-finish",
        title: "Validate and Finish",
        summary: "Run the intended validation and capture any follow-up notes.",
        allowed_files: "README.md",
        validation: "go test ./...",
        depends_on: "implement-core",
      },
    ],
  },
  {
    id: "ux-polish",
    name: "UX Polish",
    description: "Structure work for a UI tune-up with clear before-and-after verification.",
    packID: "ux-polish",
    packName: "UX Polish",
    goal: "Improve the target interface, keep the feedback loop tight, and prove the rendered result.",
    tasks: [
      {
        id: "inspect-flow",
        title: "Inspect Flow",
        summary: "Review the current UI, identify rough edges, and confirm the target experience.",
        allowed_files: "internal/httpapi",
        validation: "go test ./internal/httpapi",
        depends_on: "",
      },
      {
        id: "ship-ui-update",
        title: "Ship UI Update",
        summary: "Implement the refreshed layout, hierarchy, and state handling.",
        allowed_files: "internal/httpapi/ui",
        validation: "go test ./...",
        depends_on: "inspect-flow",
      },
      {
        id: "verify-rendered-result",
        title: "Verify Rendered Result",
        summary: "Check the interaction path, responsive layout, and any remaining visual drift.",
        allowed_files: "internal/httpapi/ui",
        validation: "go test ./...",
        depends_on: "ship-ui-update",
      },
    ],
  },
  {
    id: "bugfix-hotpath",
    name: "Bugfix Hot Path",
    description: "A fast path for a focused fix with one validation step.",
    packID: "bugfix-hotpath",
    packName: "Bugfix Hot Path",
    goal: "Resolve the critical bug, verify the fix, and leave a clean audit trail.",
    tasks: [
      {
        id: "reproduce-bug",
        title: "Reproduce Bug",
        summary: "Confirm the broken behavior and isolate the likely file scope.",
        allowed_files: "internal",
        validation: "go test ./...",
        depends_on: "",
      },
      {
        id: "fix-bug",
        title: "Fix Bug",
        summary: "Implement the focused fix without widening scope.",
        allowed_files: "internal",
        validation: "go test ./...",
        depends_on: "reproduce-bug",
      },
    ],
  },
];

async function boot() {
  try {
    setShellStatus("loading", "Loading workspace state");
    highlightNav();
    const runId = currentRunId();
    if (runId) {
      await renderRun(runId);
      return;
    }
    const handler = routes[window.location.pathname] || renderOverview;
    await handler();
  } catch (error) {
    renderFatal(error);
    setShellStatus("issue", "Surface needs attention");
  }
}

function currentRunId() {
  const match = window.location.pathname.match(/\/ui\/runs\/([^/]+)/);
  return match ? decodeURIComponent(match[1]) : "";
}

function cleanupLiveSources() {
  activeRunId = "";
  if (eventsSource) {
    eventsSource.close();
    eventsSource = null;
  }
  if (terminalSource) {
    terminalSource.close();
    terminalSource = null;
  }
  if (runRefreshTimer) {
    clearInterval(runRefreshTimer);
    runRefreshTimer = null;
  }
  if (terminalPollTimer) {
    clearInterval(terminalPollTimer);
    terminalPollTimer = null;
  }
  terminalLastEventAt = 0;
  terminalBuffer = "";
}

function highlightNav() {
  const path = window.location.pathname;
  navLinks.forEach(link => {
    const kind = link.dataset.nav;
    const active =
      (kind === "studio" && path.startsWith("/ui/studio")) ||
      (kind === "overview" && (path === "/ui" || path === "/ui/" || path.startsWith("/ui/overview"))) ||
      (kind === "runs" && path.startsWith("/ui/runs/"));
    link.classList.toggle("is-active", active);
  });
}

function setShellStatus(mode, text) {
  shellStatus.classList.remove("is-ready", "is-issue");
  if (mode === "ready") {
    shellStatus.classList.add("is-ready");
  }
  if (mode === "issue") {
    shellStatus.classList.add("is-issue");
  }
  shellStatus.lastElementChild.textContent = text;
}

function setRunsNavTarget(href) {
  const runsLink = document.querySelector('[data-nav="runs"]');
  if (runsLink) {
    runsLink.href = href;
  }
}

async function getJSON(url, options) {
  const response = await fetch(url, options);
  if (!response.ok) {
    let message = response.statusText;
    try {
      const payload = await response.json();
      message = payload.error || payload.message || message;
    } catch {
      const text = await response.text().catch(() => "");
      if (text) {
        message = text;
      }
    }
    throw new Error(message);
  }
  return response.json();
}

function safeArray(value) {
  return Array.isArray(value) ? value : [];
}

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function slugify(value) {
  return String(value || "")
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

function statusLabel(status) {
  return String(status || "unknown").replaceAll("_", " ");
}

function statusBadge(status, fallback = "unknown") {
  const resolved = status || fallback;
  return `<span class="badge ${escapeHTML(resolved)}">${escapeHTML(statusLabel(resolved))}</span>`;
}

function metricCard(label, value, detail, compact = false) {
  return `
    <article class="card">
      <p class="metric-label">${escapeHTML(label)}</p>
      <p class="metric-value ${compact ? "is-compact" : ""}">${escapeHTML(value)}</p>
      <p class="metric-detail">${escapeHTML(detail)}</p>
    </article>
  `;
}

function emptyState(title, body, actionHTML = "") {
  return `
    <div class="empty-state">
      <strong>${escapeHTML(title)}</strong>
      <p class="muted">${escapeHTML(body)}</p>
      ${actionHTML}
    </div>
  `;
}

function splitList(value) {
  return String(value || "")
    .split(/\n|,/)
    .map(item => item.trim())
    .filter(Boolean);
}

function formatDate(value) {
  if (!value || value.startsWith("0001-01-01")) {
    return "Not started";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}

function percentText(value) {
  const number = Number(value || 0);
  return `${Math.round(number)}%`;
}

function firstNonEmpty(...values) {
  return values.find(value => String(value || "").trim() !== "") || "";
}

function renderMessage(message, tone = "neutral") {
  return `
    <div class="status-note is-${escapeHTML(tone)}">
      <strong>${tone === "danger" ? "Needs attention" : tone === "success" ? "Ready to go" : "Operator note"}</strong>
      <p class="muted">${escapeHTML(message)}</p>
    </div>
  `;
}

function collectionSummary(items, validKey = "valid") {
  return {
    total: items.length,
    valid: items.filter(item => item[validKey]).length,
    invalid: items.filter(item => !item[validKey]).length,
  };
}

async function renderOverview() {
  cleanupLiveSources();
  highlightNav();
  const overview = await getJSON("/ui/api/overview");
  const packs = safeArray(overview.packs);
  const programs = safeArray(overview.programs);
  const runs = safeArray(overview.runs);
  const activeRun = runs.find(run => ["planning", "running", "waiting_on_agent"].includes(run.status)) || null;
  const packSummary = collectionSummary(packs);
  const programSummary = collectionSummary(programs);
  const failedRuns = runs.filter(run => run.status === "failed").length;
  setRunsNavTarget(activeRun ? `/ui/runs/${encodeURIComponent(activeRun.id)}` : runs[0] ? `/ui/runs/${encodeURIComponent(runs[0].id)}` : "/ui/overview");

  setShellStatus(
    activeRun ? "ready" : "ready",
    activeRun ? `Active run: ${activeRun.program_name}` : "Worker ready for orchestration",
  );

  const spotlight = activeRun ? `
    <div class="spotlight-main">
      <div>
        ${statusBadge(activeRun.status)}
        <h3>${escapeHTML(activeRun.program_name)}</h3>
        <p class="muted">${escapeHTML(activeRun.mode)} run • ${activeRun.completed_tasks}/${activeRun.total_tasks} tasks complete</p>
      </div>
      <a class="button button-primary" href="/ui/runs/${encodeURIComponent(activeRun.id)}">Open live run</a>
    </div>
    <div class="signal-grid">
      <div class="signal-card">
        <p class="metric-label">Current Pack</p>
        <strong>${escapeHTML(firstNonEmpty(activeRun.current_pack_id, "Waiting"))}</strong>
      </div>
      <div class="signal-card">
        <p class="metric-label">Current Task</p>
        <strong>${escapeHTML(firstNonEmpty(activeRun.current_task_id, "Planning"))}</strong>
      </div>
      <div class="signal-card">
        <p class="metric-label">Progress</p>
        <strong>${escapeHTML(percentText(activeRun.percent_complete))}</strong>
      </div>
    </div>
    <div class="progress"><span style="width:${Number(activeRun.percent_complete || 0)}%"></span></div>
  ` : emptyState(
    "No program is running right now",
    "The worker is idle. Author a pack, assemble a multi-pack program, or rerun a previous flow from the studio.",
    `<a class="button button-primary" href="/ui/studio">Open studio</a>`,
  );

  view.innerHTML = `
    <section class="hero-grid">
      <div class="section spotlight">
        <div class="section-header">
          <div class="toolbar-copy">
            <h2>Command Center</h2>
            <p class="section-title">Know what is ready, what is blocked, and what just changed.</p>
          </div>
        </div>
        ${spotlight}
      </div>
      <div class="section stack">
        <div class="section-header">
          <div class="toolbar-copy">
            <h2>Workspace Health</h2>
            <p class="section-title">Catalog quality at a glance.</p>
          </div>
        </div>
        <div class="signal-grid">
          <div class="signal-card">
            <p class="metric-label">Healthy Packs</p>
            <strong>${packSummary.valid}/${packSummary.total}</strong>
            <p class="muted">${packSummary.invalid ? `${packSummary.invalid} need repair` : "Catalog is clean"}</p>
          </div>
          <div class="signal-card">
            <p class="metric-label">Programs</p>
            <strong>${programSummary.total}</strong>
            <p class="muted">${programSummary.invalid ? `${programSummary.invalid} invalid definitions` : "No broken program definitions"}</p>
          </div>
          <div class="signal-card">
            <p class="metric-label">Recent Failures</p>
            <strong>${failedRuns}</strong>
            <p class="muted">${failedRuns ? "Review error details before re-running" : "No recent failed runs"}</p>
          </div>
        </div>
        <div class="chip-row">
          <span class="chip">Sequential orchestration</span>
          <span class="chip">Live terminal SSE</span>
          <span class="chip">Persistent run history</span>
        </div>
      </div>
    </section>

    <section class="cards">
      ${metricCard("Packs", packs.length, "Catalogued task-pack folders")}
      ${metricCard("Programs", programs.length, "Saved multi-pack definitions")}
      ${metricCard("Run History", runs.length, "Recent pack or program runs")}
      ${metricCard("Active", activeRun ? activeRun.program_name : "None", activeRun ? statusLabel(activeRun.status) : "No active execution")}
    </section>

    <section class="catalog-grid">
      <div class="section catalog-panel">
        <div class="toolbar">
          <div class="toolbar-copy">
            <h2>Recent Runs</h2>
            <p>Open the freshest execution context or spot the last failure quickly.</p>
          </div>
          <div class="toolbar-actions">
            <a class="button button-secondary" href="/ui/studio">Create pack</a>
          </div>
        </div>
        <div class="stack">
          ${runs.length ? runs.map(run => `
            <article class="list-row">
              <div class="list-row-head">
                <div>
                  ${statusBadge(run.status)}
                  <h3>${escapeHTML(run.program_name)}</h3>
                  <p class="muted">${escapeHTML(run.mode)} • ${escapeHTML(formatDate(run.created_at))}</p>
                </div>
                <a class="button button-secondary" href="/ui/runs/${encodeURIComponent(run.id)}">Open</a>
              </div>
              <div class="list-row-meta">
                <div class="progress"><span style="width:${Number(run.percent_complete || 0)}%"></span></div>
                <div class="mini-meta">
                  <span class="pill">${escapeHTML(run.completed_tasks)}/${escapeHTML(run.total_tasks)} tasks</span>
                  <span class="pill">${escapeHTML(percentText(run.percent_complete))}</span>
                </div>
                ${run.last_error ? `<div class="error-block">${escapeHTML(run.last_error)}</div>` : `<p class="muted">No recent failure recorded.</p>`}
              </div>
            </article>
          `).join("") : emptyState("No runs yet", "Once you start a pack or program, run history and progress rollups will appear here.")}
        </div>
      </div>

      <div class="section catalog-panel">
        <div class="toolbar">
          <div class="toolbar-copy">
            <h2>Catalog Signals</h2>
            <p>Healthy packs can run immediately. Broken manifests stay visible so they are easier to repair.</p>
          </div>
          <div class="toolbar-actions">
            <a class="button button-secondary" href="/ui/studio">Open studio</a>
          </div>
        </div>
        <div class="stack">
          ${packs.slice(0, 5).map(pack => `
            <article class="list-row">
              <div class="list-row-head">
                <div>
                  ${statusBadge(pack.valid ? "ready" : "invalid")}
                  <h3>${escapeHTML(pack.name)}</h3>
                  <p class="muted">${escapeHTML(pack.id)} • ${escapeHTML(pack.task_count)} tasks</p>
                </div>
                ${pack.valid ? `<button class="button button-secondary" data-run-pack="${escapeHTML(pack.id)}">Run</button>` : ""}
              </div>
              ${pack.valid ? `<p class="muted">Manifest and task files are ready for orchestration.</p>` : `<div class="error-block">${escapeHTML(safeArray(pack.validation_errors).join("; "))}</div>`}
            </article>
          `).join("")}
          ${packs.length > 5 ? `<p class="muted">Showing 5 of ${packs.length} packs. Open Studio for the full catalog.</p>` : ""}
        </div>
      </div>
    </section>
  `;

  view.querySelectorAll("[data-run-pack]").forEach(button => {
    button.addEventListener("click", () => startRun({ pack_id: button.dataset.runPack }));
  });
}

function taskRowMarkup(task = {}, index = 0) {
  const taskID = firstNonEmpty(task.id, `task-${index + 1}`);
  const taskTitle = firstNonEmpty(task.title, `Task ${index + 1}`);
  return `
    <article class="task-builder-row" data-task-index="${index}">
      <div class="task-builder-row-header">
        <div>
          <h3>${escapeHTML(taskTitle)}</h3>
          <p class="muted">Task ${index + 1} in the sequential pack flow.</p>
        </div>
        <div class="task-row-actions">
          <button class="button button-ghost" type="button" data-move-task="up">Move up</button>
          <button class="button button-ghost" type="button" data-move-task="down">Move down</button>
          <button class="button button-ghost" type="button" data-remove-task>Remove</button>
        </div>
      </div>
      <div class="field-grid">
        <label>Task ID<input data-field="id" value="${escapeHTML(taskID)}"></label>
        <label>Title<input data-field="title" value="${escapeHTML(taskTitle)}"></label>
      </div>
      <label>Summary<textarea data-field="summary">${escapeHTML(task.summary || "")}</textarea></label>
      <div class="field-grid">
        <label>Branch<input data-field="branch" value="${escapeHTML(firstNonEmpty(task.branch, `ptolemy/${slugify(taskID)}`))}"></label>
        <label>Depends On<input data-field="depends_on" value="${escapeHTML(firstNonEmpty(task.depends_on, safeArray(task.depends_on).join(", ")))}" placeholder="discover-context, implement-core"></label>
      </div>
      <div class="field-grid">
        <label>Allowed Files<textarea data-field="allowed_files">${escapeHTML(firstNonEmpty(task.allowed_files, safeArray(task.allowed_files).join("\n")))}</textarea></label>
        <label>Validation Commands<textarea data-field="validation">${escapeHTML(firstNonEmpty(task.validation, safeArray(task.validation).join("\n"), "go test ./..."))}</textarea></label>
      </div>
    </article>
  `;
}

function packRowMarkup(row = {}, index = 0, packOptions = []) {
  return `
    <article class="task-builder-row" data-pack-index="${index}">
      <div class="task-builder-row-header">
        <div>
          <h3>Program Pack ${index + 1}</h3>
          <p class="muted">Select a pack and optionally gate it on earlier packs.</p>
        </div>
        <div class="task-row-actions">
          <button class="button button-ghost" type="button" data-remove-pack-row>Remove</button>
        </div>
      </div>
      <div class="field-grid">
        <label>Pack
          <select data-field="pack_id">
            ${packOptions.map(option => `<option value="${escapeHTML(option.id)}" ${option.id === row.pack_id ? "selected" : ""}>${escapeHTML(option.name)} (${escapeHTML(option.id)})</option>`).join("")}
          </select>
        </label>
        <label>Depends On<input data-field="depends_on" value="${escapeHTML(firstNonEmpty(row.depends_on, safeArray(row.depends_on).join(", ")))}" placeholder="pack-a, pack-b"></label>
      </div>
    </article>
  `;
}

async function renderStudio() {
  cleanupLiveSources();
  highlightNav();
  const [packs, programs] = await Promise.all([
    getJSON("/ui/api/packs"),
    getJSON("/ui/api/programs"),
  ]);
  const packList = safeArray(packs);
  const programList = safeArray(programs);
  const validPacks = packList.filter(pack => pack.valid);

  setShellStatus("ready", "Studio ready for pack authoring");
  setRunsNavTarget("/ui/overview");

  view.innerHTML = `
    <section class="section">
      <div class="section-header">
        <div class="toolbar-copy">
          <h2>Studio</h2>
          <p class="section-title">Build packs with structure first, JSON only when you need it.</p>
          <p>Use a quick template, tune task rows, and keep validation visible before you ever hit run.</p>
        </div>
        <div class="chip-row">
          <span class="chip">Template driven</span>
          <span class="chip">Checklist friendly</span>
          <span class="chip">Sequential by default</span>
        </div>
      </div>
      <div class="template-grid">
        ${packTemplates.map(template => `
          <button class="template-button" type="button" data-pack-template="${escapeHTML(template.id)}">
            <strong>${escapeHTML(template.name)}</strong>
            <span>${escapeHTML(template.description)}</span>
          </button>
        `).join("")}
      </div>
    </section>

    <section class="helper-grid">
      <div class="section">
        <div class="toolbar">
          <div class="toolbar-copy">
            <h2>Create Pack</h2>
            <p>Author the pack manifest and its task sequence in one place.</p>
          </div>
          <div class="toolbar-actions">
            <button class="button button-secondary" type="button" id="reset-pack">Reset</button>
          </div>
        </div>
        <form id="pack-form" class="form-grid">
          <div class="field-grid">
            <label>Pack ID<input name="pack_id" value="pack-studio-demo"></label>
            <label>Name<input name="name" value="Pack Studio Demo"></label>
          </div>
          <label>Description<textarea name="description">A deterministic pack authored from the embedded studio.</textarea></label>
          <label>Goal<textarea name="goal">Implement the requested feature slice, validate it, and leave the run monitor with observable progress.</textarea></label>
          <p class="field-note">Pack Studio writes the runtime-compatible manifest shape supported by <span class="mono">internal/tasks/pack.go</span>.</p>
          <div class="toolbar">
            <div class="toolbar-copy">
              <h2>Task Sequence</h2>
              <p>Each row becomes one task file with checklist scaffolding.</p>
            </div>
            <div class="toolbar-actions">
              <button class="button button-secondary" type="button" id="add-task">Add task</button>
            </div>
          </div>
          <div id="task-list" class="task-list"></div>
          <div class="inline-actions">
            <button type="submit">Create Pack</button>
            <button class="button button-secondary" type="button" id="preview-pack">Preview payload</button>
          </div>
        </form>
        <div id="pack-result"></div>
      </div>

      <div class="section">
        <div class="toolbar">
          <div class="toolbar-copy">
            <h2>Create Program</h2>
            <p>Chain valid packs into one roll-up execution tree.</p>
          </div>
        </div>
        <form id="program-form" class="form-grid">
          <div class="field-grid">
            <label>Program ID<input name="program_id" value="demo-program"></label>
            <label>Name<input name="name" value="Demo Program"></label>
          </div>
          <label>Description<textarea name="description">Run one or more packs with rollup progress in the monitor.</textarea></label>
          <div class="toolbar">
            <div class="toolbar-copy">
              <h2>Program Packs</h2>
              <p>Use dependencies only when the sequence should wait on an earlier pack.</p>
            </div>
            <div class="toolbar-actions">
              <button class="button button-secondary" type="button" id="add-pack-row">Add pack</button>
            </div>
          </div>
          <div id="program-pack-list" class="program-pack-list"></div>
          <div class="inline-actions">
            <button type="submit">Create Program</button>
          </div>
        </form>
        <div id="program-result"></div>
      </div>
    </section>

    <section class="catalog-grid">
      <div class="section catalog-panel">
        <div class="toolbar">
          <div class="toolbar-copy">
            <h2>Known Packs</h2>
            <p>Run healthy packs directly or inspect broken ones without hiding them.</p>
          </div>
          <div class="toolbar-actions">
            <button class="button button-secondary" type="button" id="refresh-studio">Refresh</button>
          </div>
        </div>
        <div class="stack">
          ${packList.map(pack => `
            <article class="list-row">
              <div class="list-row-head">
                <div>
                  ${statusBadge(pack.valid ? "valid" : "invalid")}
                  <h3>${escapeHTML(pack.name)}</h3>
                  <p class="muted">${escapeHTML(pack.id)} • ${escapeHTML(pack.task_count)} tasks</p>
                </div>
                <div class="inline-actions">
                  ${pack.valid ? `<button class="button button-secondary" type="button" data-run-pack="${escapeHTML(pack.id)}">Run pack</button>` : ""}
                  <button class="button button-ghost" type="button" data-plan-pack="${escapeHTML(pack.id)}">Plan</button>
                </div>
              </div>
              ${pack.valid ? `<p class="muted">Ready for execution. Tasks will run sequentially through agent-runs.</p>` : `<div class="error-block">${escapeHTML(safeArray(pack.validation_errors).join("; "))}</div>`}
            </article>
          `).join("") || emptyState("No packs detected", "Use the form above to create the first runtime-compatible pack.")}
        </div>
      </div>

      <div class="section catalog-panel">
        <div class="toolbar">
          <div class="toolbar-copy">
            <h2>Known Programs</h2>
            <p>Programs are optional, but they make larger initiatives easier to track.</p>
          </div>
        </div>
        <div class="stack">
          ${programList.length ? programList.map(program => `
            <article class="list-row">
              <div class="list-row-head">
                <div>
                  ${statusBadge(program.valid ? "valid" : "invalid")}
                  <h3>${escapeHTML(program.name)}</h3>
                  <p class="muted">${escapeHTML(program.id)} • ${escapeHTML(program.pack_count)} packs</p>
                </div>
                ${program.valid ? `<button class="button button-secondary" type="button" data-run-program="${escapeHTML(program.id)}">Run program</button>` : ""}
              </div>
              ${program.valid ? `<p class="muted">Ready for sequential pack orchestration.</p>` : `<div class="error-block">${escapeHTML(safeArray(program.validation_errors).join("; "))}</div>`}
            </article>
          `).join("") : emptyState("No programs yet", "Create a program once you want progress to roll up across several packs.")}
        </div>
      </div>
    </section>
  `;

  const defaultTemplate = packTemplates[0];
  const taskListElement = document.getElementById("task-list");
  const programPackListElement = document.getElementById("program-pack-list");

  function renderTaskRows(tasks) {
    taskListElement.innerHTML = tasks.map((task, index) => taskRowMarkup(task, index)).join("");
    bindTaskRowActions();
  }

  function renderProgramPackRows(rows) {
    if (!validPacks.length) {
      programPackListElement.innerHTML = emptyState(
        "No valid packs available",
        "Repair or create at least one valid pack before building a program.",
      );
      return;
    }
    programPackListElement.innerHTML = rows.map((row, index) => packRowMarkup(row, index, validPacks)).join("");
    bindProgramPackActions();
  }

  function collectTaskRows() {
    return Array.from(taskListElement.querySelectorAll("[data-task-index]")).map(row => {
      const values = Object.fromEntries(
        Array.from(row.querySelectorAll("[data-field]")).map(field => [field.dataset.field, field.value]),
      );
      const taskID = slugify(values.id) || "task";
      return {
        id: taskID,
        title: values.title.trim(),
        summary: values.summary.trim(),
        branch: values.branch.trim() || `ptolemy/${taskID}`,
        depends_on: splitList(values.depends_on),
        allowed_files: splitList(values.allowed_files),
        validation: splitList(values.validation),
      };
    });
  }

  function collectProgramRows() {
    return Array.from(programPackListElement.querySelectorAll("[data-pack-index]")).map((row, index) => {
      const packID = row.querySelector('[data-field="pack_id"]').value;
      const dependsOn = row.querySelector('[data-field="depends_on"]').value;
      return {
        pack_id: packID,
        depends_on: splitList(dependsOn),
        order: index,
      };
    });
  }

  function applyTemplate(template) {
    document.querySelector('[name="pack_id"]').value = template.packID;
    document.querySelector('[name="name"]').value = template.packName;
    document.querySelector('[name="description"]').value = template.description;
    document.querySelector('[name="goal"]').value = template.goal;
    renderTaskRows(template.tasks);
  }

  function bindTaskRowActions() {
    taskListElement.querySelectorAll("[data-remove-task]").forEach(button => {
      button.addEventListener("click", event => {
        const rows = collectTaskRows();
        const index = Number(event.currentTarget.closest("[data-task-index]").dataset.taskIndex);
        rows.splice(index, 1);
        renderTaskRows(rows.length ? rows : [defaultTemplate.tasks[0]]);
      });
    });

    taskListElement.querySelectorAll("[data-move-task]").forEach(button => {
      button.addEventListener("click", event => {
        const rows = collectTaskRows();
        const index = Number(event.currentTarget.closest("[data-task-index]").dataset.taskIndex);
        const direction = event.currentTarget.dataset.moveTask;
        const targetIndex = direction === "up" ? index - 1 : index + 1;
        if (targetIndex < 0 || targetIndex >= rows.length) {
          return;
        }
        const [row] = rows.splice(index, 1);
        rows.splice(targetIndex, 0, row);
        renderTaskRows(rows);
      });
    });
  }

  function bindProgramPackActions() {
    programPackListElement.querySelectorAll("[data-remove-pack-row]").forEach(button => {
      button.addEventListener("click", event => {
        const rows = collectProgramRows();
        const index = Number(event.currentTarget.closest("[data-pack-index]").dataset.packIndex);
        rows.splice(index, 1);
        renderProgramPackRows(rows.length ? rows : [{ pack_id: validPacks[0]?.id || "", depends_on: [] }]);
      });
    });
  }

  renderTaskRows(defaultTemplate.tasks);
  renderProgramPackRows([{ pack_id: validPacks[0]?.id || "", depends_on: [] }]);

  view.querySelectorAll("[data-pack-template]").forEach(button => {
    button.addEventListener("click", () => {
      const template = packTemplates.find(item => item.id === button.dataset.packTemplate);
      if (template) {
        applyTemplate(template);
      }
    });
  });

  document.getElementById("add-task").addEventListener("click", () => {
    const rows = collectTaskRows();
    rows.push({
      id: `task-${rows.length + 1}`,
      title: `Task ${rows.length + 1}`,
      summary: "",
      branch: "",
      depends_on: rows.length ? [rows[rows.length - 1].id] : [],
      allowed_files: [],
      validation: ["go test ./..."],
    });
    renderTaskRows(rows);
  });

  document.getElementById("reset-pack").addEventListener("click", () => {
    applyTemplate(defaultTemplate);
    document.getElementById("pack-result").innerHTML = "";
  });

  document.getElementById("preview-pack").addEventListener("click", () => {
    const formValues = Object.fromEntries(new FormData(document.getElementById("pack-form")).entries());
    const payload = {
      pack_id: slugify(formValues.pack_id),
      name: formValues.name.trim(),
      description: formValues.description.trim(),
      goal: formValues.goal.trim(),
      created_by: "pack-studio",
      validation: ["go test ./..."],
      max_allowed_files: 8,
      require_validation: true,
      require_branch: true,
      stop_on_failure: true,
      tasks: collectTaskRows(),
    };
    document.getElementById("pack-result").innerHTML = `
      <div class="status-note is-neutral">
        <strong>Payload Preview</strong>
        <div class="error-block">${escapeHTML(JSON.stringify(payload, null, 2))}</div>
      </div>
    `;
  });

  document.getElementById("pack-form").addEventListener("submit", async event => {
    event.preventDefault();
    const formValues = Object.fromEntries(new FormData(event.currentTarget).entries());
    const payload = {
      pack_id: slugify(formValues.pack_id),
      name: formValues.name.trim(),
      description: formValues.description.trim(),
      goal: formValues.goal.trim(),
      created_by: "pack-studio",
      validation: ["go test ./..."],
      max_allowed_files: 8,
      require_validation: true,
      require_branch: true,
      stop_on_failure: true,
      tasks: collectTaskRows(),
    };

    try {
      const result = await getJSON("/ui/api/packs", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      document.getElementById("pack-result").innerHTML = renderMessage(`Created pack ${result.id}. Refreshing the catalog now.`, "success");
      await renderStudio();
    } catch (error) {
      document.getElementById("pack-result").innerHTML = renderMessage(error.message, "danger");
    }
  });

  document.getElementById("add-pack-row").addEventListener("click", () => {
    const rows = collectProgramRows();
    rows.push({ pack_id: validPacks[0]?.id || "", depends_on: [] });
    renderProgramPackRows(rows);
  });

  document.getElementById("program-form").addEventListener("submit", async event => {
    event.preventDefault();
    const formValues = Object.fromEntries(new FormData(event.currentTarget).entries());
    const payload = {
      program_id: slugify(formValues.program_id),
      name: formValues.name.trim(),
      description: formValues.description.trim(),
      packs: collectProgramRows(),
    };
    try {
      const result = await getJSON("/ui/api/programs", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      document.getElementById("program-result").innerHTML = renderMessage(`Created program ${result.id}. Refreshing the catalog now.`, "success");
      await renderStudio();
    } catch (error) {
      document.getElementById("program-result").innerHTML = renderMessage(error.message, "danger");
    }
  });

  document.getElementById("refresh-studio").addEventListener("click", () => renderStudio());

  view.querySelectorAll("[data-run-pack]").forEach(button => {
    button.addEventListener("click", () => startRun({ pack_id: button.dataset.runPack }));
  });

  view.querySelectorAll("[data-run-program]").forEach(button => {
    button.addEventListener("click", () => startRun({ program_id: button.dataset.runProgram }));
  });

  view.querySelectorAll("[data-plan-pack]").forEach(button => {
    button.addEventListener("click", async () => {
      try {
        const result = await getJSON(`/ui/api/packs/${encodeURIComponent(button.dataset.planPack)}/plan`);
        const ordered = safeArray(result.tasks).join(" -> ");
        document.getElementById("pack-result").innerHTML = renderMessage(`Plan for ${result.pack_id}: ${ordered}`, "neutral");
      } catch (error) {
        document.getElementById("pack-result").innerHTML = renderMessage(error.message, "danger");
      }
    });
  });
}

async function startRun(payload) {
  try {
    setShellStatus("loading", "Starting run");
    const run = await getJSON("/ui/api/program-runs", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    window.location.href = `/ui/runs/${run.id}`;
  } catch (error) {
    setShellStatus("issue", "Run could not start");
    alert(error.message);
  }
}

function renderRunTree(detail) {
  const packs = safeArray(detail.packs);
  if (!packs.length) {
    return emptyState("No pack tree available", "This run has not materialized its task tree yet.");
  }
  return packs.map(pack => `
    <article class="tree-node pack">
      <div class="tree-node-head">
        <div>
          ${statusBadge(pack.status)}
          <h3>${escapeHTML(pack.pack_name)}</h3>
          <p class="muted">${escapeHTML(pack.completed_tasks)}/${escapeHTML(pack.total_tasks)} tasks complete</p>
        </div>
        <span class="pill">${escapeHTML(percentText(pack.percent_complete))}</span>
      </div>
      <div class="progress"><span style="width:${Number(pack.percent_complete || 0)}%"></span></div>
      <div class="tree-children">
        ${safeArray(pack.tasks).map(task => `
          <article class="tree-node task">
            <div class="tree-node-head">
              <div>
                ${statusBadge(task.status)}
                <h3>${escapeHTML(task.title)}</h3>
                <p class="muted">${escapeHTML(task.task_id)}</p>
              </div>
              ${task.session_id ? `<span class="pill mono">${escapeHTML(task.session_id.slice(0, 8))}</span>` : ""}
            </div>
            ${task.last_error ? `<div class="error-block">${escapeHTML(task.last_error)}</div>` : ""}
            ${task.agent_run_id ? `
              <article class="tree-node agent">
                <div class="tree-node-head">
                  <div>
                    ${statusBadge(task.status)}
                    <h3>Agent session</h3>
                    <p class="muted mono">${escapeHTML(task.agent_run_id)}</p>
                  </div>
                </div>
              </article>
            ` : ""}
          </article>
        `).join("")}
      </div>
    </article>
  `).join("");
}

function renderChecklist(items) {
  const checklist = safeArray(items);
  if (!checklist.length) {
    return `<p class="muted">No checklist available for this task yet.</p>`;
  }
  return `
    <ul class="checklist">
      ${checklist.map(item => `
        <li class="${item.checked ? "done" : ""}">
          <span class="checklist-mark">${item.checked ? "✓" : "·"}</span>
          <span>${escapeHTML(item.text)}</span>
        </li>
      `).join("")}
    </ul>
  `;
}

function renderEventList(events) {
  const rows = safeArray(events);
  if (!rows.length) {
    return `<p class="muted">No run events yet.</p>`;
  }
  return rows.map(event => `
    <article class="list-row">
      <div class="list-row-head">
        <div>
          ${statusBadge(event.event_type.includes("failed") ? "failed" : event.event_type.includes("finished") ? "completed" : "running")}
          <h3>${escapeHTML(event.event_type)}</h3>
          <p class="muted">${escapeHTML(formatDate(event.created_at))}</p>
        </div>
      </div>
      <p>${escapeHTML(event.message)}</p>
    </article>
  `).join("");
}

function renderActionList(actions) {
  const rows = safeArray(actions);
  if (!rows.length) {
    return `<p class="muted">No agent actions have been recorded for the current task yet.</p>`;
  }
  return rows.map(action => `
    <article class="list-row">
      <div class="list-row-head">
        <div>
          ${statusBadge(action.status || "pending")}
          <h3>${escapeHTML(action.type || action.action || "action")}</h3>
        </div>
      </div>
      <p class="muted">${escapeHTML(firstNonEmpty(action.target, action.title, action.input, action.reason, "No action detail recorded"))}</p>
    </article>
  `).join("");
}

function renderObservationList(observations) {
  const rows = safeArray(observations);
  if (!rows.length) {
    return `<p class="muted">No observations yet.</p>`;
  }
  return rows.map(observation => `
    <article class="list-row">
      <div class="list-row-head">
        <div>
          ${statusBadge("completed")}
          <h3>${escapeHTML(observation.source || "observation")}</h3>
        </div>
      </div>
      <p>${escapeHTML(observation.summary || "")}</p>
    </article>
  `).join("");
}

function renderCommandList(commandLogs) {
  const rows = safeArray(commandLogs);
  if (!rows.length) {
    return `<p class="muted">No command logs recorded for this session yet.</p>`;
  }
  return rows.map(log => `
    <article class="list-row">
      <div class="list-row-head">
        <div>
          <h3 class="mono">${escapeHTML(log.command)}</h3>
          <p class="muted">Exit ${escapeHTML(log.exit_code)} • ${escapeHTML(log.duration_ms)}ms</p>
        </div>
      </div>
      ${log.reason ? `<p class="muted">${escapeHTML(log.reason)}</p>` : ""}
    </article>
  `).join("");
}

function waitingLabel(value) {
  return String(value || "unknown").replaceAll("_", " ");
}

function renderPathList(items, emptyMessage, className = "compact-list") {
  const rows = safeArray(items);
  if (!rows.length) {
    return `<p class="muted">${escapeHTML(emptyMessage)}</p>`;
  }
  return `
    <ul class="${className}">
      ${rows.map(item => `<li><span class="mono">${escapeHTML(item)}</span></li>`).join("")}
    </ul>
  `;
}

function renderManifestProgress(items, currentTaskId) {
  const rows = safeArray(items);
  if (!rows.length) {
    return `<p class="muted">No child-task manifest is active for this run.</p>`;
  }
  return `
    <div class="manifest-list">
      ${rows.map(item => `
        <article class="manifest-row ${item.id === currentTaskId ? "is-current" : ""}">
          <div class="list-row-head">
            <div>
              ${statusBadge(item.status || "pending")}
              <h3>${escapeHTML(firstNonEmpty(item.title, item.id))}</h3>
              <p class="muted">${escapeHTML(firstNonEmpty(item.phase, "phase pending"))}</p>
            </div>
            <div class="mini-meta">
              <span class="pill mono">${escapeHTML(item.id)}</span>
              <span class="pill">${escapeHTML(item.estimated_body_tokens || 0)} tok</span>
            </div>
          </div>
          ${item.warning ? `<p class="warning-text">${escapeHTML(item.warning)}</p>` : ""}
        </article>
      `).join("")}
    </div>
  `;
}

function renderPromptContext(operatorState, currentTask) {
  const agentRun = operatorState?.agent_run || null;
  const prompt = agentRun?.prompt || null;
  const waitingOn = firstNonEmpty(agentRun?.waiting_on, "brain_response");
  const currentTitle = firstNonEmpty(prompt?.current_task_title, currentTask?.title, "Waiting for active child task");
  const currentId = firstNonEmpty(prompt?.current_task_id, currentTask?.task_id, "pending");
  const currentPhase = firstNonEmpty(prompt?.current_task_phase, agentRun?.run?.current_phase, currentTask?.status, "pending");

  return `
    ${prompt?.error ? renderMessage(prompt.error, "danger") : renderMessage("Compact prompt assembly details are live here. Use the debug disclosure only when you need deeper prompt inspection.", "neutral")}
    <article class="timeline-card">
      <div class="timeline-label">
        <strong>${escapeHTML(currentTitle)}</strong>
        ${statusBadge(waitingOn, waitingOn)}
      </div>
      <div class="mini-meta">
        <span class="pill mono">${escapeHTML(currentId)}</span>
        <span class="pill">${escapeHTML(currentPhase)}</span>
        ${agentRun?.latest_action_type ? `<span class="pill">${escapeHTML(agentRun.latest_action_type)}</span>` : ""}
      </div>
      <div class="key-value-grid">
        <div class="key-value"><span>Waiting On</span>${escapeHTML(waitingLabel(waitingOn))}</div>
        <div class="key-value"><span>Brain Timeout</span>${escapeHTML(firstNonEmpty(prompt?.brain_timeout, "unknown"))}</div>
        <div class="key-value"><span>Prompt Tokens</span>${escapeHTML(prompt?.estimated_prompt_tokens ?? "0")}</div>
        <div class="key-value"><span>Max Context</span>${escapeHTML(prompt?.max_context_tokens ?? "0")}</div>
      </div>
    </article>

    <article class="timeline-card">
      <div class="timeline-label">
        <strong>Context Files</strong>
      </div>
      <div class="operator-grid">
        <div class="surface-inset">
          <p class="metric-label">Included</p>
          ${renderPathList(prompt?.included_context_files, "No context files were included in the assembled prompt.")}
        </div>
        <div class="surface-inset">
          <p class="metric-label">Trimmed</p>
          ${renderPathList(prompt?.trimmed_context_files, "No context files were trimmed for this prompt.")}
        </div>
      </div>
    </article>

    <article class="timeline-card">
      <div class="timeline-label">
        <strong>Previous Child Summaries</strong>
      </div>
      ${renderPathList(prompt?.previous_child_summaries, "No previous child summaries are included for this step.")}
    </article>

    <article class="timeline-card">
      <div class="timeline-label">
        <strong>Manifest Slice</strong>
      </div>
      ${renderManifestProgress(prompt?.manifest_progress, prompt?.current_task_id)}
    </article>

    ${prompt?.raw_prompt_preview ? `
      <details class="debug-panel">
        <summary>Debug prompt preview${prompt.raw_prompt_preview_truncated ? " (truncated)" : ""}</summary>
        <pre class="prompt-preview">${escapeHTML(prompt.raw_prompt_preview)}</pre>
      </details>
    ` : ""}
  `;
}

function renderRunMetrics(run, currentPack, currentTask, failureMessage) {
  return `
    ${metricCard("Program", run.program_name, run.mode === "program" ? "Multi-pack orchestration" : "Single-pack execution")}
    ${metricCard("Progress", percentText(run.percent_complete), `${run.completed_tasks}/${run.total_tasks} tasks complete`)}
    ${metricCard("Current Pack", firstNonEmpty(currentPack?.pack_name, run.current_pack_id, "Waiting"), firstNonEmpty(currentTask?.title, formatDate(run.updated_at)), true)}
    ${metricCard("Status", statusLabel(run.status), failureMessage ? "Failure details captured below" : "No blocking error recorded")}
  `;
}

function renderRunProgress(payload) {
  const detail = payload.detail;
  const run = detail.program_run;
  const currentTask = payload.current_task;
  const actions = safeArray(payload.actions);
  const observations = safeArray(payload.observations);
  const failureMessage = firstNonEmpty(run.last_error, currentTask?.last_error);

  return `
    ${failureMessage ? renderMessage(failureMessage, "danger") : run.status === "completed" ? renderMessage("This run completed cleanly. Review the final events and command history below.", "success") : renderMessage("Live operator updates are flowing below while the task pack advances.", "neutral")}
    <article class="timeline-card">
      <div class="timeline-label">
        <strong>Run Summary</strong>
        ${statusBadge(run.status)}
      </div>
      <div class="progress"><span style="width:${Number(run.percent_complete || 0)}%"></span></div>
      <div class="key-value-grid">
        <div class="key-value"><span>Workspace</span>${escapeHTML(run.workspace)}</div>
        <div class="key-value"><span>Updated</span>${escapeHTML(formatDate(run.updated_at))}</div>
        <div class="key-value"><span>Started</span>${escapeHTML(formatDate(run.started_at))}</div>
        <div class="key-value"><span>Finished</span>${escapeHTML(formatDate(run.finished_at))}</div>
      </div>
    </article>

    <article class="timeline-card">
      <div class="timeline-label">
        <strong>${escapeHTML(currentTask ? currentTask.title : "Current Task")}</strong>
        ${statusBadge(currentTask?.status || run.status)}
      </div>
      ${currentTask ? `
        <div class="mini-meta">
          <span class="pill mono">${escapeHTML(currentTask.task_id)}</span>
          <span class="pill">${escapeHTML(firstNonEmpty(currentTask.branch, "No branch"))}</span>
          <span class="pill mono">${escapeHTML(firstNonEmpty(currentTask.session_id, "Session pending"))}</span>
        </div>
        ${renderChecklist(currentTask.checklist)}
      ` : `<p class="muted">No current task is materialized yet.</p>`}
    </article>

    <article class="timeline-card">
      <div class="timeline-label">
        <strong>Recent Events</strong>
      </div>
      <div id="event-list" class="event-list">${renderEventList(safeArray(detail.events).slice(-10))}</div>
    </article>

    <article class="timeline-card">
      <div class="timeline-label">
        <strong>Recent Agent Actions</strong>
      </div>
      <div class="action-list">${renderActionList(actions)}</div>
    </article>

    <article class="timeline-card">
      <div class="timeline-label">
        <strong>Observations</strong>
      </div>
      <div class="action-list">${renderObservationList(observations)}</div>
    </article>
  `;
}

function applyRunPayload(payload) {
  const detail = payload.detail;
  const currentTask = payload.current_task;
  const operatorState = payload.operator_state || {};
  const run = detail.program_run;
  const currentPack = safeArray(detail.packs).find(pack => pack.pack_id === run.current_pack_id) || safeArray(detail.packs)[0] || null;
  const failureMessage = firstNonEmpty(run.last_error, currentTask?.last_error);

  const runStatusText = run.status === "failed"
    ? "Run stopped with a recorded failure"
    : run.status === "completed"
      ? "Run completed successfully"
      : "Run is actively streaming";
  setShellStatus(run.status === "failed" ? "issue" : "ready", runStatusText);

  const metrics = document.getElementById("run-metrics");
  if (metrics) {
    metrics.innerHTML = renderRunMetrics(run, currentPack, currentTask, failureMessage);
  }

  const tree = document.getElementById("run-tree");
  if (tree) {
    tree.innerHTML = renderRunTree(detail);
  }

  const progress = document.getElementById("run-progress");
  if (progress) {
    progress.innerHTML = renderRunProgress(payload);
  }

  const promptPanel = document.getElementById("prompt-context");
  if (promptPanel) {
    promptPanel.innerHTML = renderPromptContext(operatorState, currentTask);
  }

  const commandHistory = document.getElementById("command-history");
  if (commandHistory) {
    commandHistory.innerHTML = renderCommandList(safeArray(payload.command_logs));
  }

  const cancelButton = document.getElementById("cancel-run");
  if (cancelButton) {
    const disabled = ["failed", "completed", "cancelled"].includes(run.status);
    cancelButton.disabled = disabled;
    cancelButton.className = `button ${disabled ? "button-ghost" : "button-danger"}`;
  }

  updateTerminalMeta(operatorState.terminal || {});
}

async function renderRun(runId) {
  cleanupLiveSources();
  highlightNav();
  activeRunId = runId;
  terminalBuffer = "";
  setRunsNavTarget(`/ui/runs/${encodeURIComponent(runId)}`);

  view.innerHTML = `
    <section id="run-metrics" class="cards"></section>

    <section class="run-grid">
      <div class="run-column">
        <div class="pane">
          <div class="pane-header">
            <div>
              <h2>Program Tree</h2>
              <p>Follow progress from pack to task to agent session.</p>
            </div>
          </div>
          <div id="run-tree" class="pane-body tree"></div>
        </div>

        <div class="pane">
          <div class="pane-header">
            <div>
              <h2>Progress</h2>
              <p>Checklist state, event flow, and current task detail.</p>
            </div>
            <div class="toolbar-actions">
              <button class="button button-secondary" type="button" id="refresh-run">Refresh</button>
              <button class="button button-ghost" type="button" id="cancel-run">Cancel run</button>
            </div>
          </div>
          <div id="run-progress" class="pane-body timeline"></div>
        </div>
      </div>

      <div class="run-column run-ops-column">
        <div class="pane">
          <div class="pane-header">
            <div>
              <h2>Prompt / Context</h2>
              <p>Operator-safe view of the current child-task prompt assembly.</p>
            </div>
          </div>
          <div id="prompt-context" class="pane-body timeline prompt-pane"></div>
        </div>

        <div class="pane pane-terminal">
          <div class="pane-header">
            <div>
              <h2>Live Terminal</h2>
              <p>Streaming tmux capture with a full-viewer fallback snapshot.</p>
            </div>
            <div class="toolbar-actions">
              <button class="button button-secondary" type="button" id="refresh-terminal">Refresh terminal</button>
              <label class="pill"><input id="auto-scroll-terminal" type="checkbox" ${autoScrollTerminal ? "checked" : ""}> Auto-scroll</label>
            </div>
          </div>
          <div class="pane-body terminal-wrap">
            <div class="terminal-shell">
              <div id="terminal-meta" class="terminal-meta"></div>
              <div class="terminal-toolbar">
                <div id="terminal-freshness" class="terminal-freshness">Waiting for first terminal update.</div>
              </div>
              <pre id="terminal" class="terminal"></pre>
              <article class="timeline-card">
                <div class="timeline-label">
                  <strong>Command History</strong>
                </div>
                <div id="command-history" class="command-list"></div>
              </article>
            </div>
          </div>
        </div>
      </div>
    </section>
  `;

  document.getElementById("refresh-run").addEventListener("click", () => refreshRunData(runId));
  document.getElementById("cancel-run").addEventListener("click", async () => {
    try {
      await getJSON(`/ui/api/program-runs/${encodeURIComponent(runId)}/cancel`, { method: "POST" });
      await refreshRunData(runId);
      await pollTerminalSnapshot(runId, true);
    } catch (error) {
      alert(error.message);
    }
  });
  document.getElementById("refresh-terminal").addEventListener("click", () => pollTerminalSnapshot(runId, true));
  document.getElementById("auto-scroll-terminal").addEventListener("change", event => {
    autoScrollTerminal = event.currentTarget.checked;
  });

  await refreshRunData(runId);
  subscribeTerminal(runId);
  startRunPolling(runId);
  await pollTerminalSnapshot(runId, true);
}

async function refreshRunData(runId) {
  if (activeRunId !== runId) {
    return;
  }
  const payload = await getJSON(`/ui/api/program-runs/${encodeURIComponent(runId)}`);
  if (activeRunId !== runId) {
    return;
  }
  applyRunPayload(payload);
}

function startRunPolling(runId) {
  runRefreshTimer = window.setInterval(() => {
    if (activeRunId !== runId) {
      return;
    }
    refreshRunData(runId).catch(() => {});
  }, 4000);

  terminalPollTimer = window.setInterval(() => {
    if (activeRunId !== runId) {
      return;
    }
    const stale = Date.now() - terminalLastEventAt > 2200;
    if (stale) {
      pollTerminalSnapshot(runId, false).catch(() => {});
    }
  }, 1600);
}

function updateTerminalMeta(state) {
  const meta = document.getElementById("terminal-meta");
  const freshness = document.getElementById("terminal-freshness");
  if (!meta || !freshness) {
    return;
  }

  meta.innerHTML = `
    <div>
      <strong>${escapeHTML(firstNonEmpty(state.task_title, state.task_id, "Waiting for task session"))}</strong>
      <p class="terminal-caption">${escapeHTML(firstNonEmpty(state.message, "Waiting for terminal state"))}</p>
    </div>
    <div class="inline-actions">
      <span class="pill mono">${escapeHTML(firstNonEmpty(state.session_id, "No session"))}</span>
      <span class="pill">${escapeHTML(firstNonEmpty(state.phase, "pending"))}</span>
      <span class="pill">${escapeHTML(firstNonEmpty(state.status, "unknown"))}</span>
    </div>
  `;

  freshness.textContent = state.last_updated
    ? `Last updated ${formatDate(state.last_updated)} • source ${firstNonEmpty(state.source, "none")}`
    : "Waiting for first terminal update.";
}

function applyTerminalState(state, snapshotMode = "replace") {
  updateTerminalMeta(state);

  const terminal = document.getElementById("terminal");
  if (!terminal) {
    return;
  }

  const nextSnapshot = state.capture_available ? (state.snapshot || "") : `${firstNonEmpty(state.message, "Waiting for terminal output.")}\n`;
  if (snapshotMode === "append" && terminalBuffer && nextSnapshot.startsWith(terminalBuffer)) {
    terminalBuffer = nextSnapshot;
  } else {
    terminalBuffer = nextSnapshot;
  }
  terminal.textContent = terminalBuffer;
  if (autoScrollTerminal) {
    terminal.scrollTop = terminal.scrollHeight;
  }
}

async function pollTerminalSnapshot(runId, forceRefresh) {
  if (activeRunId !== runId) {
    return;
  }
  if (!forceRefresh && terminalSource && Date.now() - terminalLastEventAt < 1200) {
    return;
  }

  const state = await getJSON(`/ui/api/program-runs/${encodeURIComponent(runId)}/terminal`);
  if (activeRunId !== runId) {
    return;
  }
  terminalLastEventAt = Date.now();
  applyTerminalState(state, "replace");
}

function subscribeTerminal(runId) {
  const terminal = document.getElementById("terminal");
  const meta = document.getElementById("terminal-meta");
  if (!terminal || !meta) {
    return;
  }
  terminalSource = new EventSource(`/ui/api/program-runs/${encodeURIComponent(runId)}/terminal/stream`);
  terminalSource.addEventListener("meta", event => {
    const payload = JSON.parse(event.data);
    terminalLastEventAt = Date.now();
    updateTerminalMeta({
      session_id: payload.session_id,
      task_id: payload.task_id,
      task_title: payload.task_title,
      phase: payload.phase,
      status: payload.status,
      message: payload.message,
      source: payload.source,
      last_updated: new Date().toISOString(),
    });
  });
  terminalSource.addEventListener("terminal", event => {
    const payload = JSON.parse(event.data);
    terminalLastEventAt = Date.now();
    if (payload.mode === "append") {
      terminalBuffer += payload.content || "";
      terminal.textContent = terminalBuffer;
      updateTerminalMeta({
        session_id: payload.session_id,
        task_id: payload.task_id,
        status: payload.status,
        last_updated: new Date().toISOString(),
      });
    } else {
      applyTerminalState({
        session_id: payload.session_id,
        task_id: payload.task_id,
        status: payload.status,
        capture_available: true,
        snapshot: payload.content || "",
        last_updated: new Date().toISOString(),
      }, "replace");
    }
    if (autoScrollTerminal) {
      terminal.scrollTop = terminal.scrollHeight;
    }
  });
  terminalSource.addEventListener("error", () => {
    pollTerminalSnapshot(runId, true).catch(() => {});
  });
}

function renderFatal(error) {
  view.innerHTML = `
    <section class="section">
      <div class="section-header">
        <div class="toolbar-copy">
          <h2>Surface Error</h2>
          <p class="section-title">The UI hit a blocking problem.</p>
        </div>
      </div>
      ${renderMessage(error.message || String(error), "danger")}
    </section>
  `;
}

boot();
