import { api, clearSession, logoutSession, setSession } from "./api.js";
import { showGrades } from "./grades.js";
import { showBehavior } from "./behavior.js";
import { showAdmin } from "./admin.js";
import { escapeHTML, notice, renderPage } from "./ui.js";

let user = null;
let pendingGradeContext = null;
window.currentUser = null;

const roleNames = {
  student: "Ученик",
  teacher: "Учитель",
  class_teacher: "Классный руководитель",
  admin: "Администратор школы",
};

const gradeTypeNames = {
  regular: "Текущая оценка",
  homework: "Домашняя работа",
  classwork: "Работа на уроке",
  test: "Контрольная работа",
  quiz: "Самостоятельная работа",
  exam: "Экзамен",
};

function roleLabel(role) {
  return roleNames[role] || role;
}

function updateSidebar() {
  document.querySelector("#user-name").textContent = user?.full_name || "Не выполнен вход";
  document.querySelector("#user-role").textContent = user?.roles?.map(roleLabel).join(", ") || "Гостевой режим";
  document.querySelector("#logout").hidden = !user;
  document.querySelector("#admin-nav").hidden = !user?.roles?.includes("admin");
}

window.addEventListener("session-changed", event => {
  user = event.detail;
  window.currentUser = user;
  updateSidebar();
  if (!user && ["grades", "behavior", "admin"].includes(location.hash.slice(1))) location.hash = "home";
});

function homeRole(currentUser) {
  const roles = currentUser?.roles || [];
  if (roles.includes("admin")) return "admin";
  if (roles.includes("student")) return "student";
  if (roles.includes("teacher") || roles.includes("class_teacher")) return "teacher";
  return "user";
}

function showHome(message = "") {
  if (!user) {
    renderPage("Обзор", `${message ? notice(message) : ""}${loginForm()}`);
    bindLogin();
    return;
  }

  const role = homeRole(user);
  const name = escapeHTML(user.full_name || "");
  const intro = `<div class="home-intro"><h1>${role === "student" ? `Здравствуйте, ${name}!` : role === "admin" ? "Управление школой" : "Рабочее пространство"}</h1>
    <p class="sub">${role === "student" ? "Ваши оценки и ведомость поведения." : role === "admin" ? "Настройка школы, пользователи и учебные журналы." : "Ваши классы, предметы и ведомости."}</p></div>`;
  const roleContent = `<div id="role-home-content" class="role-home-content"><p class="muted">Загружаю данные…</p></div>`;
  renderPage("Обзор", `${message ? notice(message) : ""}${intro}${roleContent}`);
  loadRoleHome(role);
}

function loadRoleHome(role) {
  const root = document.querySelector("#role-home-content");
  if (!root) return;

  if (role === "student") {
    root.innerHTML = `<div class="cards dashboard-cards">
      <section class="card dashboard-card"><div class="card-heading"><h2>Успеваемость</h2><button class="text-link" id="home-grades">Журнал</button></div><div id="student-grade-summary" class="dashboard-loading">Загружаю оценки…</div></section>
      <section class="card dashboard-card"><div class="card-heading"><h2>Последние оценки</h2></div><div id="student-recent-grades" class="dashboard-loading">Загружаю оценки…</div></section>
      <section class="card dashboard-card"><div class="card-heading"><h2>Ведомость поведения</h2><button class="text-link" id="home-behavior">Открыть</button></div><div id="student-behavior-summary" class="dashboard-loading">Загружаю ведомость…</div></section>
    </div>`;
    bindHomeNavigation();
    loadStudentDashboard(root);
    return;
  }

  if (role === "teacher") {
    root.innerHTML = `<div class="cards dashboard-cards">
      <section class="card dashboard-card dashboard-shortcut"><div class="muted">Оценки и сводки</div><h2>Журнал</h2><p class="muted">Выберите назначенный класс, предмет и период.</p><button class="btn" id="home-grades">Открыть журнал</button></section>
      <section class="card dashboard-card dashboard-shortcut"><div class="muted">Критерии и итог</div><h2>Ведомость поведения</h2><p class="muted">Заполняйте ведомость для классов с выданным правом.</p><button class="btn secondary" id="home-behavior">Открыть ведомость</button></section>
    </div><section class="card context-card"><div class="card-heading"><h2>Мои классы и предметы</h2><span id="teacher-context-count" class="muted"></span></div><div id="teacher-contexts" class="context-list dashboard-loading">Загружаю назначения…</div></section>`;
    bindHomeNavigation();
    loadTeacherDashboard(root);
    return;
  }

  if (role === "admin") {
    root.innerHTML = `<div class="cards dashboard-cards">
      <section class="card dashboard-shortcut"><div class="muted">Учебный процесс</div><h2>Настройка школы</h2><p class="muted">Учебный год, классы, предметы, назначения и ведомости.</p><button class="btn" id="home-admin">Управление школой</button></section>
      <section class="card dashboard-shortcut"><div class="muted">Оценки</div><h2>Журнал</h2><p class="muted">Просмотр оценок по классам, предметам и периодам.</p><button class="btn secondary" id="home-grades">Открыть журнал</button></section>
      <section class="card dashboard-shortcut"><div class="muted">Баллы и критерии</div><h2>Ведомость поведения</h2><p class="muted">Итоговая оценка рассчитывается по 10 критериям.</p><button class="btn secondary" id="home-behavior">Открыть ведомость</button></section>
    </div>`;
    bindHomeNavigation();
    document.querySelector("#home-admin")?.addEventListener("click", () => navigate("admin"));
    return;
  }

  root.innerHTML = `<section class="card"><h2>${escapeHTML(user.full_name || "Профиль")}</h2><p class="muted">${escapeHTML(user.email || "")}</p><p>${(user.roles || []).map(role => escapeHTML(roleLabel(role))).join(", ")}</p></section>`;
}

