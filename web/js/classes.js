import { api } from "./api.js";
import { escapeHTML } from "./ui.js";

const label = item => item?.full_name || item?.name || item?.email || "—";
const normalize = name => name.trim().replace(/\s+/g, " ");
const identity = name => normalize(name).toLocaleLowerCase("ru");

export function parseStudentList(text, knownStudents = []) {
  const names = text.split(/\r?\n/).map(line => normalize(line.replace(/^\s*\d+[.)]\s*/, ""))).filter(Boolean);
  const seen = new Set();
  return names.map((name, index) => {
    const key = identity(name);
    const problem = name.length > 200 ? "ФИО длиннее 200 символов" : seen.has(key) ? "Повтор в списке" : knownStudents.some(student => identity(student.full_name) === key) ? "Карточка уже существует — выберите существующего ученика" : "";
    seen.add(key);
    return { name, row: index + 1, problem };
  });
}

export function renderClasses(panel, data, state, refresh, advanced) {
  const years = data["academic-years"] || [];
  const current = years.find(year => year.is_current) || years[0];
  let yearID = state.year_id || data.classes.find(item => item.id === state.class_id)?.academic_year_id || current?.id || "";
  let selected = state.class_id || "";
  let rosterRequest = 0;
  const mayLeave = () => !panel.querySelector('[data-dirty="true"]') || confirm("Есть несохранённые данные. Перейти в другой класс?");
  panel.innerHTML = `<div class="class-workspace"><div class="resource-heading"><span class="resource-eyebrow">УПРАВЛЕНИЕ ШКОЛОЙ</span><h2>Классы и ученики</h2><p class="muted">Откройте класс, чтобы добавить учеников и назначить учителей.</p></div>
    <div class="toolbar"><label class="field-inline">Учебный год<select id="class-year">${years.map(year => `<option value="${escapeHTML(year.id)}" ${year.id === yearID ? "selected" : ""}>${escapeHTML(year.name)}${year.is_current ? " · текущий" : ""}</option>`).join("")}</select></label><button type="button" class="btn secondary" id="class-year-setup">Настроить учебный год</button></div>
    <details class="card class-create"><summary>Создать класс</summary><form id="class-create-form" class="toolbar"><label class="field-inline">Название класса<input name="name" placeholder="Например, 7А" maxlength="50" required></label><button class="btn" ${!years.length ? "disabled" : ""}>Создать в выбранном году</button><span class="error" role="status"></span></form></details>
    <div id="class-catalog" class="resource-catalog"></div><div id="class-detail"></div></div>`;
  const draw = () => {
    rosterRequest++;
    state.year_id = yearID;
    const classes = data.classes.filter(item => item.academic_year_id === yearID);
    if (!classes.some(item => item.id === selected)) selected = classes[0]?.id || "";
    state.class_id = selected;
    const catalog = panel.querySelector("#class-catalog");
    const target = panel.querySelector("#class-detail");
    catalog.innerHTML = data.errors?.classes ? `<p class="error">${escapeHTML(data.errors.classes)}</p>` : classes.length ? classes.map(item => `<button type="button" class="resource-tile class-tile" data-class="${escapeHTML(item.id)}" aria-pressed="${item.id === selected}"><strong>${escapeHTML(item.name)}</strong><span>Открыть класс →</span></button>`).join("") : `<p class="empty">${years.length ? "В этом году ещё нет классов. Создайте первый класс выше." : "Сначала добавьте учебный год."}</p>`;
    catalog.querySelectorAll("[data-class]").forEach(button => button.addEventListener("click", () => { if (selected === button.dataset.class || !mayLeave()) return; selected = button.dataset.class; draw(); }));
    const schoolClass = classes.find(item => item.id === selected);
    if (!schoolClass) { target.innerHTML = ""; return; }
    const year = years.find(item => item.id === yearID);
    target.innerHTML = `<section class="card class-overview"><div class="card-heading"><div><h2>${escapeHTML(schoolClass.name)}</h2><span class="muted">${escapeHTML(year?.name)}</span></div><span class="status neutral" id="class-roster-count">Загрузка…</span></div>
      <div class="toolbar"><button class="btn" data-student-mode="single">Добавить ученика</button><button class="btn secondary" data-student-mode="bulk">Добавить списком</button><button class="btn secondary" data-student-mode="existing">Зачислить существующего</button></div><div id="class-student-editor"></div>
      <div class="resource-list-heading"><h3>Ученики класса</h3><label class="search-field">Поиск<input type="search" id="class-roster-search" placeholder="Фамилия или табельный номер"></label></div><div id="class-roster" class="table"><p class="empty">Загружаю учеников…</p></div></section>
      <section class="card class-assignments"><h2>Учителя и предметы</h2><div id="class-assignment-list" class="table"></div><details><summary>Добавить назначение</summary><form id="class-assignment-form" class="form-grid"><label class="field">Назначение<select name="kind"><option value="teacher-assignments">Учитель предмета</option><option value="class-teacher-assignments">Классный руководитель</option></select></label><label class="field">Учитель<select name="teacher_id" required></select></label><label class="field" id="class-subject-field">Предмет<select name="subject_id" required>${data.subjects.map(subject => `<option value="${escapeHTML(subject.id)}">${escapeHTML(subject.name)}</option>`).join("")}</select></label><div><button class="btn">Назначить</button><p class="error" role="status"></p></div></form></details></section>`;
    target.querySelectorAll("[data-student-mode]").forEach(button => button.addEventListener("click", () => showStudentEditor(button.dataset.studentMode, target, data, schoolClass, year, state, refresh)));
    loadRoster(target, schoolClass, ++rosterRequest, () => rosterRequest);
    renderAssignments(target, data, schoolClass, state, refresh);
  };
  panel.querySelector("#class-year").addEventListener("change", event => { if (!mayLeave()) { event.target.value = yearID; return; } yearID = event.target.value; selected = ""; draw(); });
  panel.querySelector("#class-year-setup").addEventListener("click", () => advanced("academic-years"));
  panel.querySelector("#class-create-form").addEventListener("submit", async event => {
    event.preventDefault();
    const form = event.currentTarget;
    const name = form.elements.name.value.trim();
    if (!name || !yearID) return;
    const button = form.querySelector("button");
    button.disabled = true;
    try {
      const created = await api("/admin/classes", { method: "POST", body: JSON.stringify({ name, academic_year_id: yearID }) });
      await refresh({ tab: "classes", class_id: created.id, year_id: yearID, message: `Класс ${name} создан. Теперь добавьте учеников.` });
    } catch (error) { form.querySelector(".error").textContent = error.message; button.disabled = false; }
  });
  draw();
}

