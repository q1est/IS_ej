import { api, apiBlob, saveBlob } from "./api.js";
import { escapeHTML, errorBox, notice, renderPage } from "./ui.js";

export async function showBehavior(user) {
  renderPage("Ведомость поведения", `<h1>Ведомость поведения</h1><p class="sub">Отдельная ведомость: проект, 10 критериев, итог и оценка.</p>
    ${notice()}<div class="toolbar"><label class="field-inline">Класс и период<select id="behavior-context"><option>Загрузка…</option></select></label>
    <button class="btn secondary" id="behavior-refresh">Обновить</button><button class="btn secondary" id="behavior-export">Экспорт CSV</button>
    ${user.roles.includes("admin") ? `<button class="btn" id="behavior-generate">Создать ведомости класса</button>` : ""}</div>
    <div id="behavior-status" class="muted"></div><div id="behavior-result" class="table"></div>`);
  try {
    const [contexts, criteria] = await Promise.all([api("/behavior/contexts"), api("/behavior/criteria")]);
    if (!contexts.length || !criteria.length) {
      document.querySelector("#behavior-status").textContent = contexts.length ? "Критерии ещё не настроены." : "Нет доступных классов и периодов.";
      return;
    }
    const selector = document.querySelector("#behavior-context");
    selector.innerHTML = contexts.map((item, index) => `<option value="${index}">${escapeHTML(item.class_name)} · ${escapeHTML(item.term_name)}</option>`).join("");
    const load = () => loadSheets(contexts[Number(selector.value)], criteria);
    selector.addEventListener("change", load);
    document.querySelector("#behavior-refresh").onclick = load;
    document.querySelector("#behavior-export").onclick = () => exportSheets(contexts[Number(selector.value)], criteria);
    document.querySelector("#behavior-generate")?.addEventListener("click", async () => {
      const context = contexts[Number(selector.value)];
      if (!confirm(`Создать пустые ведомости для активных учеников класса «${context.class_name}»?`)) return;
      try {
        const result = await api("/behavior/sheets/generate", { method: "POST", body: JSON.stringify({ class_id: context.class_id, academic_year_id: context.academic_year_id, term_id: context.term_id }) });
        alert(`Создано ведомостей: ${result.created}`);
        await load();
      } catch (error) { alert(error.message); }
    });
    await load();
  } catch (error) {
    document.querySelector("#behavior-status").textContent = error.message;
  }
}

async function loadSheets(context, criteria) {
  const status = document.querySelector("#behavior-status");
  const output = document.querySelector("#behavior-result");
  status.textContent = "Загружаю ведомости…";
  output.innerHTML = "";
  try {
    const params = new URLSearchParams({ class_id: context.class_id, term_id: context.term_id });
    const sheets = await api(`/behavior/sheets?${params}`);
    status.textContent = sheets.length ? `Учеников с ведомостями: ${sheets.length}` : "Ведомости для этого периода ещё не созданы.";
    if (!sheets.length) {
      output.innerHTML = `<p class="empty">${escapeHTML(status.textContent)}</p>`;
      return;
    }
    const header = ["№", "ФИО", "Проект", ...criteria.map(item => item.title), "Итого", "Оценка", "Статус", "Действие"];
    const rows = sheets.map((sheet, index) => `<tr>
      <td>${index + 1}</td><td>${escapeHTML(sheet.student_name)}</td><td>${escapeHTML(sheet.project || "—")}</td>
      ${criteria.map(item => `<td class="score-cell">${sheet.scores[item.id] ?? "—"}</td>`).join("")}
      <td><b>${sheet.is_complete ? `${sheet.total_score}/30` : `${sheet.scores_count}/10 заполнено`}</b></td>
      <td>${sheet.is_complete ? `<span class="status ${sheet.total_score >= 24 ? "green" : sheet.total_score >= 10 ? "yellow" : "red"}">${escapeHTML(sheet.verdict)}</span>` : `<span class="status neutral">Не заполнено</span>`}</td>
      <td><button class="btn secondary" data-open-sheet="${escapeHTML(sheet.id)}">${sheet.can_edit ? "Оценить" : "Просмотр"}</button></td></tr>`).join("");
    output.innerHTML = `<div class="wide-table"><table><thead><tr>${header.map(value => `<th>${escapeHTML(value)}</th>`).join("")}</tr></thead><tbody>${rows}</tbody></table></div>`;
    output.querySelectorAll("[data-open-sheet]").forEach(button => button.addEventListener("click", () => openSheet(button.dataset.openSheet, context, criteria)));
  } catch (error) {
    status.textContent = error.message;
  }
}

