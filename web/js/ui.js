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

export function localDate() {
  const date = new Date();
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60000);
  return local.toISOString().slice(0, 10);
}
