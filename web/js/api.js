const API = (window.APP_CONFIG?.API_BASE_URL || "").replace(/\/$/, "");

let accessToken = null;
let refreshToken = null;
let refreshPromise = null;

export function setSession(data) {
  accessToken = data?.access_token || null;
  refreshToken = data?.refresh_token || null;
  window.dispatchEvent(new CustomEvent("session-changed", { detail: data?.user || null }));
}

export function clearSession() {
  accessToken = null;
  refreshToken = null;
  window.dispatchEvent(new CustomEvent("session-changed", { detail: null }));
}

export function getAccessToken() {
  return accessToken;
}

export async function logoutSession() {
  if (!accessToken || !refreshToken) return;
  await rotateToken();
  if (!accessToken || !refreshToken) return;
  await api("/auth/logout", { method: "POST", body: JSON.stringify({ refresh_token: refreshToken }) });
}

async function readResponse(response) {
  if (response.status === 204) return null;
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(data.message || `HTTP ${response.status}`);
  return data;
}

async function rotateToken() {
  if (!refreshToken) return false;
  if (!refreshPromise) {
    refreshPromise = (async () => {
      const response = await fetch(`${API}/auth/refresh`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ refresh_token: refreshToken }),
      });
      const data = await readResponse(response);
      accessToken = data.access_token;
      refreshToken = data.refresh_token;
      return true;
    })().catch(() => {
      clearSession();
      return false;
    }).finally(() => { refreshPromise = null; });
  }
  return refreshPromise;
}

async function request(path, options = {}, retry = true) {
  const headers = {
    "Content-Type": "application/json",
    ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
    ...(options.headers || {}),
  };
  const response = await fetch(`${API}${path}`, { ...options, headers });
  if (response.status === 401 && retry && accessToken && refreshToken && path !== "/auth/refresh") {
    if (await rotateToken()) return request(path, options, false);
  }
  return readResponse(response);
}

export function api(path, options = {}) {
  if (!API) return Promise.reject(new Error("Укажите API_BASE_URL в web/config.js"));
  return request(path, options);
}

export async function apiBlob(path) {
  if (!API) throw new Error("Укажите API_BASE_URL в web/config.js");
  const response = await fetch(`${API}${path}`, {
    headers: accessToken ? { Authorization: `Bearer ${accessToken}` } : {},
  });
  if (response.status === 401 && accessToken && refreshToken && await rotateToken()) return apiBlob(path);
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data.message || `HTTP ${response.status}`);
  }
  return response.blob();
}

export function saveBlob(blob, filename) {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  URL.revokeObjectURL(url);
}

export function toCSV(filename, rows) {
  const safe = value => {
    const text = String(value ?? "");
    return /^[\s]*[=+\-@]/.test(text) ? `'${text}` : text;
  };
  const content = "\uFEFF" + rows.map(row => row.map(value => `"${safe(value).replaceAll('"', '""')}"`).join(";")).join("\r\n");
  saveBlob(new Blob([content], { type: "text/csv;charset=utf-8" }), filename);
}