async function loadRoster(target, schoolClass, version, current) {
  const root = target.querySelector("#class-roster");
  try {
    const students = await api(`/students?${new URLSearchParams({ class_id: schoolClass.id, limit: "500" })}`);
    if (!root.isConnected || version !== current()) return;
    target.querySelector("#class-roster-count").textContent = `${students.length} учеников${students.length === 500 ? " · лимит выдачи" : ""}`;
    const draw = () => {
      const query = target.querySelector("#class-roster-search").value.trim().toLocaleLowerCase("ru");
      const filtered = students.filter(student => `${student.full_name} ${student.external_id}`.toLocaleLowerCase("ru").includes(query));
      root.innerHTML = filtered.length ? `<table><thead><tr><th>№</th><th>ФИО</th><th>Табельный номер</th></tr></thead><tbody>${filtered.map((student, index) => `<tr><td>${index + 1}</td><td>${escapeHTML(student.full_name)}</td><td>${escapeHTML(student.external_id)}</td></tr>`).join("")}</tbody></table>` : `<p class="empty">${query ? "Ничего не найдено." : "В классе пока нет учеников. Добавьте одного или вставьте список."}</p>`;
    };
    target.querySelector("#class-roster-search").addEventListener("input", draw);
    draw();
  } catch (error) { if (root.isConnected && version === current()) { root.innerHTML = `<p class="error empty">${escapeHTML(error.message)}</p>`; target.querySelector("#class-roster-count").textContent = "Ошибка загрузки"; } }
}

