const state = {
  token: localStorage.getItem("dcst_admin_token") || "",
  unlocked: false,
  config: null,
  snapshot: null,
};

const elements = {
  runBadge: document.querySelector("#runBadge"),
  resultCount: document.querySelector("#resultCount"),
  bestSpeed: document.querySelector("#bestSpeed"),
  lastRun: document.querySelector("#lastRun"),
  nextRun: document.querySelector("#nextRun"),
  countdown: document.querySelector("#countdown"),
  ipSource: document.querySelector("#ipSource"),
  resultBody: document.querySelector("#resultBody"),
  errorBanner: document.querySelector("#errorBanner"),
  tokenInput: document.querySelector("#tokenInput"),
  unlockButton: document.querySelector("#unlockButton"),
  runButton: document.querySelector("#runButton"),
  refreshButton: document.querySelector("#refreshButton"),
  adminState: document.querySelector("#adminState"),
  configForm: document.querySelector("#configForm"),
  saveMessage: document.querySelector("#saveMessage"),
};

const configFields = [
  "interval_minutes", "candidates", "streams", "test_duration_seconds",
  "warmup_seconds", "test_port", "max_latency_ms", "max_loss_rate",
  "min_speed_mb_s", "test_url", "use_ipv6",
];

async function request(path, options = {}) {
  const headers = new Headers(options.headers || {});
  headers.set("Accept", "application/json");
  if (state.token) headers.set("Authorization", `Bearer ${state.token}`);
  if (options.body) headers.set("Content-Type", "application/json");
  const response = await fetch(path, { ...options, headers });
  if (response.status === 204) return null;
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(data.error || `HTTP ${response.status}`);
  return data;
}

function setAdminEnabled(enabled) {
  state.unlocked = enabled;
  elements.runButton.disabled = !enabled;
  elements.configForm.querySelector("button[type=submit]").disabled = !enabled;
  elements.adminState.textContent = enabled ? "管理功能已解锁" : "管理功能已锁定";
  elements.adminState.style.color = enabled ? "var(--green)" : "";
}

function formatTime(value) {
  if (!value) return "--";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "--";
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit",
  }).format(date);
}

function renderResults(snapshot) {
  const results = snapshot.results || [];
  elements.resultCount.textContent = String(results.length);
  elements.bestSpeed.textContent = results.length
    ? Number(results[0].download_mb_s).toFixed(2)
    : "--";
  elements.lastRun.textContent = formatTime(snapshot.updated_at || snapshot.last_run_at);
  elements.nextRun.textContent = formatTime(snapshot.next_run_at);
  elements.ipSource.textContent = snapshot.ip_source === "cloudflare-api"
    ? "Cloudflare 官方 IP 段"
    : snapshot.ready ? "内置 IP 段回退" : "等待首次测速";

  elements.resultBody.replaceChildren();
  if (!results.length) {
    const row = document.createElement("tr");
    const cell = document.createElement("td");
    cell.className = "empty";
    cell.colSpan = 6;
    cell.textContent = snapshot.running ? "正在执行首次测速…" : "暂无测速结果";
    row.append(cell);
    elements.resultBody.append(row);
  } else {
    results.forEach((item, index) => {
      const row = document.createElement("tr");
      const values = [
        String(index + 1),
        item.ip,
        `${Number(item.download_mb_s).toFixed(2)} MB/s`,
        `${Number(item.latency_ms).toFixed(2)} ms`,
        `${(Number(item.loss_rate) * 100).toFixed(0)}%`,
        item.region || "N/A",
      ];
      values.forEach((value, col) => {
        const cell = document.createElement("td");
        cell.textContent = value;
        if (col === 5 && value !== "N/A") cell.style.color = "var(--amber)";
        row.append(cell);
      });
      elements.resultBody.append(row);
    });
  }

  elements.errorBanner.hidden = !snapshot.last_error;
  elements.errorBanner.textContent = snapshot.last_error ? `最近一次任务失败：${snapshot.last_error}` : "";
  elements.runButton.disabled = !state.unlocked || snapshot.running;
  elements.runBadge.className = "badge";
  if (snapshot.running) {
    elements.runBadge.classList.add("running");
    elements.runBadge.textContent = "测速运行中";
  } else if (snapshot.last_error) {
    elements.runBadge.classList.add("error");
    elements.runBadge.textContent = "任务异常";
  } else if (snapshot.ready) {
    elements.runBadge.classList.add("ok");
    elements.runBadge.textContent = "自动运行中";
  } else {
    elements.runBadge.classList.add("pending");
    elements.runBadge.textContent = "准备中";
  }
}