function bindHomeNavigation() {
  document.querySelector("#home-grades")?.addEventListener("click", () => navigate("grades"));
  document.querySelector("#home-behavior")?.addEventListener("click", () => navigate("behavior"));
}

async function loadStudentDashboard(root) {
  const gradeSummary = root.querySelector("#student-grade-summary");
  const recentGrades = root.querySelector("#student-recent-grades");
  const behaviorSummary = root.querySelector("#student-behavior-summary");

  try {
    const contexts = await api("/grades/contexts");
    if (!contexts.length) {
      gradeSummary.innerHTML = `<p class="empty">Пока нет доступных периодов.</p>`;
      recentGrades.innerHTML = `<p class="empty">Оценки появятся здесь после их внесения.</p>`;
    } else {
      const groups = await Promise.all(contexts.slice(-12).map(async context => {
        const params = new URLSearchParams({ class_id: context.class_id, term_id: context.term_id });
        const grades = await api(`/grades?${params}`);
        return { context, grades };
      }));
      const allGrades = groups.flatMap(group => group.grades.map(grade => ({ ...grade, class_name: group.context.class_name, term_name: group.context.term_name })));
      const latestGroup = [...groups].filter(group => group.grades.length).sort((a, b) => {
        const aDate = a.grades.reduce((latest, grade) => grade.work_date > latest ? grade.work_date : latest, "");
        const bDate = b.grades.reduce((latest, grade) => grade.work_date > latest ? grade.work_date : latest, "");
        return aDate.localeCompare(bDate);
      }).at(-1);

      if (latestGroup) {
        const average = latestGroup.grades.reduce((total, grade) => total + Number(grade.value), 0) / latestGroup.grades.length;
        gradeSummary.innerHTML = `<div class="metric-value">${average.toLocaleString("ru-RU", { minimumFractionDigits: 1, maximumFractionDigits: 1 })}<span> / 5</span></div>
          <div class="muted">Средний балл · ${latestGroup.grades.length} ${plural(latestGroup.grades.length, "оценка", "оценки", "оценок")}</div>
          <p class="context-label">${escapeHTML(latestGroup.context.class_name)} · ${escapeHTML(latestGroup.context.term_name)}</p>`;
      } else {
        gradeSummary.innerHTML = `<p class="empty">В доступных периодах оценок пока нет.</p>`;
      }

      const recent = allGrades.sort((a, b) => b.work_date.localeCompare(a.work_date)).slice(0, 6);
      recentGrades.innerHTML = recent.length ? `<ul class="recent-list">${recent.map(grade => `<li><div><b>${escapeHTML(grade.subject_name || grade.class_name)}</b><span class="muted">${escapeHTML(grade.class_name)} · ${escapeHTML(formatDate(grade.work_date))} · ${escapeHTML(gradeTypeNames[grade.grade_type] || grade.grade_type)}</span></div><span class="grade-mark grade-${Number(grade.value)}">${escapeHTML(grade.value)}</span></li>`).join("")}</ul>` : `<p class="empty">Оценки появятся здесь после их внесения.</p>`;
    }
  } catch (error) {
    gradeSummary.innerHTML = `<p class="error">${escapeHTML(error.message)}</p>`;
    recentGrades.innerHTML = `<p class="muted">Не удалось загрузить список оценок.</p>`;
  }

  try {
    const contexts = await api("/behavior/contexts");
    if (!contexts.length) {
      behaviorSummary.innerHTML = `<p class="empty">Пока нет доступных периодов.</p>${behaviorScale()}`;
      return;
    }
    const groups = await Promise.all(contexts.slice(-12).map(async (context, index) => {
      const params = new URLSearchParams({ class_id: context.class_id, term_id: context.term_id });
      return { index, context, sheets: await api(`/behavior/sheets?${params}`) };
    }));
    const sheets = groups.flatMap(group => group.sheets.map(sheet => ({ ...sheet, context: group.context, contextIndex: group.index })));
    const sheet = [...sheets].sort((a, b) => b.contextIndex - a.contextIndex)[0];
    if (!sheet) {
      behaviorSummary.innerHTML = `<p class="empty">Ведомость для вас пока не создана.</p>${behaviorScale()}`;
      return;
    }
    const result = sheet.is_complete
      ? `<div class="metric-value">${escapeHTML(sheet.total_score)}<span> / 30</span></div><span class="status ${sheet.total_score >= 24 ? "green" : sheet.total_score >= 10 ? "yellow" : "red"}">${escapeHTML(verdictLabel(sheet.verdict))}</span>`
      : `<div class="metric-value">${escapeHTML(sheet.scores_count)}<span> / 10</span></div><span class="status neutral">Заполнено критериев</span>`;
    behaviorSummary.innerHTML = `${result}<p class="context-label">${escapeHTML(sheet.context.class_name)} · ${escapeHTML(sheet.context.term_name)}${sheet.project ? ` · Проект: ${escapeHTML(sheet.project)}` : ""}</p>${behaviorScale()}`;
  } catch (error) {
    behaviorSummary.innerHTML = `<p class="error">${escapeHTML(error.message)}</p>${behaviorScale()}`;
  }
}

