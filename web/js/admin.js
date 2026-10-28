import { api } from "./api.js";
import { escapeHTML, notice, renderPage } from "./ui.js";

const schemas = {
  "academic-years": { label: "Учебный год", fields: [{ key: "name", title: "Название" }, { key: "starts_on", title: "Начало", type: "date" }, { key: "ends_on", title: "Окончание", type: "date" }, { key: "is_current", title: "Текущий", type: "checkbox" }] },
  classes: { label: "Класс", fields: [{ key: "name", title: "Название" }, ref("academic_year_id", "Учебный год", "academic-years")] },
  subjects: { label: "Предмет", fields: [{ key: "name", title: "Название" }] },
  terms: { label: "Период", fields: [{ key: "name", title: "Название" }, { key: "starts_on", title: "Начало", type: "date" }, { key: "ends_on", title: "Окончание", type: "date" }, ref("academic_year_id", "Учебный год", "academic-years")] },
  students: { label: "Ученик", fields: [{ key: "external_id", title: "Табельный номер" }, { key: "full_name", title: "ФИО" }, { key: "birth_date", title: "Дата рождения", type: "date", optional: true }] },
  enrollments: { label: "Зачисление", fields: [ref("student_id", "Ученик", "students"), ref("class_id", "Класс", "classes"), ref("academic_year_id", "Учебный год", "academic-years"), { key: "from_date", title: "Дата начала", type: "date", optional: true }] },
  "teacher-assignments": { label: "Назначение учителя", fields: [ref("teacher_id", "Учитель", "users", "teacher"), ref("class_id", "Класс", "classes"), ref("subject_id", "Предмет", "subjects"), ref("academic_year_id", "Учебный год", "academic-years")] },
  "class-teacher-assignments": { label: "Классное руководство", fields: [ref("teacher_id", "Учитель", "users", "class_teacher"), ref("class_id", "Класс", "classes"), ref("academic_year_id", "Учебный год", "academic-years")] },
  "behavior-permissions": { label: "Право оценивать поведение", fields: [ref("teacher_id", "Учитель", "users", "teacher"), ref("class_id", "Класс", "classes"), ref("academic_year_id", "Учебный год", "academic-years"), { key: "can_score", title: "Разрешить", type: "checkbox" }] },
  projects: { label: "Проект", fields: [{ key: "name", title: "Название" }] },
  "student-projects": { label: "Проект ученика", fields: [ref("student_id", "Ученик", "students"), ref("project_id", "Проект", "projects"), ref("academic_year_id", "Учебный год", "academic-years")] },
};

function ref(key, title, resource, role) { return { key, title, type: "reference", resource, role }; }

export async function showAdmin() {
  renderPage("Администрирование", `<h1>Управление школой</h1><p class="sub">Справочники, назначения, приглашения и аудит. Каждая операция проверяется сервером.</p>
    ${notice()}<div id="admin-status" class="muted">Загружаю данные…</div><div id="admin-content"></div>`);
  try {
    const resourceNames = [...new Set(Object.values(schemas).flatMap(schema => schema.fields.filter(field => field.resource && field.resource !== "users").map(field => field.resource)))];
    const resourcePromises = Object.keys(schemas).map(async resource => [resource, await api(`/admin/${resource}`)]);
    const [resourcePairs, users, invites, audit] = await Promise.all([
      Promise.all(resourcePromises), api("/admin/users"), api("/admin/invites"), api("/admin/audit"),
    ]);
    const data = Object.fromEntries(resourcePairs);
    data.users = users.map(user => ({ ...user, id: user.id }));
    data.invites = invites;
    data.audit = audit;
    renderAdmin(data);
  } catch (error) {
    document.querySelector("#admin-status").textContent = error.message;
  }
}

function renderAdmin(data) {
  const output = document.querySelector("#admin-content");
  document.querySelector("#admin-status").textContent = "";
  output.innerHTML = `<div class="admin-layout"><nav class="admin-tabs">${[["resources", "Справочники"], ["invites", "Приглашения"], ["users", "Пользователи"], ["audit", "Аудит"]].map(([key, title], index) => `<button class="admin-tab ${index === 0 ? "selected" : ""}" data-admin-tab="${key}">${title}</button>`).join("")}</nav>
    <section id="admin-panel"></section></div>`;
  output.querySelectorAll("[data-admin-tab]").forEach(button => button.addEventListener("click", () => {
    output.querySelectorAll("[data-admin-tab]").forEach(item => item.classList.toggle("selected", item === button));
    const panel = output.querySelector("#admin-panel");
    if (button.dataset.adminTab === "resources") renderResources(panel, data);
    if (button.dataset.adminTab === "invites") renderInvites(panel, data);
    if (button.dataset.adminTab === "users") renderUsers(panel, data);
    if (button.dataset.adminTab === "audit") renderAudit(panel, data);
  }));
  renderResources(output.querySelector("#admin-panel"), data);
}

