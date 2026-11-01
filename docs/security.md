# Безопасность и персональные данные

## Учетные записи и секреты

- Никогда не коммитьте `.env`, database DSN, JWT secret, API key, access/refresh token или пароль.
- Пользовательский password и приглашение нельзя отправлять через публичные issue/чаты.
- Применяйте уникальный случайный `JWT_SECRET`, минимум 32 байта; храните в Netrun/secret manager.
- Используйте MFA для админов, где поддерживается. Следуйте политике ротации ключей и немедленно отзывайте утёкшие credentials.
- Не используйте демонстрационные пароли из README в production.
- Проверяйте статус пользователя, минимально необходимые роли и назначения; при уходе пользователя отключите аккаунт и отзовите refresh-сессии.

## Персональные данные

Имена, email, табельные номера, даты рождения, посещаемость, оценки и поведение учащихся — чувствительные школьные данные. Используйте synthetic data в тестах. Если реальный доступ необходим:

- получите согласование владельца системы и школы;
- используйте минимум необходимых полей и времени хранения;
- ограничьте доступ по роли/школе;
- не делайте скриншоты/экспорт в публичные тикеты;
- следуйте применимому законодательству и школьной политике хранения.

## API и браузер

- Backend — источник истины для authorization, не frontend guards.
- Не отключайте tenant scope, назначение учителей и student-only restrictions ради прохождения UI теста.
- Auth endpoints имеют rate limiting; для масштабирования одной памяти процесса недостаточно — нужна согласованная защита proxy/edge.
- CORS разрешайте только точным trusted origins. CORS не является механизмом аутентификации.
- Не устанавливайте `Access-Control-Allow-Origin: *` для чувствительного API.
- Проверяйте server-side validation, school ownership, enrollment, даты, ограничения длины и повторные записи.
- В spreadsheet export защищайте cells от formula injection; не открывайте неизвестные файлы на доверенном рабочем компьютере.

## База и эксплуатация

- Не запускайте `supabase db reset`, demo seed, validation script или адресные/массовые SQL записи на production.
- Миграции review/test сначала на disposable/QA; backup и restore drill до реальных данных.
- Production write, migration, import, reset, user/role change требуют согласования и плана rollback.
- Демо seed имеет публичные пароли и фиктивные записи; только локально/QA.
- Не логируйте request body для login/invite, authorization headers, полные DSN или ученические данные.
- Защищайте backup шифрованием, IAM и сроком хранения.

## Reporting

Следуйте [SECURITY.md](../SECURITY.md) и используйте приватное vulnerability reporting: <https://github.com/q1est/IS_ej/security/advisories/new>. Публично не раскрывайте exploit details, уязвимые URL с токеном, реальные идентификаторы пользователей и сведения о школе. Для инцидента немедленно ограничьте воздействие/отзовите секреты, затем сохраните только redacted evidence.

## Security review checklist

- [ ] Тесты cross-school access отрицательные для каждого tenant-scoped ресурса.
- [ ] Студент не читает чужие данные и не выполняет write.
- [ ] Учитель ограничен назначениями; admin права не смешиваются с student ветками.
- [ ] Disabled account теряет доступ и refresh сессии.
- [ ] Секреты отсутствуют в source, frontend, artifacts и логах.
- [ ] CSV/XLSX безопасны для spreadsheet formula interpretation.
- [ ] Production CORS и TLS соответствуют доменам/платформе.
- [ ] Публичные demo credentials отсутствуют среди активных production accounts.