async function openSheet(id, context, criteria) {
  try {
    const sheet = await api(`/behavior/sheets/${encodeURIComponent(id)}`);
    renderPage("Ведомость поведения", `<button class="btn secondary" id="behavior-back">← К ведомости класса</button><h1>${escapeHTML(sheet.student_name)}</h1>
      <p class="sub">Проект: ${escapeHTML(sheet.project || "не указан")} · ${sheet.is_complete ? `Итого ${sheet.total_score}/30 — ${escapeHTML(sheet.verdict)}` : "Ведомость не заполнена"}</p>
      <div class="table"><table><thead><tr><th>Критерий</th><th>Балл</th></tr></thead><tbody>${sheet.scores.map(item => `<tr><td class="criteria">${escapeHTML(item.title)}</td><td>${item.score ?? "—"}/${item.max_score}</td></tr>`).join("")}</tbody></table></div>
      ${sheet.can_edit ? `<details class="card new-grade" open><summary>Заполнить или исправить баллы</summary><form id="behavior-score-form" class="score-form">${sheet.scores.map(item => `<label class="field"><span>${escapeHTML(item.title)}</span><select name="${escapeHTML(item.criterion_id)}"><option value="">Не оценено</option>${Array.from({ length: item.max_score + 1 }, (_, value) => `<option value="${value}" ${Number(item.score) === value && item.score !== null ? "selected" : ""}>${value}</option>`).join("")}</select></label>`).join("")}<button class="btn">Сохранить оценки</button><span id="behavior-error" class="error"></span></form></details>` : ""}
      <div class="toolbar"><button class="btn secondary" id="behavior-sheet-export">Экспорт ведомости</button></div>`);
    document.querySelector("#behavior-back").onclick = () => showBehavior(window.currentUser);
    document.querySelector("#behavior-sheet-export").onclick = async () => {
      try { saveBlob(await apiBlob(`/behavior/sheets/${encodeURIComponent(id)}/export.csv`), "behavior.csv"); }
      catch (error) { alert(error.message); }
    };
    document.querySelector("#behavior-score-form")?.addEventListener("submit", async event => {
      event.preventDefault();
      const scores = [...new FormData(event.currentTarget).entries()].map(([criterion_id, value]) => ({ criterion_id, score: value === "" ? null : Number(value) }));
      try {
        await api(`/behavior/sheets/${encodeURIComponent(id)}/scores`, { method: "PATCH", body: JSON.stringify({ scores }) });
        await openSheet(id, context, criteria);
      } catch (error) { document.querySelector("#behavior-error").textContent = error.message; }
    });
  } catch (error) {
    renderPage("Ошибка", errorBox(error.message));
  }
}

async function exportSheets(context, criteria) {
  try {
    const params = new URLSearchParams({ class_id: context.class_id, term_id: context.term_id });
    const sheets = await api(`/behavior/sheets?${params}`);
    const rows = [["№", "ФИО", "Проект", ...criteria.map(item => item.title), "Итого", "Оценка"]];
    sheets.forEach((sheet, index) => rows.push([index + 1, sheet.student_name, sheet.project, ...criteria.map(item => sheet.scores[item.id] ?? ""), sheet.is_complete ? sheet.total_score : "", sheet.verdict || "Не заполнено"]));
    const safe = value => { const text = String(value ?? ""); return /^[\s]*[=+\-@]/.test(text) ? `'${text}` : text; };
    const text = "\uFEFF" + rows.map(row => row.map(value => `"${safe(value).replaceAll('"', '""')}"`).join(";")).join("\r\n");
    saveBlob(new Blob([text], { type: "text/csv;charset=utf-8" }), "behavior-class.csv");
  } catch (error) { alert(error.message); }
}
