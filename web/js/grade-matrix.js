import { escapeHTML } from "./ui.js";

export function buildGradeMatrix(grades, students = []) {
  const rows = new Map(students.map(student => [student.id, { id: student.id, name: student.full_name, dates: new Map(), values: [] }]));
  const dates = [...new Set(grades.map(grade => grade.work_date))].sort();
  for (const grade of grades) {
    if (!rows.has(grade.student_id)) rows.set(grade.student_id, { id: grade.student_id, name: grade.student_name, dates: new Map(), values: [] });
    const row = rows.get(grade.student_id);
    if (!row.dates.has(grade.work_date)) row.dates.set(grade.work_date, []);
    row.dates.get(grade.work_date).push(grade);
    row.values.push(Number(grade.value));
  }
  return { dates, rows: [...rows.values()].sort((a, b) => a.name.localeCompare(b.name, "ru")).map(row => ({ ...row, average: row.values.length ? row.values.reduce((sum, value) => sum + value, 0) / row.values.length : null })) };
}

export function renderGradeMatrix(grades, students = [], subjectID = "", options = {}) {
  const filtered = subjectID ? grades.filter(grade => grade.subject_id === subjectID) : grades;
  const matrix = buildGradeMatrix(filtered, students);
  matrix.dates = [...new Set([...matrix.dates, ...(options.extraDates || [])])].sort();
  const editableStudents = new Set(students.map(student => student.id));
  if (!matrix.rows.length) return `<p class="empty">Оценок пока нет.</p>`;
  const dateTitle = date => new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "long", year: "numeric" }).format(new Date(`${date}T00:00:00`));
  return `<div class="journal-scroll"><table class="journal-matrix"><caption class="sr-only">Журнал оценок: ученики, даты и средний балл</caption><thead><tr><th scope="col">Ученик</th>${matrix.dates.map(date => `<th scope="col" title="${escapeHTML(dateTitle(date))}"><time datetime="${escapeHTML(date)}">${escapeHTML(date.slice(5).split("-").reverse().join("."))}</time></th>`).join("")}<th scope="col">Средний балл</th></tr></thead><tbody>${matrix.rows.map(row => `<tr><th scope="row">${escapeHTML(row.name)}</th>${matrix.dates.map(date => `<td><div class="journal-marks">${(row.dates.get(date) || []).map(grade => `<span class="grade-mark grade-${Number(grade.value)}" tabindex="0" title="${escapeHTML([grade.subject_name, dateTitle(date), grade.comment].filter(Boolean).join(" · "))}" aria-label="${escapeHTML(`${grade.subject_name || "Оценка"}: ${grade.value}, ${dateTitle(date)}${grade.comment ? `, ${grade.comment}` : ""}`)}">${escapeHTML(grade.value)}</span>`).join("") || (options.canEdit && editableStudents.has(row.id) ? "" : `<span class="journal-no-grade" aria-label="Нет оценки">—</span>`)}${options.canEdit && editableStudents.has(row.id) ? `<button type="button" class="journal-add-mark" data-matrix-student="${escapeHTML(row.id)}" data-matrix-date="${escapeHTML(date)}" aria-label="${escapeHTML(`Выставить оценку: ${row.name}, ${dateTitle(date)}`)}" title="Выставить оценку">+</button>` : ""}</div></td>`).join("")}<td class="journal-average">${row.average === null ? "—" : row.average.toLocaleString("ru-RU", { minimumFractionDigits: 1, maximumFractionDigits: 2 })}</td></tr>`).join("")}</tbody></table></div>${!matrix.dates.length ? `<p class="empty">Ученики добавлены. Оценки появятся здесь после выставления.</p>` : ""}`;
}
