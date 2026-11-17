import { readFile } from "node:fs/promises";
import assert from "node:assert/strict";
import { test } from "node:test";
const ui = await readFile(new URL("./ui.js", import.meta.url), "utf8");
const source = (await readFile(new URL("./classes.js", import.meta.url), "utf8"))
 .replace('import { api } from "./api.js";', 'const api = () => { throw new Error("No requests expected"); };')
 .replace('"./ui.js"', JSON.stringify(`data:text/javascript;base64,${Buffer.from(ui).toString("base64")}`));
const { parseStudentList } = await import(`data:text/javascript;base64,${Buffer.from(source).toString("base64")}`);
test("student list accepts numbered lists and skips blank lines", () => {
 const rows = parseStudentList("1. Иванов   Иван\n\n2) Петрова Анна\r\n");
 assert.deepEqual(rows.map(row=>row.name),["Иванов Иван","Петрова Анна"]);
 assert(rows.every(row=>!row.problem));
});
test("student list marks duplicate and existing names before saving", () => {
 const rows = parseStudentList("Иванов Иван\nиванов  Иван\nПетрова Анна", [{full_name:"петрова Анна"}]);
 assert.equal(rows[0].problem, "");
 assert.match(rows[1].problem, /Повтор/);
 assert.match(rows[2].problem, /существует/);
});
