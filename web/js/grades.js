import { api, toCSV } from "./api.js";
import { escapeHTML, notice, renderPage, localDate, showNoContexts } from "./ui.js";

export async function showGrades(user, selectedContext = null) {
  renderPage("Журнал", `<h1>Журнал оценок</h1><p class="sub">Выберите доступный класс, предмет и период. Роль определяет режим просмотра и редактирования.</p>
    ${notice()}<div class="toolbar"><label class="field-inline">Класс, предмет и период<select id="grade-context" disabled><option>Загрузка…</option></select></label><button class="btn secondary" id="grade-refresh">Обновить</button></div>
    <div id="grade-status" class="muted" aria-live="polite">Загружаю доступные классы и периоды…</div><div id="grade-result" class="table"></div>`);
  const status = document.querySelector("#grade-status");
  const selector = document.querySelector("#grade-context");
  document.querySelector("#grade-refresh").addEventListener("click", () => showGrades(user, selectedContext));
  try {
    const contexts = await api("/grades/contexts");
    if (!contexts.length) {
      showNoContexts(status, user, selector);
      return;
    }
    selector.innerHTML = contexts.map((item, index) => `<option value="${index}">${escapeHTML(item.class_name)} · ${item.subject_name ? `${escapeHTML(item.subject_name)} · ` : ""}${escapeHTML(item.term_name)}</option>`).join("");
    selector.disabled = false;
    if (selectedContext) {
      const selectedIndex = contexts.findIndex(item => item.class_id === selectedContext.class_id
        && item.term_id === selectedContext.term_id
        && (item.subject_id || "") === (selectedContext.subject_id || ""));
      if (selectedIndex >= 0) selector.value = String(selectedIndex);
    }
    const load = () => loadContext(contexts[Number(selector.value)], user);
    selector.addEventListener("change", load);
    await load();
  } catch (error) {
    selector.disabled = true;
    selector.replaceChildren(new Option("Не удалось загрузить список", ""));
    status.textContent = error.message;
  }
}

async function loadContext(context, user, draft = null, successMessage = "") {
  const status = document.querySelector("#grade-status");
  const output = document.querySelector("#grade-result");
  status.textContent = "Загружаю журнал…";
  output.innerHTML = "";
  try {
    const params = new URLSearchParams({ class_id: context.class_id, term_id: context.term_id });
    if (context.subject_id) params.set("subject_id", context.subject_id);
    const isStudent = user.roles.includes("student");
    const isClassTeacher = user.roles.includes("class_teacher") && !context.subject_id && !isStudent;
    if (isClassTeacher) {
      const rows = await api(`/grades/class-summary?${params}`);
      output.innerHTML = rows.length
        ? `<div class="grade-tools"><label class="search-field"><span>Найти ученика</span><input type="search" data-grade-search placeholder="Фамилия или имя"></label></div>${table(["Ученик", "Предмет", "Средний балл", "Оценок"], rows.map(row => [row.student, row.subject, row.average ?? "—", row.count]))}`
        : `<p class="empty">В этом периоде пока нет оценок.</p>`;
      bindTableSearch(output);
      status.textContent = "Сводная ведомость доступна только для чтения.";
      return;
    }

    const canEdit = Boolean(context.can_edit) && !isStudent;
    const [grades, students] = await Promise.all([
      api(`/grades?${params}`),
      canEdit ? api(`/students?class_id=${encodeURIComponent(context.class_id)}&limit=500`) : Promise.resolve([]),
    ]);
    status.textContent = `${grades.length ? `Записей: ${grades.length}${grades.length === 1000 ? " (достигнут лимит выдачи)" : ""}` : "Оценок пока нет."}${successMessage ? ` · ${successMessage}` : ""}`;
    const createForm = canEdit ? newGradeForm(students, draft) : "";
    const rows = grades.map(grade => canEdit ? editableGradeRow(grade) : [grade.student_name, grade.work_date, grade.value, gradeTypeLabel(grade.grade_type), grade.comment]);
    output.innerHTML = `${createForm}${grades.length ? `<div class="grade-tools"><label class="search-field"><span>Найти ученика</span><input type="search" data-grade-search placeholder="Фамилия или имя"></label><button class="btn secondary" id="grade-export">Экспорт CSV</button></div>${table(canEdit ? ["Ученик", "Дата", "Оценка", "Тип работы", "Комментарий", "Действия"] : ["Ученик", "Дата", "Оценка", "Тип работы", "Комментарий"], rows, canEdit)}` : `<p class="empty">Оценок пока нет.</p><div class="toolbar"><button class="btn secondary" id="grade-export">Экспорт CSV</button></div>`}
      <p id="grade-action-status" class="error" aria-live="polite"></p>`;
    bindTableSearch(output);
    document.querySelector("#grade-export")?.addEventListener("click", () => toCSV("grades.csv", [["Ученик", "Дата", "Оценка", "Тип работы", "Комментарий"], ...grades.map(row => [row.student_name, row.work_date, row.value, gradeTypeLabel(row.grade_type), row.comment])]));
    if (canEdit) bindGradeEdits(output, context);
  } catch (error) {
    status.textContent = error.message;
  }
}

