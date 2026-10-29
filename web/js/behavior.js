import { api, apiBlob, saveBlob } from "./api.js";
import { escapeHTML, errorBox, notice, renderPage, showNoContexts } from "./ui.js";

export async function showBehavior(user) {
  renderPage("Ведомость поведения", `<h1>Ведомость поведения</h1><p class="sub">Отдельная ведомость: проект, 10 критериев, итог и оценка.</p>
    ${notice()}<div class="rubric-scale" aria-label="Шкала оценки поведения"><b>Шкала:</b><span>0–3 балла за каждый из 10 критериев</span><span class="status green">24–30 · образцовое</span><span class="status yellow">10–23 · допустимое</span><span class="status red">0–9 · недопустимое</span></div>
    <div class="toolbar"><label class="field-inline">Класс и период<select id="behavior-context" disabled><option>Загрузка…</option></select></label>
    <button class="btn secondary" id="behavior-refresh">Обновить</button><button class="btn secondary" id="behavior-export">Экспорт CSV</button>
    ${user.roles.includes("admin") ? `<button class="btn" id="behavior-generate">Создать ведомости класса</button>` : ""}</div>
    <div id="behavior-status" class="muted" aria-live="polite"></div><div id="behavior-result" class="table"></div>`);
  document.querySelector("#behavior-refresh").onclick = () => showBehavior(user);
  try {
    const [contexts, criteria] = await Promise.all([api("/behavior/contexts"), api("/behavior/criteria")]);
    if (!contexts.length || !criteria.length) {
      if (!contexts.length) {
        showNoContexts(document.querySelector("#behavior-status"), user, document.querySelector("#behavior-context"), "Добавьте учебный год, класс и период в настройках школы.");
        document.querySelector("#behavior-export").disabled = true;
        document.querySelector("#behavior-generate")?.setAttribute("disabled", "");
      }
      else {
        const selector = document.querySelector("#behavior-context");
        selector.replaceChildren(new Option("Критерии не настроены", ""));
        selector.disabled = true;
        document.querySelector("#behavior-export").disabled = true;
        document.querySelector("#behavior-generate")?.setAttribute("disabled", "");
        document.querySelector("#behavior-status").textContent = "Для школы не настроены критерии поведения.";
      }
      return;
    }
    const selector = document.querySelector("#behavior-context");
    selector.innerHTML = contexts.map((item, index) => `<option value="${index}">${escapeHTML(item.class_name)} · ${escapeHTML(item.term_name)}</option>`).join("");
    selector.disabled = false;
    let selectedContext = selector.value;
    const hasUnsavedScores = () => Boolean(document.querySelector('#behavior-result tr[data-changed="true"]'));
    const load = () => loadSheets(contexts[Number(selector.value)], criteria);
    selector.addEventListener("change", () => {
      if (hasUnsavedScores() && !confirm("Есть несохранённые баллы. Сменить класс или период?")) {
        selector.value = selectedContext;
        return;
      }
      selectedContext = selector.value;
      load();
    });
    document.querySelector("#behavior-refresh").onclick = () => {
      if (hasUnsavedScores() && !confirm("Есть несохранённые баллы. Обновить ведомость?")) return;
      load();
    };
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
    const selector = document.querySelector("#behavior-context");
    selector.disabled = true;
    selector.replaceChildren(new Option("Не удалось загрузить список", ""));
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
    const header = ["№", "ФИО", "Проект", ...criteria.map(item => item.title), "Итого", "Оценка", "Действие"];
    const rows = sheets.map((sheet, index) => `<tr data-behavior-sheet="${escapeHTML(sheet.id)}" data-editable="${sheet.can_edit ? "true" : "false"}">
      <td>${index + 1}</td><td>${escapeHTML(sheet.student_name)}</td><td>${escapeHTML(sheet.project || "—")}</td>
      ${criteria.map(item => scoreCell(sheet, item)).join("")}
      <td><b>${sheet.is_complete ? `${sheet.total_score}/30` : `${sheet.scores_count}/10 заполнено`}</b></td>
      <td data-sheet-verdict>${sheet.is_complete ? `<span class="status ${sheet.total_score >= 24 ? "green" : sheet.total_score >= 10 ? "yellow" : "red"}">${escapeHTML(verdictLabel(sheet.verdict))}</span>` : `<span class="status neutral">Не заполнено</span>`}</td>
      <td><div class="behavior-actions">${sheet.can_edit ? `<button class="btn" data-save-sheet>Сохранить</button>` : ""}<button class="btn secondary" data-open-sheet="${escapeHTML(sheet.id)}">Подробнее</button></div><span class="behavior-row-status" role="status" aria-live="polite"></span></td></tr>`).join("");
    const mobileRows = sheets.map(sheet => `<article class="card behavior-student-card"><div class="card-heading"><h2>${escapeHTML(sheet.student_name)}</h2>${sheet.is_complete ? `<span class="status ${sheet.total_score >= 24 ? "green" : sheet.total_score >= 10 ? "yellow" : "red"}">${escapeHTML(verdictLabel(sheet.verdict))}</span>` : `<span class="status neutral">Не заполнено</span>`}</div>
      <p class="muted">Проект: ${escapeHTML(sheet.project || "не указан")}</p><div class="behavior-student-total"><b>${sheet.is_complete ? `${sheet.total_score}/30` : `${sheet.scores_count}/10 критериев`}</b><button class="btn secondary" data-open-sheet="${escapeHTML(sheet.id)}">${sheet.can_edit ? "Оценить" : "Просмотр"}</button></div></article>`).join("");
    output.innerHTML = `<div class="grade-tools behavior-search"><label class="search-field"><span>Найти ученика</span><input type="search" data-behavior-search placeholder="Фамилия или имя"></label></div><div class="wide-table behavior-desktop"><table><thead><tr>${header.map((value, index) => `<th ${index >= 3 && index < 3 + criteria.length ? `class="criterion-column" title="${escapeHTML(value)}"` : ""}>${escapeHTML(value)}</th>`).join("")}</tr></thead><tbody>${rows}</tbody></table></div><div class="behavior-mobile">${mobileRows}</div>`;
    output.querySelector("[data-behavior-search]").addEventListener("input", event => {
      const query = event.currentTarget.value.trim().toLocaleLowerCase("ru");
      output.querySelectorAll(".behavior-desktop tbody tr").forEach(row => { row.hidden = query !== "" && !row.cells[1].textContent.toLocaleLowerCase("ru").includes(query); });
      output.querySelectorAll(".behavior-mobile .behavior-student-card").forEach(card => { card.hidden = query !== "" && !card.textContent.toLocaleLowerCase("ru").includes(query); });
    });
    output.querySelectorAll("[data-save-sheet]").forEach(button => button.addEventListener("click", () => saveScoreRow(button.closest("tr"), context, criteria)));
    output.querySelectorAll("[data-open-sheet]").forEach(button => button.addEventListener("click", () => {
      const row = button.closest("tr");
      if (row?.dataset.changed && !confirm("В этой строке есть несохранённые баллы. Открыть подробности без сохранения?")) return;
      openSheet(button.dataset.openSheet, context, criteria);
    }));
    output.querySelectorAll("[data-score-input]").forEach(select => select.addEventListener("change", () => {
      const row = select.closest("tr");
      row.dataset.changed = "true";
      row.querySelector("[data-save-sheet]").disabled = false;
      row.querySelector(".behavior-row-status").textContent = "Есть несохранённые изменения";
      updateScorePreview(row, criteria);
    }));
  } catch (error) {
    status.textContent = error.message;
  }
}

