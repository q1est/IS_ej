import { api, toCSV } from "./api.js";
import { renderGradeMatrix } from "./grade-matrix.js";
let gradeRequest = 0;
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
    if (!selector.isConnected) return;
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
    let selectedIndex = selector.value;
    const load = () => {
      if (document.querySelector('[data-journal-dirty="true"]') && !confirm("Есть несохранённые оценки. Сменить журнал?")) { selector.value = selectedIndex; return; }
      selectedIndex = selector.value;
      return loadContext(contexts[Number(selector.value)], user, null, "", contexts);
    };
    selector.addEventListener("change", load);
    await load();
  } catch (error) {
    selector.disabled = true;
    selector.replaceChildren(new Option("Не удалось загрузить список", ""));
    status.textContent = error.message;
  }
}

async function loadContext(context, user, draft = null, successMessage = "", availableContexts = [], matrixState = {}) {
  const version = ++gradeRequest;
  const status = document.querySelector("#grade-status");
  const output = document.querySelector("#grade-result");
  status.textContent = "Загружаю журнал…";
  output.innerHTML = "";
  try {
    const params = new URLSearchParams({ class_id: context.class_id, term_id: context.term_id });
    if (context.subject_id) params.set("subject_id", context.subject_id);
    const isStudent = user.roles.includes("student") && !user.roles.includes("admin");
    const canEdit = Boolean(context.can_edit) && !isStudent;
    const [grades, students] = await Promise.all([
      api(`/grades?${params}`),
      !isStudent ? api(`/students?class_id=${encodeURIComponent(context.class_id)}&limit=500`) : Promise.resolve([]),
    ]);
    if (!output.isConnected || version !== gradeRequest) return;
    status.textContent = `${grades.length ? `Записей: ${grades.length}${grades.length === 1000 ? " (достигнут лимит выдачи)" : ""}` : "Оценок пока нет."}${successMessage ? ` · ${successMessage}` : ""}`;
    const subjectContexts = availableContexts.filter(item => item.class_id === context.class_id && item.term_id === context.term_id && item.subject_id && item.can_edit);
    const createForm = canEdit ? newGradeForm(students, draft || { subject_id: context.subject_id }, subjectContexts) : "";
    const rows = grades.map(grade => canEdit ? editableGradeRow(grade) : [grade.student_name, grade.work_date, grade.value, gradeTypeLabel(grade.grade_type), grade.comment]);
    output.innerHTML = `${createForm}${grades.length ? `<div class="grade-tools"><label class="search-field"><span>Найти ученика</span><input type="search" data-grade-search placeholder="Фамилия или имя"></label><button class="btn secondary" id="grade-export">Экспорт CSV</button></div>${table(canEdit ? ["Ученик", "Дата", "Оценка", "Тип работы", "Комментарий", "Действия"] : ["Ученик", "Дата", "Оценка", "Тип работы", "Комментарий"], rows, canEdit)}` : `<p class="empty">Оценок пока нет.</p><div class="toolbar"><button class="btn secondary" id="grade-export">Экспорт CSV</button></div>`}
      <p id="grade-action-status" class="error" aria-live="polite"></p>`;
    const listHTML = output.innerHTML;
    output.oninput = event => {
      const edited = event.target.closest("#new-grade-form, tr[data-grade]");
      if (edited) edited.dataset.journalDirty = "true";
    };
    let view = "matrix";
    let subject = matrixState.subject || "";
    let selectedDate = matrixState.date || localDate();
    const subjects = [...new Map(grades.map(grade => [grade.subject_id, grade.subject_name])).entries()];
    const drawView = () => {
      if (view === "list") {
        output.innerHTML = `<div class="journal-view-tabs"><button class="btn secondary" data-view="matrix">Таблица по датам</button><button class="btn" data-view="list" aria-pressed="true">${canEdit ? "Список и редактирование" : "Список"}</button></div>${listHTML}`;
        bindTableSearch(output);
        if (canEdit) bindGradeEdits(output, context, availableContexts);
      } else {
        output.innerHTML = `<div class="journal-view-tabs"><button class="btn" data-view="matrix" aria-pressed="true">Таблица по датам</button><button class="btn secondary" data-view="list">${canEdit ? "Список и редактирование" : "Список"}</button></div><div class="grade-tools"><label class="search-field">Найти ученика<input type="search" data-grade-search placeholder="Фамилия или имя"></label>${!context.subject_id && subjects.length > 1 ? `<label class="field-inline">Предмет<select id="matrix-subject"><option value="">Все предметы</option>${subjects.map(([id, name]) => `<option value="${escapeHTML(id)}" ${id === subject ? "selected" : ""}>${escapeHTML(name)}</option>`).join("")}</select></label>` : ""}${canEdit ? `<label class="field-inline">Дата для выставления<input id="matrix-date" type="date" value="${escapeHTML(selectedDate)}" required></label>` : ""}<button class="btn secondary" id="grade-export">Экспорт CSV</button>${canEdit ? `<button class="btn" id="matrix-add-grade">Добавить оценку</button>` : ""}</div><p class="muted journal-help">${context.subject_id ? "Оценки по выбранному предмету." : "Выберите предмет для отдельного просмотра."} В одной ячейке показаны все оценки за день. Средний балл — арифметический, без округления до итоговой оценки.${canEdit ? " Нажмите + в ячейке, чтобы выставить оценку." : ""}</p><div id="matrix-grade-editor"></div>${renderGradeMatrix(grades, students, subject, { canEdit, extraDates: canEdit && selectedDate ? [selectedDate] : [] })}`;
        bindTableSearch(output);
        const mayCloseEditor = () => !output.querySelector('#matrix-grade-editor [data-saving="true"]') && (!output.querySelector('#matrix-grade-editor [data-journal-dirty="true"]') || confirm("Есть несохранённая оценка. Продолжить без сохранения?"));
        output.querySelector("#matrix-subject")?.addEventListener("change", event => { if (!mayCloseEditor()) { event.target.value = subject; return; } subject = event.target.value; drawView(); });
        output.querySelector("#matrix-date")?.addEventListener("change", event => {
          if (!event.target.value) { event.target.value = selectedDate; return; }
          if (!mayCloseEditor()) { event.target.value = selectedDate; return; }
          selectedDate = event.target.value; drawView();
        });
        const openEditor = (studentID = "", date = selectedDate) => {
          if (!mayCloseEditor()) return;
          const editor = output.querySelector("#matrix-grade-editor");
          editor.innerHTML = `<div class="matrix-editor-heading"><h3>Выставить оценку</h3><button type="button" class="text-link" id="matrix-cancel-grade">Закрыть</button></div>${newGradeForm(students, { student_id: studentID, subject_id: context.subject_id || subject, work_date: date }, subjectContexts)}`;
          editor.querySelector("#matrix-cancel-grade").addEventListener("click", () => { if (mayCloseEditor()) editor.innerHTML = ""; });
          bindMatrixGrade(editor, context, user, availableContexts, { subject, date: selectedDate });
          editor.querySelector('[name="value"]')?.focus({ preventScroll: true });
          editor.scrollIntoView({ block: "nearest" });
        };
        output.querySelectorAll("[data-matrix-student]").forEach(button => button.addEventListener("click", () => openEditor(button.dataset.matrixStudent, button.dataset.matrixDate)));
        output.querySelector("#matrix-add-grade")?.addEventListener("click", () => openEditor());
      }
      output.querySelectorAll("[data-view]").forEach(button => button.addEventListener("click", () => { if (view === button.dataset.view) return; if (output.querySelector('[data-journal-dirty="true"]') && !confirm("Есть несохранённые оценки. Сменить представление?")) return; view = button.dataset.view; drawView(); }));
      output.querySelector("#grade-export")?.addEventListener("click", exportGrades);
    };
    const exportGrades = () => {
      const visible = view === "matrix" && subject ? grades.filter(grade => grade.subject_id === subject) : grades;
      toCSV("grades.csv", [["Ученик", "Предмет", "Дата", "Оценка", "Тип работы", "Комментарий"], ...visible.map(row => [row.student_name, row.subject_name, row.work_date, row.value, gradeTypeLabel(row.grade_type), row.comment])]);
    };
    drawView();
  } catch (error) {
    if (output.isConnected && version === gradeRequest) status.textContent = error.message;
  }
}

