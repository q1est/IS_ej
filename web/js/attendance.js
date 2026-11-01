import { api } from "./api.js";
import { escapeHTML, localDate, notice, renderPage, showNoContexts } from "./ui.js";

const labels = { present: "Присутствует", absent: "Отсутствует", late: "Опоздал", excused: "Уважительная причина" };

export async function showAttendance(user) {
  renderPage("Посещаемость", `<h1>Посещаемость</h1><p class="sub">Ежедневная ведомость класса. Изменения сохраняются одной операцией и записываются в аудит.</p>${notice()}
    <div class="toolbar attendance-toolbar"><label class="field-inline">Класс<select id="attendance-class" disabled><option>Загрузка…</option></select></label><label class="field-inline">Дата<input id="attendance-date" type="date" value="${escapeHTML(localDate())}"></label><button class="btn secondary" id="attendance-load">Открыть</button><button class="btn secondary" id="attendance-all-present">Все присутствуют</button><button class="btn" id="attendance-save">Сохранить отметки</button></div>
    <p id="attendance-status" class="muted" role="status" aria-live="polite">Загружаю классы…</p><div id="attendance-result" class="table"></div>`);
  const selector = document.querySelector("#attendance-class");
  const date = document.querySelector("#attendance-date");
  const status = document.querySelector("#attendance-status");
  const output = document.querySelector("#attendance-result");
  const save = document.querySelector("#attendance-save");
  const allPresent = document.querySelector("#attendance-all-present");
  const mayEdit = user.roles.includes("admin") || user.roles.includes("teacher") || user.roles.includes("class_teacher");
  save.hidden = !mayEdit;
  allPresent.hidden = !mayEdit;
  date.disabled = !mayEdit;
  try {
    const contexts = await api("/attendance/contexts");
    if (!contexts.length) { showNoContexts(status, user, selector); save.disabled = true; allPresent.disabled = true; return; }
    selector.innerHTML = contexts.map((item, index) => `<option value="${index}">${escapeHTML(item.class_name)} · ${escapeHTML(item.academic_year_name)}</option>`).join("");
    selector.disabled = false;
    const context = () => contexts[Number(selector.value)];
    const load = async () => {
      const selected = context();
      const day = date.value || localDate();
      status.textContent = "Загружаю список класса…";
      output.innerHTML = "";
      try {
        const data = await api(`/attendance?${new URLSearchParams({ class_id: selected.class_id, date: day })}`);
        const editable = mayEdit && data.can_edit;
        save.hidden = !editable;
        allPresent.hidden = !editable;
        status.textContent = `${data.students.length} учеников · ${new Intl.DateTimeFormat("ru-RU", { dateStyle: "long" }).format(new Date(`${day}T00:00:00`))}${editable ? " · отметки можно изменить" : " · только просмотр"}`;
        if (!data.students.length) { output.innerHTML = `<p class="empty">На эту дату нет активных зачислений.</p>`; return; }
        output.innerHTML = `<table><thead><tr><th>№</th><th>Ученик</th><th>Статус</th><th>Примечание</th></tr></thead><tbody>${data.students.map((student, index) => `<tr data-student-id="${escapeHTML(student.student_id)}"><td>${index + 1}</td><td>${escapeHTML(student.student_name)}</td><td>${editable ? `<select name="status" aria-label="Посещаемость: ${escapeHTML(student.student_name)}"><option value="">Не отмечено</option>${Object.entries(labels).map(([value, label]) => `<option value="${value}" ${student.status === value ? "selected" : ""}>${label}</option>`).join("")}</select>` : escapeHTML(labels[student.status] || "Не отмечено")}</td><td>${editable ? `<input name="note" maxlength="500" value="${escapeHTML(student.note)}" aria-label="Примечание: ${escapeHTML(student.student_name)}">` : escapeHTML(student.note || "—")}</td></tr>`).join("")}</tbody></table>`;
      } catch (error) { status.textContent = error.message; }
    };
    selector.addEventListener("change", load);
    date.addEventListener("change", load);
    document.querySelector("#attendance-load").addEventListener("click", load);
    allPresent.addEventListener("click", () => output.querySelectorAll('select[name="status"]').forEach(select => { select.value = "present"; }));
    save.addEventListener("click", async () => {
      const records = [...output.querySelectorAll("tbody tr[data-student-id]")].map(row => ({ student_id: row.dataset.studentId, status: row.querySelector('[name="status"]').value, note: row.querySelector('[name="note"]').value })).filter(item => item.status);
      if (!records.length) { status.textContent = "Сначала отметьте посещаемость хотя бы одному ученику."; return; }
      save.disabled = true;
      status.textContent = "Сохраняю…";
      try {
        const result = await api("/attendance", { method: "PUT", body: JSON.stringify({ class_id: context().class_id, date: date.value, records }) });
        status.textContent = `Сохранено отметок: ${result.saved}.`;
        await load();
      } catch (error) { status.textContent = error.message; }
      finally { save.disabled = false; }
    });
    await load();
  } catch (error) {
    selector.disabled = true;
    selector.replaceChildren(new Option("Не удалось загрузить классы", ""));
    status.textContent = error.message;
  }
}