function scoreCell(sheet, criterion) {
  if (!sheet.can_edit) return `<td class="score-cell">${sheet.scores[criterion.id] ?? "—"}</td>`;
  const current = sheet.scores[criterion.id];
  const options = Array.from({ length: criterion.max_score + 1 }, (_, value) => `<option value="${value}" ${Number(current) === value && current !== null && current !== undefined ? "selected" : ""}>${value}</option>`).join("");
  return `<td class="score-cell"><select data-score-input data-criterion="${escapeHTML(criterion.id)}" aria-label="${escapeHTML(criterion.title)}"><option value="" ${current === null || current === undefined ? "selected" : ""}>—</option>${options}</select></td>`;
}

function updateScorePreview(row, criteria) {
  const values = [...row.querySelectorAll("[data-score-input]")].map(input => input.value);
  const count = values.filter(value => value !== "").length;
  const totalCell = row.cells[3 + criteria.length];
  const verdictCell = row.cells[4 + criteria.length];
  if (count === criteria.length) {
    const total = values.reduce((sum, value) => sum + Number(value), 0);
    totalCell.innerHTML = `<b>${total}/30</b>`;
    const type = total >= 24 ? "green" : total >= 10 ? "yellow" : "red";
    const verdict = total >= 24 ? "образцовое" : total >= 10 ? "допустимое" : "недопустимое";
    verdictCell.innerHTML = `<span class="status ${type}">${verdict} поведение</span>`;
  } else {
    totalCell.innerHTML = `<b>${count}/10 заполнено</b>`;
    verdictCell.innerHTML = `<span class="status neutral">Не заполнено</span>`;
  }
}

