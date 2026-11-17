import { readFile } from "node:fs/promises";
import assert from "node:assert/strict";
import { test } from "node:test";
const ui = await readFile(new URL("./ui.js", import.meta.url), "utf8");
const source = (await readFile(new URL("./grade-matrix.js", import.meta.url), "utf8")).replace('"./ui.js"', JSON.stringify(`data:text/javascript;base64,${Buffer.from(ui).toString("base64")}`));
const { buildGradeMatrix, renderGradeMatrix } = await import(`data:text/javascript;base64,${Buffer.from(source).toString("base64")}`);
const grades = [
 { student_id: "a", student_name: "Анна", subject_id: "math", subject_name: "Математика", work_date: "2026-11-02", value: 5, comment: "<script>" },
 { student_id: "a", student_name: "Анна", subject_id: "math", subject_name: "Математика", work_date: "2026-11-02", value: 3 },
 { student_id: "b", student_name: "Борис", subject_id: "ru", subject_name: "Русский язык", work_date: "2026-11-01", value: 2 },
];
test("matrix keeps multiple marks per day, sorts dates, and includes students with no marks", () => {
 const matrix = buildGradeMatrix(grades, [{ id: "c", full_name: "Вера" }]);
 assert.deepEqual(matrix.dates, ["2026-11-01", "2026-11-02"]);
 assert.deepEqual(matrix.rows.map(row => row.name), ["Анна", "Борис", "Вера"]);
 assert.equal(matrix.rows[0].dates.get("2026-11-02").length, 2);
 assert.equal(matrix.rows[0].average, 4);
 assert.equal(matrix.rows[2].average, null);
});
test("matrix escapes comments and filters by subject without exposing other marks", () => {
 const html = renderGradeMatrix(grades, [], "math");
 assert.match(html, /&lt;script&gt;/);
 assert.doesNotMatch(html, /<script>|Борис|Русский язык/);
 assert.match(html, /4,0/);
});
test("empty matrix keeps roster and distinguishes ungraded students", () => {
 assert.match(renderGradeMatrix([], [{id:"a",full_name:"Анна"}]), /Оценки появятся/);
 assert.match(renderGradeMatrix([], []), /Оценок пока нет/);
});
