// Poll only the public readiness endpoint. Business requests are never replayed here.
export function createServerReadiness({ baseURL, onState = () => {}, fetchImpl = (...args) => fetch(...args), now = () => Date.now(), budgetMs = 60_000, attemptMs = 10_000, cacheMs = 30_000, pause = ms => new Promise(resolve => setTimeout(resolve, ms)) }) {
  let readyAt = -Infinity;
  let pending = null;
  const touch = () => { readyAt = now(); };
  const invalidate = () => { readyAt = -Infinity; };

  async function probe(timeoutMs) {
    const controller = new AbortController();
    let timer;
    try {
      return await Promise.race([
        (async () => {
          const response = await fetchImpl(`${baseURL}/healthz`, { method: "GET", headers: { Accept: "application/json" }, cache: "no-store", credentials: "omit", signal: controller.signal });
          if ([400, 401, 403, 404].includes(response.status)) {
            const error = new Error("Не удалось проверить сервер. Проверьте адрес API и настройки доступа.");
            error.code = "server_configuration";
            throw error;
          }
          if (!response.ok) throw new Error("Сервер пока не готов");
          // A hosting provider may return an HTML wake-up page with HTTP 200.
          const data = await response.json();
          if (data?.status !== "ok") throw new Error("Сервер пока не готов");
        })(),
        new Promise((_, reject) => {
          timer = setTimeout(() => {
            controller.abort();
            reject(new Error("Проверка сервера заняла слишком много времени"));
          }, timeoutMs);
        }),
      ]);
    } finally {
      clearTimeout(timer);
    }
  }

  async function poll() {
    const startedAt = now();
    const deadlineAt = startedAt + budgetMs;
    let attempt = 0;
    try {
      if (!baseURL) {
        const error = new Error("Адрес сервера не настроен. Укажите API_BASE_URL в web/config.js.");
        error.code = "server_configuration";
        throw error;
      }
      while (now() < deadlineAt) {
        attempt++;
        onState({ status: "checking", attempt, startedAt, deadlineAt });
        try {
          await probe(Math.min(attemptMs, deadlineAt - now()));
          touch();
          onState({ status: "ready" });
          return;
        } catch (error) {
          if (error.code === "server_configuration") throw error;
          const remaining = deadlineAt - now();
          if (remaining <= 0) break;
          await pause(Math.min([1000, 2000, 3000, 5000][Math.min(attempt - 1, 3)], remaining));
        }
      }
      const error = new Error("Сервер не ответил за минуту. Проверьте интернет или попробуйте подключиться ещё раз. Если проблема сохраняется, администратору нужно проверить сервер в Netrun.");
      error.code = "server_unavailable";
      throw error;
    } catch (error) {
      invalidate();
      onState({ status: "unavailable", message: error.message });
      throw error;
    }
  }

  return {
    touch,
    invalidate,
    wait(force = false) {
      if (pending) return pending;
      if (!force && now() - readyAt < cacheMs) return Promise.resolve();
      pending = poll().finally(() => { pending = null; });
      return pending;
    },
  };
}