async function saveScoreRow(row, context, criteria) {
  const button = row.querySelector("[data-save-sheet]");
  const status = row.querySelector(".behavior-row-status");
  button.disabled = true;
  status.textContent = "Сохраняю…";
  const scores = [...row.querySelectorAll("[data-score-input]")].map(input => ({
    criterion_id: input.dataset.criterion,
    score: input.value === "" ? null : Number(input.value),
  }));
  try {
    await api(`/behavior/sheets/${encodeURIComponent(row.dataset.behaviorSheet)}/scores`, { method: "PATCH", body: JSON.stringify({ scores }) });
    await loadSheets(context, criteria);
    const updatedRow = document.querySelector(`#behavior-result [data-behavior-sheet="${CSS.escape(row.dataset.behaviorSheet)}"]`);
    if (updatedRow) {
      const updatedStatus = updatedRow.querySelector(".behavior-row-status");
      updatedStatus.textContent = "Сохранено";
      updatedStatus.className = "behavior-row-status success";
    }
  } catch (error) {
    button.disabled = false;
    status.textContent = error.message;
    status.className = "behavior-row-status error";
  }
}

async function openSheet(id, context, criteria) {
  try {
    const sheet = await api(`/behavior/sheets/${encodeURIComponent(id)}`);
    renderPage("Ведомость поведения", `<button class="btn secondary" id="behavior-back">← К ведомости класса</button><h1>${escapeHTML(sheet.student_name)}</h1>
      <p class="sub">Проект: ${escapeHTML(sheet.project || "не указан")} · ${sheet.is_complete ? `Итого ${sheet.total_score}/30 — ${escapeHTML(verdictLabel(sheet.verdict))}` : "Ведомость не заполнена"}</p>
      <div class="table"><table><thead><tr><th>Критерий</th><th>Балл</th></tr></thead><tbody>${sheet.scores.map(item => `<tr><td class="criteria">${escapeHTML(item.title)}</td><td>${item.score ?? "—"}/${item.max_score}</td></tr>`).join("")}</tbody></table></div>
      ${sheet.can_edit ? `<details class="card new-grade" open><summary>Заполнить или исправить баллы</summary><p class="muted">Выберите значение от 0 до ${sheet.scores[0]?.max_score ?? 3}. Пустое значение оставляет критерий без оценки.</p><form id="behavior-score-form" class="score-form">${sheet.scores.map(item => `<label class="field"><span>${escapeHTML(item.title)} <small>0–${item.max_score}</small></span><select name="${escapeHTML(item.criterion_id)}" aria-label="${escapeHTML(item.title)}"><option value="">Не оценено</option>${Array.from({ length: item.max_score + 1 }, (_, value) => `<option value="${value}" ${Number(item.score) === value && item.score !== null ? "selected" : ""}>${value}</option>`).join("")}</select></label>`).join("")}<button class="btn">Сохранить оценки</button><span id="behavior-error" class="error"></span></form></details>` : ""}
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

function verdictLabel(value) {
  const labels = { образцовое: "Образцовое поведение", допустимое: "Допустимое поведение", недопустимое: "Недопустимое поведение" };
  return labels[value] || value || "Оценка не выставлена";
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