function updateCountdown() {
  if (!state.snapshot?.next_run_at) {
    elements.countdown.textContent = "自动调度";
    return;
  }
  const diff = new Date(state.snapshot.next_run_at).getTime() - Date.now();
  if (diff <= 0) {
    elements.countdown.textContent = "即将开始";
    return;
  }
  const minutes = Math.floor(diff / 60000);
  const seconds = Math.floor((diff % 60000) / 1000);
  elements.countdown.textContent = `${minutes} 分 ${String(seconds).padStart(2, "0")} 秒后`;
}

async function loadStatus() {
  state.snapshot = await request("/api/status");
  renderResults(state.snapshot);
  updateCountdown();
}

function fillConfig(config) {
  state.config = config;
  configFields.forEach((name) => {
    const field = elements.configForm.elements.namedItem(name);
    if (!field) return;
    if (field.type === "checkbox") field.checked = Boolean(config[name]);
    else field.value = config[name];
  });
}

async function loadConfig() {
  fillConfig(await request("/api/config"));
}

async function unlock() {
  state.token = elements.tokenInput.value.trim();
  if (!state.token) {
    elements.saveMessage.textContent = "请输入管理员令牌";
    return;
  }
  try {
    await request("/api/auth", { method: "POST" });
    localStorage.setItem("dcst_admin_token", state.token);
    setAdminEnabled(true);
    elements.saveMessage.textContent = "管理功能已解锁";
  } catch (error) {
    localStorage.removeItem("dcst_admin_token");
    state.token = "";
    setAdminEnabled(false);
    elements.saveMessage.textContent = error.message;
  }
}

async function runNow() {
  elements.runButton.disabled = true;
  try {
    await request("/api/run", { method: "POST" });
    elements.saveMessage.textContent = "测速任务已启动";
    await loadStatus();
  } catch (error) {
    elements.saveMessage.textContent = error.message;
  } finally {
    elements.runButton.disabled = !state.unlocked;
  }
}

async function saveConfig(event) {
  event.preventDefault();
  if (!state.unlocked) return;
  const payload = {};
  configFields.forEach((name) => {
    const field = elements.configForm.elements.namedItem(name);
    if (!field) return;
    if (field.type === "checkbox") payload[name] = field.checked;
    else if (field.type === "number") payload[name] = Number(field.value);
    else payload[name] = field.value;
  });
  try {
    const updated = await request("/api/config", {
      method: "POST",
      body: JSON.stringify(payload),
    });
    fillConfig(updated);
    elements.saveMessage.textContent = "配置已保存，自动调度已重置";
    await loadStatus();
  } catch (error) {
    elements.saveMessage.textContent = error.message;
  }
}

elements.tokenInput.value = state.token;
elements.unlockButton.addEventListener("click", unlock);
elements.runButton.addEventListener("click", runNow);
elements.refreshButton.addEventListener("click", () => loadStatus().catch((error) => {
  elements.saveMessage.textContent = error.message;
}));
elements.configForm.addEventListener("submit", saveConfig);

setAdminEnabled(false);
Promise.all([loadStatus(), loadConfig()]).catch((error) => {
  elements.saveMessage.textContent = error.message;
});
setInterval(() => loadStatus().catch(() => {}), 5000);
setInterval(updateCountdown, 1000);

if (state.token) {
  request("/api/auth", { method: "POST" })
    .then(() => setAdminEnabled(true))
    .catch(() => {
      localStorage.removeItem("dcst_admin_token");
      state.token = "";
      setAdminEnabled(false);
    });
}
