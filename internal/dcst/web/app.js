const state = {
  initialized: false,
  authenticated: false,
  authMode: "login",
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
  unlockButton: document.querySelector("#unlockButton"),
  logoutButton: document.querySelector("#logoutButton"),
  runButton: document.querySelector("#runButton"),
  refreshButton: document.querySelector("#refreshButton"),
  adminState: document.querySelector("#adminState"),
  configForm: document.querySelector("#configForm"),
  saveMessage: document.querySelector("#saveMessage"),
  passwordButton: document.querySelector("#passwordButton"),
  updateButton: document.querySelector("#updateButton"),
  authModal: document.querySelector("#authModal"),
  authEyebrow: document.querySelector("#authEyebrow"),
  authTitle: document.querySelector("#authTitle"),
  authDescription: document.querySelector("#authDescription"),
  authCloseButton: document.querySelector("#authCloseButton"),
  authError: document.querySelector("#authError"),
  setupForm: document.querySelector("#setupForm"),
  loginForm: document.querySelector("#loginForm"),
  changePasswordForm: document.querySelector("#changePasswordForm"),
  setupPassword: document.querySelector("#setupPassword"),
  setupConfirm: document.querySelector("#setupConfirm"),
  loginPassword: document.querySelector("#loginPassword"),
  currentPassword: document.querySelector("#currentPassword"),
  newPassword: document.querySelector("#newPassword"),
  newPasswordConfirm: document.querySelector("#newPasswordConfirm"),
};

const configFields = [
  "interval_minutes", "candidates", "streams", "test_duration_seconds",
  "warmup_seconds", "test_port", "max_latency_ms", "max_loss_rate",
  "min_speed_mb_s", "test_url", "use_ipv6",
];

async function request(path, options = {}) {
  const headers = new Headers(options.headers || {});
  headers.set("Accept", "application/json");
  if (options.body) headers.set("Content-Type", "application/json");
  const response = await fetch(path, { ...options, headers, credentials: "same-origin" });
  if (response.status === 204) return null;
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    const error = new Error(data.error || `HTTP ${response.status}`);
    error.status = response.status;
    throw error;
  }
  return data;
}

