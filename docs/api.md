# API: маршруты и правила доступа

## Общие правила

- Base URL задаётся окружением; интерфейс получает только публичный HTTPS URL API.
- JSON API использует `Content-Type: application/json`, если не указано иначе.
- Защищённые маршруты ожидают `Authorization: Bearer <access_token>`.
- API повторно проверяет аутентификацию, активность учётной записи, школьную область, роль и назначение на сервере. Скрытая frontend-кнопка не заменяет backend authorization.
- Ошибка обычно имеет форму `{ "code": "...", "message": "...", "details": null }`.
- Даты передаются в ISO `YYYY-MM-DD`. Идентификаторы UUID берутся из ответов API, не угадываются.
- Не сохраняйте примеры с рабочими токенами или личными данными в документации/issues.

## Публичные/аутентификационные маршруты

| Метод | Путь | Доступ/назначение |
|---|---|---|
| `GET` | `/healthz` | Проверка процесса и DB ping; без токена |
| `POST` | `/auth/login` | Войти по email/паролю; rate limited |
| `POST` | `/auth/refresh` | Повернуть refresh session и получить новую пару |
| `POST` | `/auth/logout` | Отозвать refresh session |
| `POST` | `/invites/accept` | Активировать приглашение и учётную запись |
| `GET` | `/me` | Защищённый профиль текущего пользователя |

### Login

Запрос: `{ "email": "qa@example.test", "password": "...", "school_id": "optional UUID" }`. Не добавляйте реальные пароли в примеры/логи. При нескольких школах с одним email может требоваться `school_id`.

## Ученики

| Метод | Путь | Правила |
|---|---|---|
| `GET` | `/students?class_id=...&limit=...&offset=...` | Ученик — только связанная карточка; учитель — ученики разрешённых классов; администратор — школьная область |
| `GET` | `/students/{id}` | Собственная карточка ученика, допустимый учебный scope учителя либо школьный admin scope |

## Оценки

| Метод | Путь | Правила |
|---|---|---|
| `GET` | `/grades/contexts` | Возвращает допустимые class/subject/term contexts и `can_edit` |
| `GET` | `/grades/assignments` | Администратор или учитель с назначениями |
| `GET` | `/grades?class_id=...&term_id=...&subject_id=...` | Ученик может не передавать class, остальные выбирают разрешённый класс |
| `GET` | `/grades/class-summary?class_id=...&term_id=...` | Сводка для классного руководителя в пределах назначения; admin поддержан кодом |
| `POST` | `/grades` | Создание оценки; проверить зачисление, назначение и work key |
| `PATCH` | `/grades/{id}` | Изменение собственной по scope оценки; аудируется |
| `DELETE` | `/grades/{id}` | Мягкое удаление; аудируется |

Пример тела создания (UUID и данные должны быть из QA fixtures):

```json
{
  "student_id": "<student UUID>",
  "subject_id": "<subject UUID>",
  "class_id": "<class UUID>",
  "term_id": "<term UUID>",
  "value": 4,
  "grade_type": "homework",
  "work_date": "2026-10-30",
  "comment": "QA sample"
}
```

Оценка — целое 1–5. Не используйте приведённую дату как подсказку production года; она иллюстративна. Ответ `409` может означать конфликт уникальной работы. Сверьте текущий frontend payload и backend validation.

## Посещаемость

| Метод | Путь | Правила |
|---|---|---|
| `GET` | `/attendance/contexts` | Доступные класс/учебный год с `can_edit` |
| `GET` | `/attendance?class_id=...&date=YYYY-MM-DD` | Список зачисленных на дату и отметки; ученику ограничивается собственным профилем |
| `PUT` | `/attendance` | Пакетная запись разрешённого класса и даты |

Пример тела `PUT`:

```json
{
  "class_id": "<class UUID>",
  "date": "2026-10-30",
  "records": [
    { "student_id": "<student UUID>", "status": "absent", "note": "QA sample" }
  ]
}
```

Допустимые статусы: `present`, `absent`, `late`, `excused`. Пакет содержит 1–500 уникальных учеников; заметка до 500 символов. Сервер проверяет зачисление на дату и сохраняет атомарно.

## Поведение

| Метод | Путь | Правила |
|---|---|---|
| `GET` | `/behavior/criteria` | Критерии текущей школы |
| `GET` | `/behavior/contexts` | Классы/периоды, видимые ученику, назначенному учителю/class teacher или админу |
| `GET` | `/behavior/sheets?class_id=...&term_id=...` | Ученик видит собственное; учитель — разрешённый класс; admin — школа |
| `POST` | `/behavior/sheets/generate` | Только admin; создаёт отсутствующие ведомости активным ученикам |
| `POST` | `/behavior/sheets` | Ручное создание ведомости, admin-only по текущему handler |
| `GET` | `/behavior/sheets/{id}` | Проверка scope; возвращает лист и критерии |
| `PATCH` | `/behavior/sheets/{id}/scores` | Admin или teacher с явным behavior permission |
| `GET` | `/behavior/sheets/{id}/export.csv` | Проверка видимости и completeness критериев |

Создание/экспорт требуют корректно сконфигурированных 10 критериев. Excel header mapping динамический. Используйте сначала `/behavior/criteria`, затем экспортированный workbook; не полагайтесь на устаревший шаблон.

## Администрирование

Защита: `admin` текущей школы.

- `GET/POST /admin/{resource}` — справочники (точный перечень см. `internal/admin/handler.go` и README).
- `GET /admin/users`, `PATCH /admin/users/{id}` — каталог и управление ролями/статусом с защитой от self-lockout/потери последнего администратора.
- `GET /admin/invites`, `POST /admin/invites/{id}/revoke` — список/отзыв приглашений.
- `POST /admin/users/{id}/revoke-sessions` — завершение refresh-сессий пользователя.
- `GET /admin/audit` — аудит школы.
- `GET /admin/excel`, `POST /admin/excel` — экспорт/импорт workbook; multipart upload.
- `POST /invites` — создание приглашения администратором.

Не передавайте пользовательский Excel как JSON: multipart form field называется `file` (проверяйте актуальный frontend/client код). Импорт потенциально меняет много строк; прежде чем отправлять, сделайте backup и проверьте файл.

## CORS и HTTP ошибки

CORS разрешает только точные origins из `ALLOWED_ORIGINS`. В production задавайте HTTPS Pages origin без пути (например, `https://q1est.github.io`). Не включайте `*` при bearer-аутентификации.

Ожидаемые классы ошибок: 400 — input validation; 401 — отсутствующая/невалидная сессия; 403 — недостаточный доступ; 404 — объект не найден в разрешённом scope; 409 — конфликт состояния/уникальности/критериев; 5xx — сервер/БД. Конкретный code/message надо сверять с handler данного endpoint.
