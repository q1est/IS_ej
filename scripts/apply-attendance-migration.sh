#!/usr/bin/env bash
set -euo pipefail

: "${DATABASE_URL:?DATABASE_URL must be set to the intended PostgreSQL database}"

migration=supabase/migrations/20261101140000_attendance_register.sql
if [[ ! -f "$migration" ]]; then
  printf 'Migration not found: %s\n' "$migration" >&2
  exit 1
fi

printf 'Target: %s\n' "$(psql "$DATABASE_URL" --no-psqlrc --tuples-only --no-align --command="select current_database() || ' @ ' || coalesce(inet_server_addr()::text, 'local')")"
printf 'This applies the attendance table migration to the DATABASE_URL target.\n'
read -r -p 'Type APPLY to continue: ' confirmation
if [[ "$confirmation" != 'APPLY' ]]; then
  printf 'Cancelled. No migration was applied.\n'
  exit 1
fi

psql "$DATABASE_URL" --no-psqlrc --set=ON_ERROR_STOP=1 --file="$migration"
printf 'Attendance migration applied.\n'