function showStudentEditor(mode, target, data, schoolClass, year, state, refresh) {
  const root = target.querySelector("#class-student-editor");
  const previous = root.querySelector("form");
  if (previous?.dataset.saving) return;
  if (previous?.dataset.dirty && !confirm("В форме есть несохранённые данные. Открыть другую форму?")) return;
  const titles = { single: "Новый ученик", bulk: "Ученики списком", existing: "Зачисление существующего ученика" };
  root.innerHTML = `<form class="student-editor form-grid"><div class="card-heading field-wide"><h3>${titles[mode]}</h3><button type="button" class="text-link" data-close>Закрыть</button></div>
    ${mode === "single" ? `<label class="field field-wide">ФИО<input name="full_name" maxlength="200" required placeholder="Иванов Иван Иванович"></label><details class="field-wide"><summary>Дополнительные данные</summary><label class="field">Дата рождения<input type="date" name="birth_date"></label><label class="field">Табельный номер<input name="external_id" maxlength="64" placeholder="Будет создан автоматически"></label></details>` : mode === "bulk" ? `<label class="field field-wide">Каждый ученик с новой строки<textarea name="names" rows="7" required placeholder="Иванов Иван Иванович&#10;Петрова Анна Сергеевна"></textarea><small>Можно вставить нумерованный список. До 100 учеников за один раз. Табельные номера создаются автоматически.</small></label><div id="student-preview" class="field-wide"></div>` : `<label class="field field-wide">Найти карточку<input type="search" name="search" placeholder="Фамилия или табельный номер"></label><label class="field field-wide">Ученик<select name="student_id" required></select><small>Если ученик уже зачислен в другом классе этого года, сначала оформите перевод через настройки.</small></label>`}
    <label class="field">Дата зачисления<input type="date" name="from_date" min="${escapeHTML(year.starts_on)}" max="${escapeHTML(year.ends_on)}" value="${escapeHTML(year.starts_on)}" required></label><div><span class="muted">Класс ${escapeHTML(schoolClass.name)} · ${escapeHTML(year.name)}</span></div><div class="field-wide"><button class="btn" type="submit">${mode === "bulk" ? "Добавить список" : "Добавить в класс"}</button><p class="error" role="status"></p></div></form>`;
  const form = root.querySelector("form");
  form.addEventListener("input", () => { form.dataset.dirty = "true"; });
  root.querySelector("[data-close]").addEventListener("click", () => { if (!form.dataset.dirty || confirm("Закрыть форму без сохранения?")) root.innerHTML = ""; });
  const submit = form.querySelector('[type="submit"]');
  const preview = () => {
    const rows = parseStudentList(form.elements.names.value, data.students);
    const invalid = !rows.length || rows.length > 100 || rows.some(row => row.problem);
    submit.disabled = invalid;
    root.querySelector("#student-preview").innerHTML = `<p class="muted">К добавлению: ${rows.length}${rows.length > 100 ? " · максимум 100" : ""}</p>${rows.length ? `<div class="table student-preview"><table><thead><tr><th>№</th><th>ФИО</th><th>Проверка</th></tr></thead><tbody>${rows.map(row => `<tr><td>${row.row}</td><td>${escapeHTML(row.name)}</td><td class="${row.problem ? "error" : "success"}">${escapeHTML(row.problem || "Готово")}</td></tr>`).join("")}</tbody></table></div>` : ""}`;
    return rows;
  };
  if (mode === "bulk") { form.elements.names.addEventListener("input", preview); preview(); }
  if (mode === "existing") {
    const drawOptions = () => {
      const query = form.elements.search.value.trim().toLocaleLowerCase("ru");
      const enrolled = new Set(data.enrollments.filter(item => item.class_id === schoolClass.id && item.status === "active").map(item => item.student_id));
      const options = data.students.filter(item => !enrolled.has(item.id) && `${item.full_name} ${item.external_id}`.toLocaleLowerCase("ru").includes(query));
      form.elements.student_id.innerHTML = `<option value="">Выберите ученика</option>${options.map(item => `<option value="${escapeHTML(item.id)}">${escapeHTML(item.full_name)} · ${escapeHTML(item.external_id)}</option>`).join("")}`;
    };
    form.elements.search.addEventListener("input", drawOptions);
    drawOptions();
  }
  form.addEventListener("submit", async event => {
    event.preventDefault();
    let students;
    if (mode === "bulk") { const rows = preview(); if (!rows.length || rows.length > 100 || rows.some(row => row.problem)) return; students = rows.map(row => ({ full_name: row.name })); }
    else if (mode === "existing") students = [{ student_id: form.elements.student_id.value }];
    else students = [{ full_name: form.elements.full_name.value, birth_date: form.elements.birth_date.value, external_id: form.elements.external_id.value }];
    form.dataset.saving = "true";
    const controls = [...form.querySelectorAll("input,select,textarea,button")];
    controls.forEach(control => { control.disabled = true; });
    submit.textContent = "Сохраняю…";
    form.querySelector(".error").textContent = "";
    try {
      const result = await api(`/admin/classes/${encodeURIComponent(schoolClass.id)}/students`, { method: "POST", body: JSON.stringify({ students, from_date: form.elements.from_date.value }) });
      await refresh({ ...state, tab: "classes", class_id: schoolClass.id, message: `В класс ${schoolClass.name} добавлено учеников: ${result.enrolled}.` });
    } catch (error) { delete form.dataset.saving; controls.forEach(control => { control.disabled = false; }); form.querySelector(".error").textContent = error.message; submit.textContent = mode === "bulk" ? "Добавить список" : "Добавить в класс"; }
  });
  form.querySelector("input,textarea,select")?.focus();
}