function newGradeForm(students, draft = null) {
  if (!students.length) return `<p class="empty">В выбранном классе нет активных учеников.</p>`;
  return `<details class="card new-grade" open><summary>Быстро добавить оценку</summary><form id="new-grade-form" class="form-grid">
    <label class="field field-wide">Найти ученика<input type="search" data-student-search placeholder="Начните вводить фамилию или имя"></label>
    <label class="field">Ученик<select name="student_id" required>${students.map(student => `<option value="${escapeHTML(student.id)}" ${draft?.student_id === student.id ? "selected" : ""}>${escapeHTML(student.full_name)}</option>`).join("")}</select></label>
    <label class="field">Дата<input name="work_date" type="date" value="${escapeHTML(draft?.work_date || localDate())}" required></label>
    <label class="field">Оценка<select name="value" required><option value="" disabled ${draft?.value ? "" : "selected"}>Выберите оценку</option>${[2, 3, 4, 5].map(value => `<option value="${value}" ${Number(draft?.value) === value ? "selected" : ""}>${value}</option>`).join("")}</select></label>
    <label class="field">Тип работы<select name="grade_type" required>${gradeTypeOptions(draft?.grade_type || "regular")}</select></label>
    <label class="field field-wide">Комментарий<input name="comment" value="${escapeHTML(draft?.comment || "")}" maxlength="500"></label>
    <div><button class="btn">Сохранить</button><span id="new-grade-error" class="error"></span></div>
  </form></details>`;
}

function editableGradeRow(grade) {
  return `<tr data-grade="${escapeHTML(grade.id)}"><td>${escapeHTML(grade.student_name)}</td>
    <td><input name="work_date" type="date" value="${escapeHTML(grade.work_date)}"></td>
    <td><select name="value">${[2, 3, 4, 5].map(value => `<option value="${value}" ${Number(grade.value) === value ? "selected" : ""}>${value}</option>`).join("")}</select></td>
    <td><select name="grade_type">${gradeTypeOptions(grade.grade_type)}</select></td>
    <td><input name="comment" value="${escapeHTML(grade.comment)}" maxlength="500"></td>
    <td><button class="btn secondary" data-save-grade>Сохранить</button> <button class="btn danger" data-delete-grade>Удалить</button></td></tr>`;
}

function table(headers, rows, raw = false) {
  const values = rows.map(row => raw ? row : `<tr>${row.map(value => `<td>${escapeHTML(value)}</td>`).join("")}</tr>`);
  return `<table><thead><tr>${headers.map(header => `<th>${escapeHTML(header)}</th>`).join("")}</tr></thead><tbody>${values.join("")}</tbody></table>`;
}