function behaviorScale() {
  return `<p class="rubric-note">10 критериев по 0–3 балла. 24–30 — образцовое поведение, 10–23 — допустимое, 0–9 — недопустимое.</p>`;
}

function verdictLabel(value) {
  const labels = { образцовое: "Образцовое поведение", допустимое: "Допустимое поведение", недопустимое: "Недопустимое поведение" };
  return labels[value] || value || "Оценка не выставлена";
}

async function loadTeacherDashboard(root) {
  const output = root.querySelector("#teacher-contexts");
  const count = root.querySelector("#teacher-context-count");
  try {
    const contexts = await api("/grades/contexts");
    count.textContent = `${contexts.length} ${plural(contexts.length, "доступный журнал", "доступных журнала", "доступных журналов")}`;
    if (!contexts.length) {
      output.innerHTML = `<p class="empty">Назначения классов и предметов пока не настроены.</p>`;
      return;
    }
    output.classList.remove("dashboard-loading");
    output.innerHTML = contexts.map((context, index) => `<div class="context-item"><div><b>${escapeHTML(context.class_name)}${context.subject_name ? ` · ${escapeHTML(context.subject_name)}` : ""}</b><span class="muted">${escapeHTML(context.term_name)} · ${context.can_edit ? "можно выставлять оценки" : "только просмотр"}</span></div><button class="btn secondary" data-open-context="${index}">${context.can_edit ? "Открыть журнал" : "Открыть сводку"}</button></div>`).join("");
    output.querySelectorAll("[data-open-context]").forEach(button => button.addEventListener("click", () => navigate("grades", contexts[Number(button.dataset.openContext)])));
  } catch (error) {
    output.innerHTML = `<p class="error">${escapeHTML(error.message)}</p>`;
  }
}

function plural(value, one, few, many) {
  const lastTwo = value % 100;
  const last = value % 10;
  if (lastTwo >= 11 && lastTwo <= 14) return many;
  if (last === 1) return one;
  if (last >= 2 && last <= 4) return few;
  return many;
}

function formatDate(value) {
  const date = new Date(`${value}T00:00:00`);
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "short" }).format(date);
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
      const data = await api("/invites/accept", { method: "POST", body: JSON.stringify({ token: form.get("token"), full_name: form.get("full_name"), password: form.get("password") }) });
      setSession(data);
      const cleanURL = new URL(location.href);
      cleanURL.searchParams.delete("invite");
      history.replaceState(null, "", cleanURL);
      showHome("Аккаунт активирован. Вы вошли в систему.");
    } catch (error) { document.querySelector("#invite-error").textContent = error.message; }
  });
}

async function navigate(route, selectedContext = null) {
  if (selectedContext) pendingGradeContext = selectedContext;
  const currentRoute = (location.hash.slice(1) || "home").split("?")[0];
  if (currentRoute !== route) {
    location.hash = route;
    return;
  }
  if (!user && route !== "home") {
    location.hash = "home";
    showHome("Сначала выполните вход.");
    return;
  }
  if (route === "grades") {
    const context = pendingGradeContext;
    pendingGradeContext = null;
    await showGrades(user, context);
  } else if (route === "behavior") await showBehavior(user);
  else if (route === "admin" && user?.roles.includes("admin")) await showAdmin();
  else showHome();
}

document.querySelectorAll(".nav").forEach(button => button.addEventListener("click", () => navigate(button.dataset.page)));
window.addEventListener("hashchange", () => navigate(location.hash.slice(1).split("?")[0] || "home"));
document.querySelector("#logout").addEventListener("click", async () => {
  try { await logoutSession(); } catch {}
  clearSession();
  location.hash = "home";
  showHome("Вы вышли из системы.");
});

updateSidebar();
const initialRoute = location.hash.slice(1).split("?")[0] || "home";
if (new URLSearchParams(location.search).has("invite")) {
  location.hash = "home";
  showHome();
} else {
  navigate(initialRoute);
}