function renderResources(panel, data) {
  panel.innerHTML = `<div class="toolbar"><label class="field-inline">Что добавить<select id="admin-resource">${Object.entries(schemas).map(([key, schema]) => `<option value="${key}">${schema.label}</option>`).join("")}</select></label></div><div id="resource-form"></div>
    <h2>Записи</h2><label class="field-inline">Справочник<select id="admin-list-resource">${Object.entries(schemas).map(([key, schema]) => `<option value="${key}">${schema.label}</option>`).join("")}</select></label><div id="resource-table" class="table"></div>`;
  const resource = panel.querySelector("#admin-resource");
  const list = panel.querySelector("#admin-list-resource");
  const drawForm = () => {
    const name = resource.value;
    const schema = schemas[name];
    panel.querySelector("#resource-form").innerHTML = `<form id="resource-create" class="card form-grid">${schema.fields.map(field => renderField(field, data)).join("")}<button class="btn">Добавить ${schema.label.toLowerCase()}</button><span id="resource-error" class="error"></span></form>`;
    panel.querySelector("#resource-create").addEventListener("submit", async event => {
      event.preventDefault();
      const body = {};
      for (const field of schema.fields) {
        const input = event.currentTarget.elements[field.key];
        if (field.type === "checkbox") body[field.key] = input.checked;
        else if (input.value || !field.optional) body[field.key] = input.value;
      }
      try {
        await api(`/admin/${name}`, { method: "POST", body: JSON.stringify(body) });
        await showAdmin();
      } catch (error) { panel.querySelector("#resource-error").textContent = error.message; }
    });
  };
  const drawList = () => drawResourceList(panel.querySelector("#resource-table"), data[list.value]);
  resource.addEventListener("change", drawForm);
  list.addEventListener("change", drawList);
  drawForm();
  drawList();
}

function renderField(field, data) {
  if (field.type === "checkbox") return `<label class="field"><span>${field.title}</span><input name="${field.key}" type="checkbox"></label>`;
  if (field.type === "reference") {
    const options = (data[field.resource] || []).filter(item => !field.role || item.roles?.split(", ").includes(field.role));
    const label = item => item.full_name || item.name || item.email || item.id;
    return `<label class="field">${field.title}<select name="${field.key}" ${field.optional ? "" : "required"}><option value="">Выберите…</option>${options.map(item => `<option value="${escapeHTML(item.id)}">${escapeHTML(label(item))}</option>`).join("")}</select></label>`;
  }
  return `<label class="field">${field.title}<input name="${field.key}" type="${field.type || "text"}" ${field.optional ? "" : "required"}></label>`;
}

function drawResourceList(target, rows) {
  if (!rows?.length) { target.innerHTML = `<p class="empty">Записей пока нет.</p>`; return; }
  const columns = Object.keys(rows[0]);
  target.innerHTML = `<table><thead><tr>${columns.map(key => `<th>${escapeHTML(key)}</th>`).join("")}</tr></thead><tbody>${rows.map(row => `<tr>${columns.map(key => `<td>${escapeHTML(row[key] ?? "—")}</td>`).join("")}</tr>`).join("")}</tbody></table>`;
}

