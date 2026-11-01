# Разработка и локальный запуск

## Требования

- Go 1.23 или новее.
- PostgreSQL, желательно отдельный локальный Supabase stack/QA database.
- Node.js с поддержкой встроенного `node --test` для frontend-тестов.
- `psql` для скрипта миграций/seed; Supabase CLI — если используются команды `supabase db ...`.

## Переменные окружения API

Настроить перед запуском `cmd/api`:

| Переменная | Назначение |
|---|---|
| `DATABASE_URL` | PostgreSQL DSN, только из секретного окружения |
| `JWT_SECRET` | случайный секрет минимум 32 байта; не коммитить |
| `ALLOWED_ORIGINS` | comma-separated список точных frontend origins |
| `PORT` | порт HTTP сервера, при необходимости |

Go процесс не читает `.env` автоматически. Устанавливайте переменные средствами shell, IDE или secret manager. Не помещайте DSN/JWT в код или `web/config.js`.

## Сборка и локальный запуск

```sh
go mod download
go test ./...
go vet ./...
go run ./cmd/api
```

В другом терминале из корня репозитория:

```sh
python -m http.server 5500 -d web
```

Добавьте точный origin локального сервера в `ALLOWED_ORIGINS`. Frontend `web/config.js` по hostname использует `http://127.0.0.1:18081` для localhost/127.0.0.1; если API слушает иной адрес/порт, задайте правильный локальный URL без публикации секрета.

Проверьте `GET /healthz`, затем откройте страницу. Health check подтверждает только доступность API и DB ping.

## Миграции и fixture

1. Убедитесь, что DSN указывает на изолированную disposable БД.
2. Просмотрите SQL миграции и порядок имен файлов.
3. Для demo fixture используйте [database.md](database.md). Скрипт `scripts/validate-demo-seed.sh` применяет миграции и вставляет demo seed в БД из `DATABASE_URL` — он не проверяет, что БД действительно безопасно изолирована.
4. Отдельно указывайте `TEST_DATABASE_URL` только для интеграционного теста.

Никогда не указывайте production DSN для reset, fixture scripts или integration tests.

## Тесты

```sh
go test ./...
go test -race ./...
go vet ./...
node --test web/js/api.test.mjs
```

Изолированная база нужна для API integration test:

```sh
TEST_DATABASE_URL='postgresql://.../qa_database?sslmode=require' \
  go test ./internal/api -run TestDemoTeacherInvitationAssignmentAndGradeWorkflow -count=1 -v
```

Если `TEST_DATABASE_URL` не установлена, интеграционный тест должен пропуститься (skip); это не означает, что workflow был полностью протестирован.

## Первый администратор

Для пустой/настроенной базы используйте CLI, который запросит пароль скрыто:

```sh
go run ./cmd/seedadmin --email admin@example.org --name "School Admin"
```

Сверьтесь с сообщением CLI и [database.md](database.md): bootstrap администратора/критериев выполняйте только с правильным DSN. Команда не сбрасывает пароль существующего пользователя.

## Изменение frontend

Frontend использует нативные ES modules без обязательной сборки. Проверяйте изменённые файлы через `node --check`, тесты через `node --test web/js/api.test.mjs`, а функциональные состояния — браузером с локальным backend и fixtures.

## Чеклист перед отправкой изменений

- [ ] `gofmt` для изменённых Go файлов.
- [ ] `go test ./...`.
- [ ] `go vet ./...`.
- [ ] `node --check` изменённых модулей и `node --test web/js/api.test.mjs`.
- [ ] Изменения миграций тестировались на disposable PostgreSQL.
- [ ] Smoke-функции/UI проверены по [сценарию функционального тестирования](functional-testing.md).
- [ ] Нет секретов/реальных данных, `.env`, пользовательских данных в логах и скриншотах.
- [ ] Не изменены workflow, если это отдельно запрещено задачей.
