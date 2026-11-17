// An independent dialog preserves the current route and unsaved form values.
export function installServerStatus({ retry }) {
  const dialog = document.createElement("dialog");
  dialog.id = "server-status";
  dialog.className = "server-status";
  dialog.setAttribute("aria-labelledby", "server-status-title");
  dialog.setAttribute("aria-describedby", "server-status-message");
  dialog.innerHTML = `<div class="server-status-icon" aria-hidden="true"><span></span></div><p class="server-status-label">ШКОЛЬНЫЙ ЖУРНАЛ</p><h1 id="server-status-title">Подключаемся к серверу</h1><p id="server-status-message">Если сервер спит, первый запрос запустит его. Обычно нужно немного подождать.</p><p id="server-status-progress" class="muted" role="status" aria-live="polite"></p><button type="button" class="btn" id="server-status-retry" hidden>Повторить подключение</button>`;
  document.body.append(dialog);
  const button = dialog.querySelector("button");
  const title = dialog.querySelector("h1");
  const message = dialog.querySelector("#server-status-message");
  const progress = dialog.querySelector("#server-status-progress");
  let showTimer = null;
  let ticker = null;
  let waiting = false;
  let activeRun = null;
  const clearTimers = () => { clearTimeout(showTimer); clearInterval(ticker); showTimer = null; ticker = null; };
  const show = () => { if (!dialog.open) dialog.showModal(); };
  dialog.addEventListener("cancel", event => event.preventDefault());
  button.addEventListener("click", async () => {
    if (waiting) return;
    button.disabled = true;
    try { await retry(); } catch { /* The readiness state event renders the error. */ }
    finally { button.disabled = false; }
  });
  window.addEventListener("server-state", event => {
    const state = event.detail;
    if (state.status === "ready") {
      waiting = false;
      activeRun = null;
      clearTimers();
      if (dialog.open) dialog.close();
      return;
    }
    if (state.status === "checking") {
      waiting = true;
      dialog.classList.remove("unavailable");
      title.textContent = "Подключаемся к серверу";
      message.textContent = "Если сервер спит, первый запрос запустит его. Обычно нужно немного подождать.";
      button.hidden = true;
      const updateProgress = () => { progress.textContent = `Ожидаем готовности · осталось до ${Math.max(0, Math.ceil((state.deadlineAt - Date.now()) / 1000))} сек.`; };
      updateProgress();
      if (activeRun !== state.startedAt) {
        clearTimers();
        activeRun = state.startedAt;
        // Avoid flashing an overlay for an already running server.
        showTimer = setTimeout(show, 500);
      }
      clearInterval(ticker);
      ticker = setInterval(updateProgress, 1000);
      return;
    }
    waiting = false;
    activeRun = null;
    clearTimers();
    dialog.classList.add("unavailable");
    title.textContent = "Не удалось подключиться";
    message.textContent = state.message;
    progress.textContent = "Ваши введённые данные остаются на странице.";
    button.hidden = false;
    button.disabled = false;
    show();
  });
  window.addEventListener("pagehide", clearTimers);
}
