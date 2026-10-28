import { api, toCSV } from "./api.js";
import { escapeHTML, notice, renderPage, localDate } from "./ui.js";

export async function showGrades(user) {
  renderPage("Журнал", `<h1>Журнал оценок</h1><p class="sub">Выберите доступный класс, предмет и период. Роль определяет режим просмотра и редактирования.</p>
    ${notice()}<div class="toolbar"><label class="field-inline">Контекст<select id="grade-context"></select></label></div>
    <div id="grade-status" class="muted">Загружаю доступные классы и периоды…</div><div id="grade-result" class="table"></div>`);
  const status = document.querySelector("#grade-status");
  try {
    const contexts = await api("/grades/contexts");
    if (!contexts.length) {
      status.textContent = "Для этой учётной записи пока нет доступных классов или периодов.";
      return;
    }
    const selector = document.querySelector("#grade-context");
    selector.innerHTML = contexts.map((item, index) => `<option value="${index}">${escapeHTML(item.class_name)} · ${item.subject_name ? `${escapeHTML(item.subject_name)} · ` : ""}${escapeHTML(item.term_name)}</option>`).join("");
    const load = () => loadContext(contexts[Number(selector.value)], user);
    selector.addEventListener("change", load);
    await load();
  } catch (error) {
    status.textContent = error.message;
  }
}

async function loadContext(context, user) {
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
        ? table(["Ученик", "Предмет", "Средний балл", "Оценок"], rows.map(row => [row.student, row.subject, row.average ?? "—", row.count]))
        : `<p class="empty">В этом периоде пока нет оценок.</p>`;
      status.textContent = "Сводная ведомость доступна только для чтения.";
      return;
    }

    const canEdit = Boolean(context.can_edit) && !isStudent;
    const [grades, students] = await Promise.all([
      api(`/grades?${params}`),
      canEdit ? api(`/students?class_id=${encodeURIComponent(context.class_id)}&limit=500`) : Promise.resolve([]),
    ]);
    status.textContent = grades.length ? `Записей: ${grades.length}${grades.length === 1000 ? " (достигнут лимит выдачи)" : ""}` : "Оценок пока нет.";
    const createForm = canEdit ? newGradeForm(students) : "";
    const rows = grades.map(grade => canEdit ? editableGradeRow(grade) : [grade.student_name, grade.work_date, grade.value, grade.grade_type, grade.comment]);
    output.innerHTML = `${createForm}${grades.length ? table(canEdit ? ["Ученик", "Дата", "Оценка", "Тип", "Комментарий", "Действия"] : ["Ученик", "Дата", "Оценка", "Тип", "Комментарий"], rows, canEdit) : `<p class="empty">Оценок пока нет.</p>`}
      <div class="toolbar"><button class="btn secondary" id="grade-export">Экспорт CSV</button></div>`;
    document.querySelector("#grade-export")?.addEventListener("click", () => toCSV("grades.csv", [["Ученик", "Дата", "Оценка", "Тип", "Комментарий"], ...grades.map(row => [row.student_name, row.work_date, row.value, row.grade_type, row.comment])]));
    if (canEdit) bindGradeEdits(output, context);
  } catch (error) {
    status.textContent = error.message;
  }
}

function newGradeForm(students) {
  if (!students.length) return `<p class="empty">В выбранном классе нет активных учеников.</p>`;
  return `<details class="card new-grade"><summary>Добавить оценку</summary><form id="new-grade-form" class="form-grid">
    <label class="field">Ученик<select name="student_id" required>${students.map(student => `<option value="${escapeHTML(student.id)}">${escapeHTML(student.full_name)}</option>`).join("")}</select></label>
    <label class="field">Дата<input name="work_date" type="date" value="${localDate()}" required></label>
    <label class="field">Оценка<select name="value" required>${[2, 3, 4, 5].map(value => `<option>${value}</option>`).join("")}</select></label>
    <label class="field">Тип работы<input name="grade_type" value="regular" maxlength="40" required></label>
    <label class="field field-wide">Комментарий<input name="comment" maxlength="500"></label>
    <div><button class="btn">Сохранить</button><span id="new-grade-error" class="error"></span></div>
  </form></details>`;
}

function editableGradeRow(grade) {
  return `<tr data-grade="${escapeHTML(grade.id)}"><td>${escapeHTML(grade.student_name)}</td>
    <td><input name="work_date" type="date" value="${escapeHTML(grade.work_date)}"></td>
    <td><select name="value">${[2, 3, 4, 5].map(value => `<option value="${value}" ${Number(grade.value) === value ? "selected" : ""}>${value}</option>`).join("")}</select></td>
    <td><input name="grade_type" value="${escapeHTML(grade.grade_type)}" maxlength="40"></td>
    <td><input name="comment" value="${escapeHTML(grade.comment)}" maxlength="500"></td>
    <td><button class="btn secondary" data-save-grade>Сохранить</button> <button class="btn danger" data-delete-grade>Удалить</button></td></tr>`;
}

function table(headers, rows, raw = false) {
  const values = rows.map(row => raw ? row : `<tr>${row.map(value => `<td>${escapeHTML(value)}</td>`).join("")}</tr>`);
  return `<table><thead><tr>${headers.map(header => `<th>${escapeHTML(header)}</th>`).join("")}</tr></thead><tbody>${values.join("")}</tbody></table>`;
}

function bindGradeEdits(output, context) {
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
      await loadContext(context, window.currentUser);
    } catch (reason) { error.textContent = reason.message; }
  });
  output.querySelectorAll("[data-save-grade]").forEach(button => button.addEventListener("click", async () => {
    const row = button.closest("tr");
    const body = Object.fromEntries(new FormDataRow(row).entries());
    try {
      await api(`/grades/${encodeURIComponent(row.dataset.grade)}`, { method: "PATCH", body: JSON.stringify({
        work_date: body.work_date, value: Number(body.value), grade_type: body.grade_type, comment: body.comment,
      }) });
      await loadContext(context, window.currentUser);
    } catch (error) { alert(error.message); }
  }));
  output.querySelectorAll("[data-delete-grade]").forEach(button => button.addEventListener("click", async () => {
    const row = button.closest("tr");
    if (!confirm("Удалить эту оценку? Она останется в аудите.")) return;
    try {
      await api(`/grades/${encodeURIComponent(row.dataset.grade)}`, { method: "DELETE" });
      await loadContext(context, window.currentUser);
    } catch (error) { alert(error.message); }
  }));
}

class FormDataRow {
  constructor(row) { this.row = row; }
  entries() { return [...this.row.querySelectorAll("input,select")].map(field => [field.name, field.value]); }
}