function setAdminEnabled(enabled) {
  state.authenticated = enabled;
  const running = Boolean(state.snapshot?.running);
  elements.runButton.disabled = !enabled || running;
  elements.configForm.querySelector("button[type=submit]").disabled = !enabled;
  elements.passwordButton.disabled = !enabled;
  elements.updateButton.hidden = !state.snapshot?.update_enabled;
  elements.updateButton.disabled = !enabled || !state.snapshot?.update_enabled;
  elements.logoutButton.hidden = !enabled;
  elements.unlockButton.hidden = enabled;
  elements.adminState.textContent = enabled ? "管理功能已登录" : "管理功能未登录";
  elements.adminState.style.color = enabled ? "var(--green)" : "";
  if (enabled && state.authMode !== "setup") closeAuthModal();
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
  elements.runButton.disabled = !state.authenticated || snapshot.running;
  elements.updateButton.hidden = !snapshot.update_enabled;
  elements.updateButton.disabled = !state.authenticated || !snapshot.update_enabled;
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

function setAuthMode(mode) {
  state.authMode = mode;
  elements.setupForm.hidden = mode !== "setup";
  elements.loginForm.hidden = mode !== "login";
  elements.changePasswordForm.hidden = mode !== "change";
  elements.authError.hidden = true;
  elements.authError.textContent = "";
  elements.authCloseButton.hidden = mode === "setup";
  if (mode === "setup") {
    elements.authEyebrow.textContent = "FIRST RUN SETUP";
    elements.authTitle.textContent = "初始化管理员密码";
    elements.authDescription.textContent = "首次使用请设置一个 8 位数字密码。密码只保存为安全哈希，不保存明文。";
  } else if (mode === "change") {
    elements.authEyebrow.textContent = "SECURITY";
    elements.authTitle.textContent = "修改登录密码";
    elements.authDescription.textContent = "验证当前密码后设置新的 8 位数字密码，已有登录会话将失效。";
  } else {
    elements.authEyebrow.textContent = "ADMIN ACCESS";
    elements.authTitle.textContent = "登录管理";
    elements.authDescription.textContent = "输入初始化时设置的 8 位数字密码。";
  }
}

function openAuthModal(mode) {
  setAuthMode(mode);
  elements.authModal.hidden = false;
  const firstInput = mode === "setup"
    ? elements.setupPassword
    : mode === "change" ? elements.currentPassword : elements.loginPassword;
  firstInput.focus();
}

function closeAuthModal() {
  if (state.authMode === "setup") return;
  elements.authModal.hidden = true;
  elements.authError.hidden = true;
  [elements.loginPassword, elements.currentPassword, elements.newPassword, elements.newPasswordConfirm]
    .forEach((input) => { input.value = ""; });
}

function showAuthError(error) {
  elements.authError.textContent = error.message || String(error);
  elements.authError.hidden = false;
}

async function refreshAuthStatus() {
  const auth = await request("/api/auth/status");
  state.initialized = auth.initialized;
  setAdminEnabled(auth.authenticated);
  if (!auth.initialized) openAuthModal("setup");
}

async function setupPassword(event) {
  event.preventDefault();
  const password = elements.setupPassword.value;
  const confirm = elements.setupConfirm.value;
  try {
    await request("/api/setup", {
      method: "POST",
      body: JSON.stringify({ password, confirm }),
    });
    state.initialized = true;
    state.authMode = "login";
    elements.setupPassword.value = "";
    elements.setupConfirm.value = "";
    elements.authModal.hidden = true;
    setAdminEnabled(true);
    elements.saveMessage.textContent = "管理员密码初始化完成";
  } catch (error) {
    showAuthError(error);
  }
}

async function login(event) {
  event.preventDefault();
  try {
    await request("/api/login", {
      method: "POST",
      body: JSON.stringify({ password: elements.loginPassword.value }),
    });
    setAdminEnabled(true);
    elements.saveMessage.textContent = "登录成功";
  } catch (error) {
    showAuthError(error);
  }
}

async function changePassword(event) {
  event.preventDefault();
  try {
    await request("/api/password", {
      method: "POST",
      body: JSON.stringify({
        current: elements.currentPassword.value,
        password: elements.newPassword.value,
        confirm: elements.newPasswordConfirm.value,
      }),
    });
    closeAuthModal();
    elements.saveMessage.textContent = "登录密码已更新";
  } catch (error) {
    showAuthError(error);
  }
}

async function logout() {
  try {
    await request("/api/logout", { method: "POST" });
  } finally {
    setAdminEnabled(false);
    elements.saveMessage.textContent = "已退出管理登录";
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
    elements.runButton.disabled = !state.authenticated;
  }
}

async function saveConfig(event) {
  event.preventDefault();
  if (!state.authenticated) return;
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

async function requestUpdate() {
  const previousVersion = state.snapshot?.version || "";
  elements.updateButton.disabled = true;
  elements.saveMessage.textContent = "正在请求容器更新…";
  try {
    await request("/api/update", { method: "POST" });
    elements.saveMessage.textContent = "更新已触发，容器可能短暂离线并自动重启";
    let attempts = 0;
    const timer = setInterval(async () => {
      attempts += 1;
      try {
        await loadStatus();
        if (state.snapshot.version !== previousVersion) {
          elements.saveMessage.textContent = `更新完成，当前版本 ${state.snapshot.version}`;
          clearInterval(timer);
          elements.updateButton.disabled = false;
        }
      } catch (_) {
        // The container is expected to disconnect while being recreated.
      }
      if (attempts >= 40) {
        clearInterval(timer);
        elements.saveMessage.textContent = "更新请求已完成；如版本未变化，说明当前已是最新镜像。";
        elements.updateButton.disabled = false;
      }
    }, 3000);
  } catch (error) {
    elements.saveMessage.textContent = error.message;
    elements.updateButton.disabled = false;
  }
}

[elements.setupPassword, elements.setupConfirm, elements.loginPassword,
  elements.currentPassword, elements.newPassword, elements.newPasswordConfirm]
  .forEach((input) => input.addEventListener("input", () => {
    input.value = input.value.replace(/\D/g, "").slice(0, 8);
  }));

elements.unlockButton.addEventListener("click", () => openAuthModal("login"));
elements.logoutButton.addEventListener("click", logout);
elements.authCloseButton.addEventListener("click", closeAuthModal);
elements.authModal.addEventListener("click", (event) => {
  if (event.target === elements.authModal) closeAuthModal();
});
elements.setupForm.addEventListener("submit", setupPassword);
elements.loginForm.addEventListener("submit", login);
elements.changePasswordForm.addEventListener("submit", changePassword);
elements.passwordButton.addEventListener("click", () => openAuthModal("change"));
elements.updateButton.addEventListener("click", requestUpdate);
elements.runButton.addEventListener("click", runNow);
elements.refreshButton.addEventListener("click", () => loadStatus().catch((error) => {
  elements.saveMessage.textContent = error.message;
}));
elements.configForm.addEventListener("submit", saveConfig);

setAdminEnabled(false);
Promise.all([loadStatus(), loadConfig(), refreshAuthStatus()]).catch((error) => {
  elements.saveMessage.textContent = error.message;
});
setInterval(() => loadStatus().catch(() => {}), 5000);
setInterval(() => refreshAuthStatus().catch(() => {}), 30000);
setInterval(updateCountdown, 1000);