function bindTableSearch(output) {
  const search = output.querySelector("[data-grade-search]");
  search?.addEventListener("input", () => {
    const query = search.value.trim().toLocaleLowerCase("ru");
    output.querySelectorAll("tbody tr").forEach(row => {
      row.hidden = query !== "" && !(row.cells[0]?.textContent || "").toLocaleLowerCase("ru").includes(query);
    });
  });
}

function gradeTypeLabel(value) {
  return gradeTypes.find(item => item.value === value)?.label || value;
}

const gradeTypes = [
  { value: "regular", label: "Текущая оценка" },
  { value: "homework", label: "Домашняя работа" },
  { value: "classwork", label: "Работа на уроке" },
  { value: "quiz", label: "Самостоятельная работа" },
  { value: "test", label: "Контрольная работа" },
  { value: "exam", label: "Экзамен" },
  { value: "other", label: "Другое" },
];

function gradeTypeOptions(selected) {
  const known = gradeTypes.find(item => item.value === selected);
  const custom = !known && selected ? `<option value="${escapeHTML(selected)}" selected>${escapeHTML(selected)}</option>` : "";
  return `${custom}${gradeTypes.map(item => `<option value="${item.value}" ${item.value === selected ? "selected" : ""}>${item.label}</option>`).join("")}`;
}

function bindGradeEdits(output, context) {
  const studentSearch = output.querySelector("[data-student-search]");
  const studentSelect = output.querySelector('#new-grade-form [name="student_id"]');
  studentSearch?.addEventListener("input", () => {
    const query = studentSearch.value.trim().toLocaleLowerCase("ru");
    const options = [...studentSelect.options];
    options.forEach(option => { option.hidden = query !== "" && !option.textContent.toLocaleLowerCase("ru").includes(query); });
    if (studentSelect.selectedOptions[0]?.hidden) {
      const firstVisible = options.find(option => !option.hidden);
      if (firstVisible) studentSelect.value = firstVisible.value;
    }
  });
  document.querySelector("#new-grade-form")?.addEventListener("submit", async event => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const error = document.querySelector("#new-grade-error");
    error.textContent = "";
    try {
      await api("/grades", { method: "POST", body: JSON.stringify({
        student_id: form.get("student_id"), class_id: context.class_id, subject_id: context.subject_id,
        term_id: context.term_id, work_date: form.get("work_date"), value: Number(form.get("value")),
        grade_type: form.get("grade_type"), comment: form.get("comment"),
      }) });
      await loadContext(context, window.currentUser, {
        student_id: form.get("student_id"), work_date: form.get("work_date"), value: null,
        grade_type: form.get("grade_type"), comment: "",
      }, "Оценка добавлена");
    } catch (reason) { error.textContent = reason.message; }
  });
  output.querySelectorAll("[data-save-grade]").forEach(button => button.addEventListener("click", async () => {
    const row = button.closest("tr");
    const body = Object.fromEntries(new FormDataRow(row).entries());
    try {
      await api(`/grades/${encodeURIComponent(row.dataset.grade)}`, { method: "PATCH", body: JSON.stringify({
        work_date: body.work_date, value: Number(body.value), grade_type: body.grade_type, comment: body.comment,
      }) });
      await loadContext(context, window.currentUser, null, "Изменения сохранены");
    } catch (error) { output.querySelector("#grade-action-status").textContent = error.message; }
  }));
  output.querySelectorAll("[data-delete-grade]").forEach(button => button.addEventListener("click", async () => {
    const row = button.closest("tr");
    if (!confirm("Удалить эту оценку? Она останется в аудите.")) return;
    try {
      await api(`/grades/${encodeURIComponent(row.dataset.grade)}`, { method: "DELETE" });
      await loadContext(context, window.currentUser, null, "Оценка удалена");
    } catch (error) { output.querySelector("#grade-action-status").textContent = error.message; }
  }));
}

class FormDataRow {
  constructor(row) { this.row = row; }
  entries() { return [...this.row.querySelectorAll("input,select")].map(field => [field.name, field.value]); }
}
