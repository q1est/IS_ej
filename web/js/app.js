import { api, clearSession, logoutSession, setSession } from "./api.js";
import { showGrades } from "./grades.js";
import { showBehavior } from "./behavior.js";
import { showAdmin } from "./admin.js";
import { escapeHTML, notice, renderPage } from "./ui.js";

let user = null;
window.currentUser = null;

function updateSidebar() {
  document.querySelector("#user-name").textContent = user?.full_name || "Не выполнен вход";
  document.querySelector("#user-role").textContent = user?.roles?.join(", ") || "Гостевой режим";
  document.querySelector("#logout").hidden = !user;
  document.querySelector("#admin-nav").hidden = !user?.roles?.includes("admin");
}

window.addEventListener("session-changed", event => {
  user = event.detail;
  window.currentUser = user;
  updateSidebar();
  if (!user && ["grades", "behavior", "admin"].includes(location.hash.slice(1))) location.hash = "home";
});

function showHome(message = "") {
  renderPage("Обзор", `<h1>Школьный журнал</h1><p class="sub">Оценки и ведомость поведения доступны как отдельные разделы.</p>
    ${message ? notice(message) : ""}${user ? `<div class="cards"><div class="card"><div class="muted">Пользователь</div><div class="big">${escapeHTML(user.full_name)}</div><div class="muted">${escapeHTML(user.email)}</div></div>
      <div class="card"><div class="muted">Роли</div><div class="big">${user.roles.map(escapeHTML).join(", ")}</div></div><div class="card"><div class="muted">Разделы</div>
      <p><button class="btn" id="home-grades">Открыть журнал</button> <button class="btn secondary" id="home-behavior">Ведомость поведения</button></p>
      ${user.roles.includes("admin") ? `<button class="btn secondary" id="home-admin">Управление школой</button>` : ""}</div></div>` : loginForm()}`);
  if (!user) bindLogin();
  document.querySelector("#home-grades")?.addEventListener("click", () => navigate("grades"));
  document.querySelector("#home-behavior")?.addEventListener("click", () => navigate("behavior"));
  document.querySelector("#home-admin")?.addEventListener("click", () => navigate("admin"));
}

function loginForm() {
  const token = new URLSearchParams(location.search).get("invite") || "";
  return `<div class="card login"><h2>Вход в систему</h2><form id="login-form">
    <label class="field">Email<input name="email" type="email" required autocomplete="username"></label>
    <label class="field">Пароль<input name="password" type="password" required autocomplete="current-password"></label>
    <label class="field">ID школы <span class="muted">(только если одинаковый email есть в нескольких школах)</span><input name="school_id"></label>
    <p id="login-error" class="error"></p><button class="btn">Войти</button></form>
    <p class="muted">Регистрация открыта только по персональному приглашению.</p>
    <button class="btn secondary" id="show-invite">Принять приглашение</button>
    <div id="invite-slot">${token ? inviteForm(token) : ""}</div></div>`;
}

function bindLogin() {
  document.querySelector("#login-form")?.addEventListener("submit", async event => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    try {
      const data = await api("/auth/login", { method: "POST", body: JSON.stringify({ email: form.get("email"), password: form.get("password"), school_id: form.get("school_id") }) });
      setSession(data);
      showHome();
    } catch (error) { document.querySelector("#login-error").textContent = error.message; }
  });
  document.querySelector("#show-invite")?.addEventListener("click", () => {
    const slot = document.querySelector("#invite-slot");
    slot.innerHTML = slot.innerHTML ? "" : inviteForm(new URLSearchParams(location.search).get("invite") || "");
    bindInvite();
  });
  bindInvite();
}

function inviteForm(token = "") {
  return `<form id="invite-form" class="invite-form"><h3>Активация аккаунта</h3>
    <label class="field">Токен приглашения<input name="token" value="${escapeHTML(token)}" required autocomplete="one-time-code"></label>
    <label class="field">ФИО<input name="full_name" required autocomplete="name"></label>
    <label class="field">Пароль (не менее 12 символов)<input name="password" type="password" minlength="12" required autocomplete="new-password"></label>
    <label class="field">Повтор пароля<input name="confirm_password" type="password" minlength="12" required autocomplete="new-password"></label>
    <p id="invite-error" class="error"></p><button class="btn">Активировать аккаунт</button></form>`;
}

function bindInvite() {
  document.querySelector("#invite-form")?.addEventListener("submit", async event => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    if (form.get("password") !== form.get("confirm_password")) {
      document.querySelector("#invite-error").textContent = "Пароли не совпадают.";
      return;
    }
    try {
      await api("/invites/accept", { method: "POST", body: JSON.stringify({ token: form.get("token"), full_name: form.get("full_name"), password: form.get("password") }) });
      document.querySelector("#invite-error").textContent = "Аккаунт активирован. Теперь войдите по email и паролю.";
      document.querySelector("#invite-error").className = "success";
    } catch (error) { document.querySelector("#invite-error").textContent = error.message; }
  });
}

async function navigate(route) {
  if (location.hash.slice(1) !== route) location.hash = route;
  if (!user && route !== "home") {
    location.hash = "home";
    showHome("Сначала выполните вход.");
    return;
  }
  if (route === "grades") await showGrades(user);
  else if (route === "behavior") await showBehavior(user);
  else if (route === "admin" && user?.roles.includes("admin")) await showAdmin();
  else showHome();
}

document.querySelectorAll(".nav").forEach(button => button.addEventListener("click", () => navigate(button.dataset.page)));
window.addEventListener("hashchange", () => navigate(location.hash.slice(1) || "home"));
document.querySelector("#logout").addEventListener("click", async () => {
	try { await logoutSession(); } catch {}
  clearSession();
  location.hash = "home";
  showHome("Вы вышли из системы.");
});

updateSidebar();
const initialRoute = location.hash.slice(1) || "home";
if (new URLSearchParams(location.search).has("invite")) {
  location.hash = "home";
  showHome();
} else {
  navigate(initialRoute);
}