function newGradeForm(students, draft = null, subjectContexts = []) {
  if (!students.length) return `<p class="empty">В выбранном классе нет активных учеников.</p>`;
  if (!subjectContexts.length) return `<p class="empty">Для выбранного класса и периода не найдено доступных предметов.</p>`;
  const subjectOptions = `<label class="field">Предмет<select name="subject_id" required>${subjectContexts.map(item => `<option value="${escapeHTML(item.subject_id)}" ${item.subject_id === (draft?.subject_id || "") ? "selected" : ""}>${escapeHTML(item.subject_name)}</option>`).join("")}</select></label>`;
  return `<details class="card new-grade" open><summary>Быстро добавить оценку</summary><form id="new-grade-form" class="form-grid">
    <label class="field field-wide">Найти ученика<input type="search" data-student-search placeholder="Начните вводить фамилию или имя"></label>
    <label class="field">Ученик<select name="student_id" required>${students.map(student => `<option value="${escapeHTML(student.id)}" ${draft?.student_id === student.id ? "selected" : ""}>${escapeHTML(student.full_name)}</option>`).join("")}</select></label>
    ${subjectOptions}
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

function bindStudentSearch(output) {
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
}

function bindMatrixGrade(editor, context, user, availableContexts, matrixState) {
  bindStudentSearch(editor);
  const form = editor.querySelector("#new-grade-form");
  form?.addEventListener("submit", async event => {
    event.preventDefault();
    if (form.dataset.saving) return;
    const values = new FormData(form);
    const subjectID = values.get("subject_id");
    const workDate = values.get("work_date");
    const error = editor.querySelector("#new-grade-error");
    const controls = [...editor.querySelectorAll("button,input,select")].map(control => [control, control.disabled]);
    const submit = form.querySelector('button[type="submit"], button:not([type])');
    const originalText = submit.textContent;
    form.dataset.saving = "true";
    controls.forEach(([control]) => { control.disabled = true; });
    submit.textContent = "Сохраняю…";
    error.textContent = "";
    try {
      await api("/grades", { method: "POST", body: JSON.stringify({
        student_id: values.get("student_id"), class_id: context.class_id, subject_id: subjectID,
        term_id: context.term_id, work_date: workDate, value: Number(values.get("value")),
        grade_type: values.get("grade_type"), comment: values.get("comment"),
      }) });
      if (!editor.isConnected) return;
      const nextIndex = availableContexts.findIndex(item => item.class_id === context.class_id && item.term_id === context.term_id && item.subject_id === subjectID);
      const nextContext = availableContexts[nextIndex] || context;
      if (nextIndex >= 0) document.querySelector("#grade-context").value = String(nextIndex);
      await loadContext(nextContext, user, null, "Оценка добавлена", availableContexts, { ...matrixState, date: workDate, subject: nextContext.subject_id ? "" : subjectID });
    } catch (reason) {
      error.textContent = reason.status === 409 ? "На эту дату уже есть оценка за такой тип работы. Выберите другой тип работы или измените существующую оценку в режиме «Список»." : reason.message;
    } finally {
      delete form.dataset.saving;
      controls.forEach(([control, disabled]) => { control.disabled = disabled; });
      submit.textContent = originalText;
    }
  });
}

function bindGradeEdits(output, context, availableContexts = []) {
  bindStudentSearch(output);
  document.querySelector("#new-grade-form")?.addEventListener("submit", async event => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const error = document.querySelector("#new-grade-error");
    error.textContent = "";
    try {
      const subjectID = form.get("subject_id") || context.subject_id;
      await api("/grades", { method: "POST", body: JSON.stringify({
        student_id: form.get("student_id"), class_id: context.class_id, subject_id: subjectID,
        term_id: context.term_id, work_date: form.get("work_date"), value: Number(form.get("value")),
        grade_type: form.get("grade_type"), comment: form.get("comment"),
      }) });
      const subjectContextIndex = availableContexts.findIndex(item => item.class_id === context.class_id
        && item.term_id === context.term_id && item.subject_id === subjectID);
      const subjectContext = availableContexts[subjectContextIndex] || context;
      const contextSelector = document.querySelector("#grade-context");
      if (subjectContextIndex >= 0 && contextSelector) contextSelector.value = String(subjectContextIndex);
      await loadContext(subjectContext, window.currentUser, {
        student_id: form.get("student_id"), subject_id: subjectID, work_date: form.get("work_date"), value: null,
        grade_type: form.get("grade_type"), comment: "",
      }, "Оценка добавлена", availableContexts);
    } catch (reason) { error.textContent = reason.message; }
  });
  output.querySelectorAll("[data-save-grade]").forEach(button => button.addEventListener("click", async () => {
    const row = button.closest("tr");
    const body = Object.fromEntries(new FormDataRow(row).entries());
    try {
      await api(`/grades/${encodeURIComponent(row.dataset.grade)}`, { method: "PATCH", body: JSON.stringify({
        work_date: body.work_date, value: Number(body.value), grade_type: body.grade_type, comment: body.comment,
      }) });
      await loadContext(context, window.currentUser, null, "Изменения сохранены", availableContexts);
    } catch (error) { output.querySelector("#grade-action-status").textContent = error.message; }
  }));
  output.querySelectorAll("[data-delete-grade]").forEach(button => button.addEventListener("click", async () => {
    const row = button.closest("tr");
    if (!confirm("Удалить эту оценку? Она останется в аудите.")) return;
    try {
      await api(`/grades/${encodeURIComponent(row.dataset.grade)}`, { method: "DELETE" });
      await loadContext(context, window.currentUser, null, "Оценка удалена", availableContexts);
    } catch (error) { output.querySelector("#grade-action-status").textContent = error.message; }
  }));
}

class FormDataRow {
  constructor(row) { this.row = row; }
  entries() { return [...this.row.querySelectorAll("input,select")].map(field => [field.name, field.value]); }
}
