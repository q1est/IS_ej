import { api, apiBlob, saveBlob } from "./api.js";
import { escapeHTML, notice, renderPage } from "./ui.js";

const schemas = {
  "academic-years": { label: "Учебный год", fields: [{ key: "name", title: "Название" }, { key: "starts_on", title: "Начало", type: "date" }, { key: "ends_on", title: "Окончание", type: "date" }, { key: "is_current", title: "Текущий", type: "checkbox" }] },
  classes: { label: "Класс", fields: [{ key: "name", title: "Название" }, ref("academic_year_id", "Учебный год", "academic-years")] },
  subjects: { label: "Предмет", fields: [{ key: "name", title: "Название" }] },
  terms: { label: "Период", fields: [{ key: "name", title: "Название" }, { key: "starts_on", title: "Начало", type: "date" }, { key: "ends_on", title: "Окончание", type: "date" }, ref("academic_year_id", "Учебный год", "academic-years")] },
  students: { label: "Ученик", fields: [{ key: "external_id", title: "Табельный номер" }, { key: "full_name", title: "ФИО" }, { key: "birth_date", title: "Дата рождения", type: "date", optional: true }, { key: "class_id", title: "Класс для зачисления", type: "reference", resource: "classes", optional: true }] },
  enrollments: { label: "Зачисление", fields: [ref("student_id", "Ученик", "students"), ref("class_id", "Класс", "classes"), ref("academic_year_id", "Учебный год", "academic-years"), { key: "from_date", title: "Дата начала", type: "date", optional: true }] },
  "teacher-assignments": { label: "Назначение учителя", fields: [ref("teacher_id", "Учитель", "users", "teacher"), ref("class_id", "Класс", "classes"), ref("subject_id", "Предмет", "subjects"), ref("academic_year_id", "Учебный год", "academic-years")] },
  "class-teacher-assignments": { label: "Классное руководство", fields: [ref("teacher_id", "Учитель", "users", "class_teacher"), ref("class_id", "Класс", "classes"), ref("academic_year_id", "Учебный год", "academic-years")] },
  "behavior-permissions": { label: "Право оценивать поведение", fields: [ref("teacher_id", "Учитель", "users", "teacher"), ref("class_id", "Класс", "classes"), ref("academic_year_id", "Учебный год", "academic-years"), { key: "can_score", title: "Разрешить", type: "checkbox" }] },
  projects: { label: "Проект", fields: [{ key: "name", title: "Название" }] },
  "student-projects": { label: "Проект ученика", fields: [ref("student_id", "Ученик", "students"), ref("project_id", "Проект", "projects"), ref("academic_year_id", "Учебный год", "academic-years")] },
};

function ref(key, title, resource, role) { return { key, title, type: "reference", resource, role }; }