function renderAssignments(target, data, schoolClass, state, refresh) {
  const assignments = [...data["class-teacher-assignments"].filter(item => item.class_id === schoolClass.id).map(item => ({ ...item, subject: "Классный руководитель" })), ...data["teacher-assignments"].filter(item => item.class_id === schoolClass.id).map(item => ({ ...item, subject: label(data.subjects.find(subject => subject.id === item.subject_id)) }))];
  const list = target.querySelector("#class-assignment-list");
  const error = data.errors?.["teacher-assignments"] || data.errors?.["class-teacher-assignments"] || data.errors?.users;
  list.innerHTML = error ? `<p class="error empty">${escapeHTML(error)}</p>` : assignments.length ? `<table><thead><tr><th>Предмет / назначение</th><th>Учитель</th></tr></thead><tbody>${assignments.map(item => `<tr><td>${escapeHTML(item.subject)}</td><td>${escapeHTML(label(data.users.find(user => user.id === item.teacher_id)))}</td></tr>`).join("")}</tbody></table>` : `<p class="empty">Учителя пока не назначены.</p>`;
  const form = target.querySelector("#class-assignment-form");
  const update = () => {
    const role = form.elements.kind.value === "teacher-assignments" ? "teacher" : "class_teacher";
    form.elements.teacher_id.innerHTML = `<option value="">Выберите учителя</option>${data.users.filter(user => user.status === "active" && user.roles?.split(", ").includes(role)).map(user => `<option value="${escapeHTML(user.id)}">${escapeHTML(user.full_name)}</option>`).join("")}`;
    form.elements.subject_id.required = role === "teacher";
    form.elements.subject_id.disabled = role !== "teacher";
    target.querySelector("#class-subject-field").hidden = role !== "teacher";
  };
  form.addEventListener("input", () => { form.dataset.dirty = "true"; });
  form.elements.kind.addEventListener("change", update);
  update();
  form.addEventListener("submit", async event => {
    event.preventDefault();
    const button = form.querySelector("button"); button.disabled = true;
    try {
      const body = { teacher_id: form.elements.teacher_id.value, class_id: schoolClass.id, academic_year_id: schoolClass.academic_year_id };
      if (!form.elements.subject_id.disabled) body.subject_id = form.elements.subject_id.value;
      await api(`/admin/${form.elements.kind.value}`, { method: "POST", body: JSON.stringify(body) });
      await refresh({ ...state, tab: "classes", message: "Назначение добавлено." });
    } catch (error) { form.querySelector(".error").textContent = error.message; button.disabled = false; }
  });
}