function renderInvites(panel, data) {
  const students = data.students || [];
  const classes = data.classes || [];
  const years = data["academic-years"] || [];
  panel.innerHTML = `<div class="card"><h2>Новое приглашение</h2><form id="invite-form" class="form-grid">
    <label class="field">Email<input name="email" type="email" required></label>
    <label class="field">Роль<select name="role"><option value="student">Ученик</option><option value="teacher">Учитель</option><option value="class_teacher">Классный руководитель</option><option value="admin">Администратор</option></select></label>
    <label class="field" id="invite-target-wrap">Ученик<select name="target_id"><option value="">Выберите ученика</option>${students.map(item => `<option value="${escapeHTML(item.id)}">${escapeHTML(item.full_name)} (${escapeHTML(item.external_id)})</option>`).join("")}</select></label>
    <label class="field">Учебный год<select name="academic_year_id"><option value="">Без привязки</option>${years.map(item => `<option value="${escapeHTML(item.id)}">${escapeHTML(item.name)}</option>`).join("")}</select></label>
    <div><button class="btn">Создать одноразовое приглашение</button><span id="invite-error" class="error"></span></div>
  </form><p class="muted">Скопируйте выданный токен и передайте его только нужному человеку.</p><div id="invite-created"></div></div>
    <h2>Приглашения</h2><div class="table"><table><thead><tr><th>Email</th><th>Роль</th><th>Тип</th><th>Срок</th><th>Использовано</th><th></th></tr></thead><tbody>${data.invites.map(invite => `<tr><td>${escapeHTML(invite.email)}</td><td>${escapeHTML(invite.role)}</td><td>${escapeHTML(invite.target_type)}</td><td>${escapeHTML(invite.expires_at)}</td><td>${escapeHTML(invite.used_at || "—")}</td><td>${invite.used_at ? "" : `<button class="btn danger" data-revoke-invite="${escapeHTML(invite.id)}">Отозвать</button>`}</td></tr>`).join("")}</tbody></table></div>`;
  const role = panel.querySelector('[name="role"]');
  const targetWrap = panel.querySelector("#invite-target-wrap");
  const target = panel.querySelector('[name="target_id"]');
  const updateTarget = () => {
    if (role.value === "student") {
      targetWrap.hidden = false;
      targetWrap.firstChild.textContent = "Ученик";
      target.innerHTML = `<option value="">Выберите ученика</option>${students.map(item => `<option value="${escapeHTML(item.id)}">${escapeHTML(item.full_name)} (${escapeHTML(item.external_id)})</option>`).join("")}`;
      target.required = true;
    } else if (role.value === "class_teacher") {
      targetWrap.hidden = false;
      targetWrap.firstChild.textContent = "Классное руководство (права назначаются отдельно)";
      target.innerHTML = `<option value="">Выберите класс</option>${classes.map(item => `<option value="${escapeHTML(item.id)}">${escapeHTML(item.name)}</option>`).join("")}`;
      target.required = false;
    } else {
      targetWrap.hidden = true;
      target.innerHTML = "<option value=\"\"></option>";
      target.required = false;
    }
  };
  role.addEventListener("change", updateTarget);
  updateTarget();
  panel.querySelector("#invite-form").addEventListener("submit", async event => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const targetID = form.get("target_id") || "";
    const yearID = form.get("academic_year_id");
    try {
      const inviteRole = form.get("role");
      const targetType = inviteRole === "student" ? "student" : inviteRole === "class_teacher" && targetID ? "class" : "";
      const result = await api("/invites", { method: "POST", body: JSON.stringify({ email: form.get("email"), role: inviteRole, target_type: targetType, target_id: targetID, academic_year_id: yearID }) });
      const inviteURL = new URL(location.href);
      inviteURL.hash = "home";
      inviteURL.search = new URLSearchParams({ invite: result.token }).toString();
      panel.querySelector("#invite-created").innerHTML = `<div class="notice">Передайте персональную ссылку и токен по защищённому каналу. Токен виден только сейчас.<br><a href="${escapeHTML(inviteURL.toString())}">${escapeHTML(inviteURL.toString())}</a><br><code>${escapeHTML(result.token)}</code></div>`;
    } catch (error) { panel.querySelector("#invite-error").textContent = error.message; }
  });
  panel.querySelectorAll("[data-revoke-invite]").forEach(button => button.addEventListener("click", async () => {
    if (!confirm("Отозвать это приглашение?")) return;
    try { await api(`/admin/invites/${encodeURIComponent(button.dataset.revokeInvite)}/revoke`, { method: "POST", body: "{}" }); await showAdmin(); }
    catch (error) { alert(error.message); }
  }));
}

function renderUsers(panel, data) {
  panel.innerHTML = `<h2>Пользователи школы</h2><div class="table"><table><thead><tr><th>ФИО</th><th>Email</th><th>Роли</th><th>Статус</th><th>Действие</th></tr></thead><tbody>${data.users.map(user => `<tr><td>${escapeHTML(user.full_name)}</td><td>${escapeHTML(user.email)}</td><td>${escapeHTML(user.roles)}</td><td>${escapeHTML(user.status)}</td><td><button class="btn danger" data-revoke-user="${escapeHTML(user.id)}">Завершить сессии</button></td></tr>`).join("")}</tbody></table></div>`;
  panel.querySelectorAll("[data-revoke-user]").forEach(button => button.addEventListener("click", async () => {
    if (!confirm("Завершить все refresh-сессии этого пользователя?")) return;
    try { await api(`/admin/users/${encodeURIComponent(button.dataset.revokeUser)}/revoke-sessions`, { method: "POST", body: "{}" }); await showAdmin(); }
    catch (error) { alert(error.message); }
  }));
}

function renderAudit(panel, data) {
  const rows = data.audit || [];
  panel.innerHTML = `<h2>Последние изменения</h2><div class="table">${tableRows(rows, ["created_at", "actor_user_id", "action", "entity", "entity_id", "old_value", "new_value"], true)}</div>`;
}

function tableRows(rows, keys, includeHeader = false) {
  if (!rows?.length) return `<p class="empty">Записей пока нет.</p>`;
  return `<table>${includeHeader ? `<thead><tr>${keys.map(key => `<th>${escapeHTML(key)}</th>`).join("")}</tr></thead>` : ""}<tbody>${rows.map(row => `<tr>${keys.map(key => `<td>${escapeHTML(typeof row[key] === "object" ? JSON.stringify(row[key]) : row[key])}</td>`).join("")}</tr>`).join("")}</tbody></table>`;
}
