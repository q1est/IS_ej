#!/usr/bin/env bash
set -euo pipefail

: "${DATABASE_URL:?DATABASE_URL must point to an isolated test database}"
export PGCLIENTENCODING="${PGCLIENTENCODING:-UTF8}"

for migration in supabase/migrations/*.sql; do
  psql "$DATABASE_URL" --no-psqlrc --set=ON_ERROR_STOP=1 --file="$migration" >/dev/null
done

read_counts() {
  psql "$DATABASE_URL" --no-psqlrc --tuples-only --no-align --field-separator='|' --set=ON_ERROR_STOP=1 --command="
    WITH demo AS (SELECT school_id FROM users WHERE email='admin@demo-school.local')
    SELECT
      (SELECT COUNT(*) FROM schools WHERE id=demo.school_id),
      (SELECT COUNT(*) FROM academic_years WHERE school_id=demo.school_id),
      (SELECT COUNT(*) FROM classes WHERE school_id=demo.school_id),
      (SELECT COUNT(*) FROM terms WHERE school_id=demo.school_id),
      (SELECT COUNT(*) FROM subjects WHERE school_id=demo.school_id),
      (SELECT COUNT(*) FROM users WHERE school_id=demo.school_id),
      (SELECT COUNT(*) FROM user_roles WHERE school_id=demo.school_id),
      (SELECT COUNT(*) FROM students WHERE school_id=demo.school_id),
      (SELECT COUNT(*) FROM enrollments WHERE school_id=demo.school_id),
      (SELECT COUNT(*) FROM grades WHERE school_id=demo.school_id),
      (SELECT COUNT(*) FROM behavior_criteria WHERE school_id=demo.school_id),
      (SELECT COUNT(*) FROM behavior_sheets WHERE school_id=demo.school_id),
      (SELECT COUNT(*) FROM behavior_scores WHERE school_id=demo.school_id),
      (SELECT COUNT(*) FROM teacher_assignments WHERE school_id=demo.school_id),
      (SELECT COUNT(*) FROM class_teacher_assignments WHERE school_id=demo.school_id),
      (SELECT COUNT(*) FROM behavior_permissions WHERE school_id=demo.school_id),
      (SELECT COUNT(*) FROM projects WHERE school_id=demo.school_id),
      (SELECT COUNT(*) FROM student_projects WHERE school_id=demo.school_id),
      (SELECT COUNT(*) FROM invites WHERE school_id=demo.school_id),
      (SELECT COUNT(*) FROM refresh_sessions rs JOIN users u ON u.id=rs.user_id WHERE u.school_id=demo.school_id),
      (SELECT COUNT(*) FROM audit_log WHERE entity_id=demo.school_id::text AND action='seed_demo'),
      (SELECT COUNT(*) FROM roles WHERE code IN ('student','teacher','class_teacher','admin'))
    FROM demo;"
}

expected_counts="1|2|4|8|6|14|15|10|20|320|10|20|200|16|4|4|2|15|1|1|1|4"
for _ in 1 2; do
  psql "$DATABASE_URL" --no-psqlrc --set=ON_ERROR_STOP=1 --file=supabase/seed.sql >/dev/null
done

counts=$(read_counts)
if [[ "$counts" != "$expected_counts" ]]; then
  printf 'Unexpected demo dataset counts: %s\nExpected: %s\n' "$counts" "$expected_counts" >&2
  exit 1
fi

password_checks=$(psql "$DATABASE_URL" --no-psqlrc --tuples-only --no-align --set=ON_ERROR_STOP=1 --command="
  SELECT bool_and(u.password_hash = crypt(credentials.password, u.password_hash))
  FROM (VALUES
    ('admin@demo-school.local','DemoAdmin2026!'),
    ('math.teacher@demo-school.local','Teacher2026!'),
    ('language.teacher@demo-school.local','Teacher2026!'),
    ('class.teacher@demo-school.local','Teacher2026!'),
    ('student01@demo-school.local','Student2026!'),
    ('student10@demo-school.local','Student2026!')
  ) AS credentials(email,password)
  JOIN users u ON u.email=credentials.email
    AND u.school_id=(SELECT school_id FROM users WHERE email='admin@demo-school.local');")
if [[ "$password_checks" != "t" ]]; then
  printf 'Demo account password verification failed.\n' >&2
  exit 1
fi

printf 'Migrations and repeatable demo seed passed: %s\n' "$counts"