export async function showAdmin(state = {}) {
  renderPage("Администрирование", `<h1>Управление школой</h1><p class="sub">Справочники, назначения, приглашения и аудит. Каждая операция проверяется сервером.</p>
    ${notice()}<div id="admin-status" class="muted">Загружаю данные…</div><div id="admin-content"></div>`);
  const requests = [
    ...Object.keys(schemas).map(resource => [`resource:${resource}`, `/admin/${resource}`]),
    ["users", "/admin/users"], ["invites", "/admin/invites"], ["audit", "/admin/audit"],
  ];
  const results = await Promise.allSettled(requests.map(([, path]) => api(path)));
  const data = {};
  const errors = {};
  results.forEach((result, index) => {
    const [key] = requests[index];
    const name = key.startsWith("resource:") ? key.slice("resource:".length) : key;
    if (result.status === "fulfilled") data[name] = result.value;
    else { data[name] = []; errors[name] = result.reason?.message || "Не удалось загрузить данные"; }
  });
  data.users = (data.users || []).map(user => ({ ...user, id: user.id }));
  data.errors = errors;
  renderAdmin(data, state);
  const failedCount = Object.keys(errors).length;
  if (failedCount) {
    const status = document.querySelector("#admin-status");
    status.className = "error";
    status.textContent = `Не удалось загрузить разделов: ${failedCount}. Остальные данные доступны; откройте вкладку раздела, чтобы увидеть ошибку.`;
    const retry = document.createElement("button");
    retry.type = "button";
    retry.className = "btn secondary";
    retry.textContent = "Обновить все данные";
    retry.addEventListener("click", () => showAdmin(state));
    status.append(" ", retry);
  }
}
function renderAdmin(data, state = {}) {
  const output = document.querySelector("#admin-content");
  const status = document.querySelector("#admin-status");
  status.className = state.message ? "success" : "muted";
  status.textContent = state.message || "";
  output.innerHTML = `<div class="admin-layout"><nav class="admin-tabs">${[["resources", "Справочники"], ["invites", "Приглашения"], ["users", "Пользователи"], ["audit", "Аудит"]].map(([key, title], index) => `<button class="admin-tab ${index === 0 ? "selected" : ""}" data-admin-tab="${key}">${title}</button>`).join("")}</nav>
    <section id="admin-panel"></section></div>`;
  output.querySelectorAll("[data-admin-tab]").forEach(button => button.addEventListener("click", () => {
    output.querySelectorAll("[data-admin-tab]").forEach(item => item.classList.toggle("selected", item === button));
    const panel = output.querySelector("#admin-panel");
    const tab = button.dataset.adminTab;
    if (tab === "resources") renderResources(panel, data, state);
    if (tab === "invites") renderInvites(panel, data);
    if (tab === "users") renderUsers(panel, data);
    if (tab === "audit") renderAudit(panel, data);
  }));
  renderResources(output.querySelector("#admin-panel"), data, state);
}
function renderResources(panel, data, state = {}) {
  panel.innerHTML = `<section class="card excel-tools"><div><h2>Данные Excel</h2><p class="muted">Скачайте книгу школы, отредактируйте листы и загрузите её обратно. Импорт обновляет или добавляет записи, ограничен 1 000 строками и 20 МБ. Пустые строки пропускаются; ошибка отменяет весь импорт. В ведомости поведения пустая ячейка не меняет балл, для удаления укажите «УДАЛИТЬ».</p></div>
      <div class="toolbar"><button type="button" class="btn secondary" data-excel-download>Экспорт Excel</button><button type="button" class="btn" data-excel-select>Импорт Excel</button></div>
      <input id="excel-file" type="file" accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" hidden><p id="excel-status" class="muted" aria-live="polite"></p></section>
    <div class="toolbar"><label class="field-inline">Что добавить<select id="admin-resource">${Object.entries(schemas).map(([key, schema]) => `<option value="${key}">${schema.label}</option>`).join("")}</select></label></div><div id="resource-form"></div>
    <h2>Записи</h2><label class="field-inline">Справочник<select id="admin-list-resource">${Object.entries(schemas).map(([key, schema]) => `<option value="${key}">${schema.label}</option>`).join("")}</select></label><div id="resource-table" class="table"></div>
    <div class="toolbar"><button type="button" class="btn secondary" data-excel-download>Экспорт Excel</button><button type="button" class="btn" data-excel-select>Импорт Excel</button></div>`;
  const resource = panel.querySelector("#admin-resource");
  const list = panel.querySelector("#admin-list-resource");
  if (schemas[state.resource]) resource.value = state.resource;
  if (schemas[state.list]) list.value = state.list;
  const drawForm = () => {
    const name = resource.value;
    const schema = schemas[name];
    panel.querySelector("#resource-form").innerHTML = `<form id="resource-create" class="card form-grid">${schema.fields.map(field => renderField(field, data)).join("")}<button type="submit" class="btn">Добавить ${schema.label.toLowerCase()}</button><span id="resource-error" class="error"></span></form>`;
    panel.querySelector("#resource-create").addEventListener("submit", async event => {
      event.preventDefault();
      const submit = event.currentTarget.querySelector("button[type=submit]");
      submit.disabled = true;
      submit.textContent = "Сохраняю…";
      const body = {};
      for (const field of schema.fields) {
        const input = event.currentTarget.elements[field.key];
        if (field.type === "checkbox") body[field.key] = input.checked;
        else if (input.value || !field.optional) body[field.key] = input.value;
      }
      try {
        await api(`/admin/${name}`, { method: "POST", body: JSON.stringify(body) });
        await showAdmin({
          resource: name,
          list: name,
          message: name === "students"
            ? body.class_id ? "Ученик добавлен и зачислен в класс." : "Карточка ученика добавлена. Зачислите его в класс, чтобы данные появились в журналах."
            : `${schema.label} добавлен.`,
        });
      } catch (error) {
        submit.disabled = false;
        submit.textContent = `Добавить ${schema.label.toLowerCase()}`;
        panel.querySelector("#resource-error").textContent = error.message;
      }
    });
  };
  const drawList = () => {
    const target = panel.querySelector("#resource-table");
    drawResourceList(target, data[list.value], data.errors?.[list.value]);
    const retry = target.querySelector("[data-retry-resource]");
    retry?.addEventListener("click", async () => {
      retry.disabled = true;
      retry.textContent = "Загружаю…";
      try {
        data[list.value] = await api(`/admin/${list.value}`);
        delete data.errors[list.value];
        drawList();
      } catch (error) {
        data.errors[list.value] = error.message;
        drawList();
      }
    });
  };
  resource.addEventListener("change", drawForm);
  list.addEventListener("change", drawList);
  panel.querySelectorAll("[data-excel-download]").forEach(button => button.addEventListener("click", async () => {
    button.disabled = true;
    const status = panel.querySelector("#excel-status");
    status.textContent = "Формирую файл…";
    try {
      const blob = await apiBlob("/admin/excel");
      saveBlob(blob, "school-data.xlsx");
      status.textContent = "Файл Excel скачан.";
    } catch (error) {
      status.textContent = error.message;
      status.className = "error";
    } finally { button.disabled = false; }
  }));
  panel.querySelectorAll("[data-excel-select]").forEach(button => button.addEventListener("click", () => panel.querySelector("#excel-file").click()));
  panel.querySelector("#excel-file").addEventListener("change", async event => {
    const input = event.currentTarget;
    const file = input.files?.[0];
    if (!file) return;
    const status = panel.querySelector("#excel-status");
    const buttons = panel.querySelectorAll("[data-excel-select]");
    buttons.forEach(button => { button.disabled = true; });
    status.className = "muted";
    status.textContent = "Проверяю и импортирую данные…";
    try {
      const form = new FormData();
      form.append("file", file);
      const result = await api("/admin/excel", { method: "POST", body: form });
      const summary = Object.entries(result.imported || {}).map(([name, count]) => `${name}: ${count}`).join("; ");
      await showAdmin({ resource: resource.value, list: list.value, message: `Импорт завершён (${result.total} строк). ${summary}` });
    } catch (error) {
      const detail = error.details?.[0];
      status.className = "error";
      status.textContent = detail ? `${detail.sheet}, строка ${detail.row}: ${detail.reason}` : error.message;
    } finally {
      buttons.forEach(button => { button.disabled = false; });
      input.value = "";
    }
  });
  drawForm();
  drawList();
}

