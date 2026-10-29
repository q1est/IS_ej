export function escapeHTML(value) {
  return String(value ?? "").replace(/[&<>"']/g, char => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  })[char]);
}

export function renderPage(title, html) {
  document.querySelector("#title").textContent = title;
  document.querySelector("#content").innerHTML = html;
  const route = location.hash.slice(1) || "home";
  document.querySelectorAll(".nav").forEach(button => button.classList.toggle("active", button.dataset.page === route));
}

export function notice(message = "Доступ к данным проверяется сервером для каждого запроса.") {
  return `<div class="notice">${escapeHTML(message)}</div>`;
}

export function errorBox(message) {
  return `<p class="error">${escapeHTML(message)}</p>`;
}

export function showNoContexts(element, user, selector = null, adminMessage = "Добавьте учебный год, класс, предмет и период в настройках школы, чтобы открыть журнал.") {
  if (selector) {
    selector.disabled = true;
    const option = document.createElement("option");
    option.textContent = "Нет доступных классов или периодов";
    selector.replaceChildren(option);
  }
  const message = document.createElement("p");
  message.className = "empty";
  message.textContent = user?.roles?.includes("admin")
    ? adminMessage
    : "Для этой учётной записи пока нет назначенных классов или периодов. Обратитесь к администратору школы.";
  element.replaceChildren(message);
  if (user?.roles?.includes("admin")) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "btn secondary";
    button.textContent = "Настроить школу";
    button.addEventListener("click", () => { location.hash = "admin"; });
    element.append(button);
  }
}

export function localDate() {
  const date = new Date();
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60000);
  return local.toISOString().slice(0, 10);
}
