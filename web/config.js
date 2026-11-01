// Локальный адрес нужен только для `python -m http.server 5500 -d web`.
// GitHub Pages workflow перезаписывает этот файл production URL из NETRUN_API_URL.
window.APP_CONFIG = { API_BASE_URL: window.location.hostname === "127.0.0.1" || window.location.hostname === "localhost" ? "http://127.0.0.1:18081" : "" };