function renderField(field, data) {
  if (field.type === "checkbox") return `<label class="field"><span>${field.title}</span><input name="${field.key}" type="checkbox"></label>`;
  if (field.type === "reference") {
    const options = (data[field.resource] || []).filter(item => !field.role || item.roles?.split(", ").includes(field.role));
    const label = item => item.full_name || item.name || item.email || item.id;
    const help = field.resource === "users" && field.role === "teacher" && !options.length
      ? `<small class="field-hint">Учитель появится здесь после активации приглашения. Проверьте вкладку «Пользователи».</small>` : "";
    return `<label class="field">${field.title}<select name="${field.key}" ${field.optional ? "" : "required"}><option value="">${options.length ? (field.optional ? "Не выбирать" : "Выберите…") : "Нет доступных пользователей"}</option>${options.map(item => `<option value="${escapeHTML(item.id)}">${escapeHTML(label(item))}</option>`).join("")}</select>${help}</label>`;
  }
  return `<label class="field">${field.title}<input name="${field.key}" type="${field.type || "text"}" ${field.optional ? "" : "required"}></label>`;
}

function drawResourceList(target, rows, error = "") {
  if (error) { target.innerHTML = `<div class="notice error">${escapeHTML(error)}<button type="button" class="btn secondary" data-retry-resource>Повторить загрузку</button></div>`; return; }
  if (!rows?.length) { target.innerHTML = `<p class="empty">Записей пока нет.</p>`; return; }
  const columns = Object.keys(rows[0]);
  target.innerHTML = `<table><thead><tr>${columns.map(key => `<th>${escapeHTML(key)}</th>`).join("")}</tr></thead><tbody>${rows.map(row => `<tr>${columns.map(key => `<td>${escapeHTML(row[key] ?? "—")}</td>`).join("")}</tr>`).join("")}</tbody></table>`;
}

