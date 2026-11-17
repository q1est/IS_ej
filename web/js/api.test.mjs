import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";

const readinessSource = await readFile(new URL("./server-readiness.js", import.meta.url), "utf8");
const readinessURL = `data:text/javascript;base64,${Buffer.from(readinessSource).toString("base64")}`;
const source = (await readFile(new URL("./api.js", import.meta.url), "utf8")).replace('"./server-readiness.js"', JSON.stringify(readinessURL));
let moduleNumber = 0;

function createStorage(initial = {}) {
  const values = new Map(Object.entries(initial));
  return {
    getItem(key) { return values.get(key) ?? null; },
    setItem(key, value) { values.set(key, String(value)); },
    removeItem(key) { values.delete(key); },
  };
}

async function loadApi(t, { storage, fetchImpl, healthImpl }) {
  const previous = {
    window: globalThis.window,
    sessionStorage: globalThis.sessionStorage,
    fetch: globalThis.fetch,
  };
  t.after(() => {
    for (const [key, value] of Object.entries(previous)) {
      if (value === undefined) delete globalThis[key];
      else globalThis[key] = value;
    }
  });

  globalThis.sessionStorage = storage;
  globalThis.window = {
    APP_CONFIG: { API_BASE_URL: "https://api.test" },
    events: [],
    dispatchEvent(event) { this.events.push(event); },
  };
  if (fetchImpl) globalThis.fetch = (url, options) => String(url).endsWith("/healthz")
    ? healthImpl ? healthImpl(url, options) : Promise.resolve(Response.json({ status: "ok" }))
    : fetchImpl(url, options);
  const moduleURL = `data:text/javascript;base64,${Buffer.from(source).toString("base64")}#test-${moduleNumber++}`;
  return import(moduleURL);
}

test("restores the session and current user after a page reload", async t => {
  const user = { id: "user-1", full_name: "Анна Петрова", roles: ["admin"] };
  const storage = createStorage({
    "school-journal-session": JSON.stringify({ access_token: "saved-access", refresh_token: "saved-refresh", user }),
  });
  const api = await loadApi(t, { storage });

  const restored = api.restoreSession();

  assert.equal(restored.access_token, "saved-access");
  assert.equal(api.getAccessToken(), "saved-access");
  assert.deepEqual(window.events.at(-1).detail, user);
});

test("persists rotated access and refresh tokens after an expired access token", async t => {
  const user = { id: "teacher-1", roles: ["teacher"] };
  const storage = createStorage({
    "school-journal-session": JSON.stringify({ access_token: "expired-access", refresh_token: "old-refresh", user }),
  });
  const calls = [];
  const fetchImpl = async (url, options = {}) => {
    calls.push({ url: String(url), authorization: options.headers?.Authorization || "" });
    if (String(url).endsWith("/auth/refresh")) {
      return Response.json({ access_token: "new-access", refresh_token: "new-refresh", expires_in: 900 });
    }
    if (options.headers?.Authorization === "Bearer expired-access") {
      return Response.json({ message: "expired" }, { status: 401 });
    }
    return Response.json({ ok: true });
  };
  const api = await loadApi(t, { storage, fetchImpl });
  api.restoreSession();

  const result = await api.api("/protected");

  assert.deepEqual(result, { ok: true });
  assert.equal(calls.length, 3);
  assert.equal(calls[1].url, "https://api.test/auth/refresh");
  assert.equal(calls[2].authorization, "Bearer new-access");
  const saved = JSON.parse(storage.getItem("school-journal-session"));
  assert.equal(saved.access_token, "new-access");
  assert.equal(saved.refresh_token, "new-refresh");
  assert.deepEqual(saved.user, user);
});


test("business requests wait for health and a failed write is not replayed", async t => {
  let releaseHealth;
  let healthCalls = 0;
  const calls = [];
  const api = await loadApi(t, {
    storage: createStorage(),
    healthImpl: () => {
      healthCalls++;
      if (healthCalls === 1) return new Promise(resolve => { releaseHealth = resolve; });
      return Promise.resolve(Response.json({ status: "ok" }));
    },
    fetchImpl: async (url, options) => {
      calls.push({ url: String(url), method: options.method || "GET" });
      if (options.method === "POST") return Response.json({ message: "Unavailable" }, { status: 503 });
      return Response.json([]);
    },
  });
  const first = api.api("/students");
  const second = api.api("/grades");
  assert.equal(healthCalls, 1);
  assert.equal(calls.length, 0);
  releaseHealth(Response.json({ status: "ok" }));
  await Promise.all([first, second]);
  await assert.rejects(api.api("/grades", { method: "POST", body: "{}" }), { status: 503 });
  assert.equal(calls.filter(call => call.method === "POST").length, 1);
  await api.api("/students");
  assert.equal(healthCalls, 2);
});
