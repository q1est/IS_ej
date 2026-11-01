# Развертывание и эксплуатация

## Состав окружения

```text
GitHub Pages / static hosting: каталог web/
Netrun Docker Web App: Go API из Dockerfile
Supabase/PostgreSQL: отдельная managed база и schema migrations
GitHub Actions: проверки и публикация по настроенным workflows
```

Текущий deployment workflow может запускать backend deploy вслед за успешным CI при push в `main`. Всегда проверяйте repository rules/Actions и текущие workflow; документация не разрешает обход CI и не меняет его настройки.

## Runtime secrets/configuration

В Netrun project secrets/configuration требуются:

- `DATABASE_URL` — приватный PostgreSQL DSN; использовать рекомендованный платформой pooler/SSL.
- `JWT_SECRET` — случайный секрет как минимум 32 байта; хранить только в secret manager.
- `ALLOWED_ORIGINS` — точный опубликованный frontend origin (без пути, без wildcard).
- `PORT` — задаёт платформа; image defaults documented in `deploy/netrun/README.md`.

Для GitHub Actions в существующей конфигурации используются Netrun API key/project ID и публичный backend URL Pages config. Точные названия секретов сверяйте с текущими workflow. Не копируйте закрытый `DATABASE_URL`/JWT в GitHub variable, README, issue, frontend bundle, build log или артефакт.

## Netrun backend

Подробные шаги первого создания проекта находятся в [`../deploy/netrun/README.md`](../deploy/netrun/README.md). Вкратце:

1. Проверить доступность Dockerfile/build context и runtime port.
2. Создать Netrun web project из репозитория/branch, указанного в текущей настройке.
3. Внести runtime secrets через кабинет.
4. Настроить GitHub Actions secrets/variables по существующему workflow.
5. Проверить успешный CI до deployment и убедиться, что публикуется именно протестированный commit.
6. Открыть API `/healthz`, затем провести smoke API/UI на QA account.

Push в `main` может автоматически публиковать backend. Не делайте push или ручной redeploy без понимания данного эффекта и разрешения владельца. Ручное редактирование production данных требует отдельного разрешения.

## GitHub Pages frontend

Frontend — статические файлы. API base URL находится в `web/config.js`; опубликованный workflow может подменять его из GitHub variable/secret. После деплоя:

1. Убедиться, что Pages build успешен.
2. Проверить загруженный `web/config.js`, что в bundle указан только публичный HTTPS API endpoint.
3. Проверить точный CORS origin на backend.
4. Открыть сайт в гостевой сессии и проверить отсутствие Console errors.
5. В QA environment проверить вход/роли, не используя production записи без согласованного тестового аккаунта.

## Порядок выпуска

1. Review diff; не включать локальные `.env`, токены, реальные выгрузки или тестовые персональные данные.
2. Запустить Go tests/vet, JS tests, миграционные/интеграционные проверки на disposable базе.
3. Проверить функциональные кейсы `docs/functional-testing.md`.
4. Сверить commit/branch и CI result. Учесть, что `main` может автоматически деплоить.
5. Применить миграции отдельным согласованным этапом по процедуре `database.md` — не считать deploy приложения применением SQL.
6. Проверить API health, CORS, login/refresh, основные чтения и безопасный QA smoke.
7. Проверить deployment logs, метрики ошибок и rollback plan.

## Health и диагностика

- `GET /healthz` делает ping к PostgreSQL и сообщает доступность приложения/БД.
- 401: проверьте session expiration, Authorization header и активность пользователя.
- 403: сверяйте principal roles, школьный scope, class/subject assignment и permissions.
- `criteria_incomplete`: проверьте количество criteria в школе; не создавайте их вручную без проверенного Excel/schema шаблона.
- Attendance SQL/table errors: проверьте, применена ли миграция `attendance_records` в целевой БД.
- Не отображаются таблицы: проверьте Network запросы admin resources/users/invites/audit и backend логи; успешный health check не гарантирует, что все таблицы созданы.
- CORS: origin должен точно совпадать, включая схему и домен, но не включать path.

Логи перед отправкой необходимо очистить от access/refresh tokens, email, имён, табельных номеров, DSN и ученических данных.

## Rollback

Rollback приложения и schema rollback — разные операции. Версию образа можно откатить по процедуре платформы, если предыдущая версия совместима с текущей схемой. Не делайте `db reset` для rollback. Для миграций подготовьте отдельный migration-specific rollback/forward-fix план и восстановление backup; destructive restore требует явного согласования.