function renderInvites(panel, data) {
  if (data.errors?.invites) { renderSectionError(panel, data.errors.invites); return; }
  const students = data.students || [];
  const classes = data.classes || [];
  const years = data["academic-years"] || [];
  panel.innerHTML = `<div class="card"><h2>Новое приглашение</h2><form id="invite-form" class="form-grid">
    <label class="field">Email<input name="email" type="email" required></label>
    <label class="field">Роль<select name="role"><option value="student">Ученик</option><option value="teacher">Учитель</option><option value="class_teacher">Классный руководитель</option><option value="admin">Администратор</option></select><small id="invite-teacher-hint" class="field-hint" hidden>После активации приглашения учитель появится в справочнике «Назначение учителя».</small></label>
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
    const teacherHint = panel.querySelector("#invite-teacher-hint");
    if (teacherHint) teacherHint.hidden = role.value !== "teacher";
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
  if (data.errors?.users) { renderSectionError(panel, data.errors.users); return; }
  const roleOptions = ["student", "teacher", "class_teacher", "admin"];
  panel.innerHTML = `<h2>Пользователи школы</h2><div class="table"><table><thead><tr><th>ФИО</th><th>Email</th><th>Роли</th><th>Статус</th><th>Действия</th></tr></thead><tbody>${data.users.map(user => `<tr data-user-row="${escapeHTML(user.id)}"><td>${escapeHTML(user.full_name)}</td><td>${escapeHTML(user.email)}</td><td><div class="user-role-edit">${roleOptions.map(role => `<label><input type="checkbox" data-user-role="${role}" ${user.roles?.split(", ").includes(role) ? "checked" : ""}>${role}</label>`).join("")}</div></td><td><select data-user-status><option value="active" ${user.status === "active" ? "selected" : ""}>active</option><option value="disabled" ${user.status === "disabled" ? "selected" : ""}>disabled</option></select></td><td><button class="btn" data-save-user="${escapeHTML(user.id)}">Сохранить</button> <button class="btn secondary" data-revoke-user="${escapeHTML(user.id)}">Завершить сессии</button><span class="user-action-status" role="status" aria-live="polite"></span></td></tr>`).join("")}</tbody></table></div>`;
  panel.querySelectorAll("[data-save-user]").forEach(button => button.addEventListener("click", async () => {
    const row = button.closest("tr");
    const status = row.querySelector(".user-action-status");
    const roles = [...row.querySelectorAll("[data-user-role]:checked")].map(input => input.dataset.userRole);
    button.disabled = true;
    status.textContent = "Сохраняю…";
    try {
      await api(`/admin/users/${encodeURIComponent(button.dataset.saveUser)}`, { method: "PATCH", body: JSON.stringify({ status: row.querySelector("[data-user-status]").value, roles }) });
      status.textContent = "Сохранено";
      status.className = "user-action-status success";
    } catch (error) { status.textContent = error.message; status.className = "user-action-status error"; }
    finally { button.disabled = false; }
  }));
  panel.querySelectorAll("[data-revoke-user]").forEach(button => button.addEventListener("click", async () => {
    if (!confirm("Завершить все refresh-сессии этого пользователя?")) return;
    try { await api(`/admin/users/${encodeURIComponent(button.dataset.revokeUser)}/revoke-sessions`, { method: "POST", body: "{}" }); await showAdmin(); }
    catch (error) { alert(error.message); }
  }));
}

function renderSectionError(panel, message) {
  panel.innerHTML = `<div class="notice error">${escapeHTML(message)}<button type="button" class="btn secondary" data-retry-section>Повторить загрузку</button></div>`;
  panel.querySelector("[data-retry-section]").addEventListener("click", () => showAdmin());
}

function renderAudit(panel, data) {
  if (data.errors?.audit) { renderSectionError(panel, data.errors.audit); return; }
  const rows = data.audit || [];
  panel.innerHTML = `<h2>Последние изменения</h2><div class="table">${tableRows(rows, ["created_at", "actor_user_id", "action", "entity", "entity_id", "old_value", "new_value"], true)}</div>`;
}

function tableRows(rows, keys, includeHeader = false) {
  if (!rows?.length) return `<p class="empty">Записей пока нет.</p>`;
  return `<table>${includeHeader ? `<thead><tr>${keys.map(key => `<th>${escapeHTML(key)}</th>`).join("")}</tr></thead>` : ""}<tbody>${rows.map(row => `<tr>${keys.map(key => `<td>${escapeHTML(typeof row[key] === "object" ? JSON.stringify(row[key]) : row[key])}</td>`).join("")}</tr>`).join("")}</tbody></table>`;
}
