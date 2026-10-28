# Электронный школьный журнал

Учебный MVP на Go, PostgreSQL/Supabase и ванильном JavaScript. Backend и frontend разделены: браузер вызывает только REST API и не подключается к Supabase напрямую.

## Архитектура

- `cmd/api` — запуск HTTP-сервера и graceful shutdown.
- `cmd/seedadmin` — безопасное создание первого администратора и 10 критериев поведения.
- `internal/auth`, `students`, `grades`, `behavior`, `admin` — независимые доменные модули.
- `internal/platform` — PostgreSQL, HTTP-ответы, конфигурация и rate limit.
- `internal/audit`, `internal/identity`, `internal/api` — аудит, идентификация и сборка приложения.
- `web/js` — отдельные модули API, интерфейса, журнала, поведения, администрирования и маршрутизации.
- `supabase/migrations` — версионируемая схема БД для Supabase.

## Быстрый запуск

1. Supabase-проект `school-journal` (`fmkudsrekkgfmiojhprt`) уже создан. Миграции находятся в `supabase/migrations` и применяются по порядку. Вторая миграция добавляет tenant-FK и стабильную привязку аккаунта ученика; она намеренно остановится на существующих неконсистентных или межшкольных связях.
2. Установите Go 1.23+. Настройте в терминале `DATABASE_URL`, `JWT_SECRET` (минимум 32 случайных байта), `ALLOWED_ORIGINS` (список origin через запятую) и при необходимости `PORT`. `.env.example` — образец переменных, Go сам не загружает `.env`.
3. Выполните `go mod download`, затем `go test ./...`.
4. Создайте первого администратора: `go run ./cmd/seedadmin --email admin@example.org --name "School Admin"`. Пароль вводится скрыто и должен содержать не менее 12 символов. Команда не сбрасывает пароль существующей учётной записи.
5. Запустите API командой `go run ./cmd/api`. Проверка готовности: `GET /healthz`.
6. В `web/config.js` задайте HTTPS URL API. Отдайте каталог `web` обычным статическим сервером; локально удобно использовать `python -m http.server 5500 -d web`. Добавьте origin этого сервера в `ALLOWED_ORIGINS`.

Docker-образ собирается многоступенчато, запускается от непривилегированного пользователя и проверяет `/healthz`. GitHub Pages публикует `web/`, а workflow `deploy-netrun.yml` синхронизирует backend с проектом Netrun.

## Supabase и публикация

- Supabase API URL: `https://fmkudsrekkgfmiojhprt.supabase.co`.
- Локальная конфигурация CLI уже указывает на проект `fmkudsrekkgfmiojhprt`.
- Для будущих миграций используйте `npx supabase@latest db push` после проверки SQL.
- GitHub Pages: `https://q1est.github.io/IS_ej/` после включения Pages с источником `GitHub Actions`.
- URL backend задаётся переменной репозитория `NETRUN_API_URL`; Pages подставляет её в `web/config.js` во время публикации.

### Netrun deployment

Пошаговая настройка находится в [`deploy/netrun/README.md`](deploy/netrun/README.md). Первый проект создаётся в кабинете Netrun из GitHub-репозитория. После этого каждый push в `main` запускает `.github/workflows/deploy-netrun.yml`, который загружает архив исходников и обновляет существующий проект.

В настройках самого проекта Netrun задаются `DATABASE_URL`, `JWT_SECRET` и `ALLOWED_ORIGINS=https://q1est.github.io`. В GitHub Actions нужны только `NETRUN_API_KEY` и числовой `NETRUN_PROJECT_ID`; архив не содержит `.env` и секретов.

## Основные сценарии

- Вход по email и паролю, одноразовые приглашения на 7 дней, refresh-сессии с ротацией и аудитом. Публичной регистрации нет.
- Ученик видит только собственные оценки и ведомость. Привязка к ученику хранится по ID, а не по ФИО.
- Учитель видит назначенные комбинации класса, предмета и периода; создаёт, редактирует и мягко удаляет оценки. Повторная оценка той же работы возвращает конфликт.
- Классный руководитель получает read-only сводку оценок и ведомость поведения своего класса.
- Учитель видит ведомость поведения только при явном разрешении `can_score` и может менять баллы только в рамках разрешения.
- Администратор управляет справочниками, зачислениями, назначениями, проектами и приглашениями; может завершить refresh-сессии пользователя и читать аудит.
- Ведомости поведения хранят неполное состояние отдельно. Полный результат пересчитывается на сервере и атомарно записывается с аудитом; пустой балл можно очистить.
- Экспорт CSV сохраняет русские заголовки, разделитель `;`, и защищает текст от формул электронных таблиц.

## API

- Auth: `POST /auth/login`, `POST /auth/refresh`, `POST /auth/logout`, `POST /invites/accept`, `GET /me`.
- Ученики: `GET /students`, `GET /students/{id}`.
- Оценки: `GET /grades/contexts`, `GET /grades`, `GET /grades/class-summary`, `POST /grades`, `PATCH /grades/{id}`, `DELETE /grades/{id}`.
- Поведение: `GET /behavior/contexts`, `GET /behavior/criteria`, `GET /behavior/sheets`, `POST /behavior/sheets/generate`, `GET /behavior/sheets/{id}`, `PATCH /behavior/sheets/{id}/scores`, `GET /behavior/sheets/{id}/export.csv`.
- Администрирование: `GET/POST /admin/{resource}`, `GET /admin/users`, `GET /admin/invites`, `POST /admin/invites/{id}/revoke`, `POST /admin/users/{id}/revoke-sessions`, `GET /admin/audit`, `POST /invites`.

Все защищённые маршруты повторно проверяют роль и школьную область на сервере. Ошибки используют формат `{ "code": "...", "message": "...", "details": null }`. API ограничивает тело JSON 1 MiB. Auth endpoints защищены in-memory rate limit; для нескольких экземпляров добавьте ограничение на reverse proxy.

## Границы MVP

Перед загрузкой персональных данных нужны независимый security review и тесты доступа с реальной тестовой PostgreSQL, MFA администратора, массовый CSV-импорт, доставка приглашений по email, изменение/отключение ролей через интерфейс, резервное копирование и юридическая проверка применимых требований к персональным данным. Интеграционные тесты прав и проверка миграций на реальном экземпляре Supabase здесь ещё не выполнены. Не используйте систему для реальных данных до этих проверок.

Репозиторий связан с GitHub: `https://github.com/q1est/IS_ej`. Deployment workflows не хранят секреты в коде и не публикуют `.env`.
