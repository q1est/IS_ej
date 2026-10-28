CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE schools (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name text NOT NULL CHECK(length(trim(name))>0),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE users (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 school_id uuid NOT NULL REFERENCES schools(id) ON DELETE RESTRICT,
 email citext NOT NULL, password_hash text, full_name text NOT NULL,
 status text NOT NULL DEFAULT 'invited' CHECK(status IN ('invited','active','disabled')),
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(school_id,email)
);
CREATE TABLE roles (
 id smallserial PRIMARY KEY, code text NOT NULL UNIQUE CHECK(code IN ('student','teacher','class_teacher','admin')), name text NOT NULL
);
INSERT INTO roles(code,name) VALUES ('student','Ученик'),('teacher','Учитель'),('class_teacher','Классный руководитель'),('admin','Администратор') ON CONFLICT DO NOTHING;
CREATE TABLE academic_years (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), school_id uuid NOT NULL REFERENCES schools(id) ON DELETE RESTRICT,
 name text NOT NULL, starts_on date NOT NULL, ends_on date NOT NULL, is_current boolean NOT NULL DEFAULT false,
 CHECK(starts_on<=ends_on), UNIQUE(school_id,name)
);
CREATE UNIQUE INDEX academic_year_one_current_per_school ON academic_years(school_id) WHERE is_current;
CREATE TABLE classes (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), school_id uuid NOT NULL REFERENCES schools(id) ON DELETE RESTRICT,
 name text NOT NULL, academic_year_id uuid NOT NULL REFERENCES academic_years(id) ON DELETE RESTRICT, UNIQUE(academic_year_id,name)
);
CREATE TABLE subjects (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), school_id uuid NOT NULL REFERENCES schools(id) ON DELETE RESTRICT,
 name text NOT NULL, UNIQUE(school_id,name)
);
CREATE TABLE students (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), school_id uuid NOT NULL REFERENCES schools(id) ON DELETE RESTRICT,
 external_id text NOT NULL, full_name text NOT NULL, birth_date date, UNIQUE(school_id,external_id)
);
CREATE TABLE enrollments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), student_id uuid NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
 class_id uuid NOT NULL REFERENCES classes(id) ON DELETE RESTRICT,
 academic_year_id uuid NOT NULL REFERENCES academic_years(id) ON DELETE RESTRICT,
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','transferred','completed')),
 from_date date, to_date date, CHECK(to_date IS NULL OR from_date IS NULL OR to_date>=from_date),
 UNIQUE(class_id,student_id,academic_year_id)
);
CREATE UNIQUE INDEX one_active_enrollment_per_year ON enrollments(student_id,academic_year_id) WHERE status='active';
CREATE TABLE user_roles (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 role_id smallint NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
 school_id uuid NOT NULL REFERENCES schools(id) ON DELETE RESTRICT,
 academic_year_id uuid REFERENCES academic_years(id) ON DELETE RESTRICT,
 scope_id uuid
);
CREATE UNIQUE INDEX user_roles_unique_scope
 ON user_roles(user_id,role_id,school_id,academic_year_id,scope_id) NULLS NOT DISTINCT;
CREATE TABLE teacher_assignments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), teacher_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 class_id uuid NOT NULL REFERENCES classes(id) ON DELETE RESTRICT,
 subject_id uuid NOT NULL REFERENCES subjects(id) ON DELETE RESTRICT,
 academic_year_id uuid NOT NULL REFERENCES academic_years(id) ON DELETE RESTRICT,
 UNIQUE(teacher_id,class_id,subject_id,academic_year_id)
);
CREATE TABLE class_teacher_assignments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), teacher_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 class_id uuid NOT NULL REFERENCES classes(id) ON DELETE RESTRICT,
 academic_year_id uuid NOT NULL REFERENCES academic_years(id) ON DELETE RESTRICT, UNIQUE(class_id,academic_year_id)
);
CREATE TABLE terms (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), academic_year_id uuid NOT NULL REFERENCES academic_years(id) ON DELETE RESTRICT,
 name text NOT NULL, starts_on date NOT NULL, ends_on date NOT NULL, CHECK(starts_on<=ends_on), UNIQUE(academic_year_id,name)
);
CREATE TABLE grades (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), student_id uuid NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
 subject_id uuid NOT NULL REFERENCES subjects(id) ON DELETE RESTRICT, class_id uuid NOT NULL REFERENCES classes(id) ON DELETE RESTRICT,
 term_id uuid NOT NULL REFERENCES terms(id) ON DELETE RESTRICT, value smallint NOT NULL CHECK(value BETWEEN 1 AND 5),
 grade_type text NOT NULL DEFAULT 'regular', work_date date NOT NULL, comment text,
 created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT, created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz
);
CREATE INDEX grades_lookup ON grades(class_id,subject_id,term_id,work_date) WHERE deleted_at IS NULL;
CREATE TABLE behavior_criteria (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), school_id uuid NOT NULL REFERENCES schools(id) ON DELETE RESTRICT,
 code text NOT NULL, title text NOT NULL, description text, max_score smallint NOT NULL DEFAULT 3 CHECK(max_score BETWEEN 0 AND 3),
 sort_order smallint NOT NULL, UNIQUE(school_id,code), UNIQUE(school_id,sort_order)
);
CREATE TABLE behavior_sheets (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), class_id uuid NOT NULL REFERENCES classes(id) ON DELETE RESTRICT,
 student_id uuid NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
 academic_year_id uuid NOT NULL REFERENCES academic_years(id) ON DELETE RESTRICT,
 term_id uuid NOT NULL REFERENCES terms(id) ON DELETE RESTRICT,
 total_score smallint NOT NULL DEFAULT 0 CHECK(total_score BETWEEN 0 AND 30),
 verdict text NOT NULL DEFAULT 'недопустимое' CHECK(verdict IN ('образцовое','допустимое','недопустимое')),
 updated_by uuid REFERENCES users(id) ON DELETE RESTRICT, updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
 UNIQUE(student_id,academic_year_id,term_id)
);
CREATE TABLE behavior_scores (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), behavior_sheet_id uuid NOT NULL REFERENCES behavior_sheets(id) ON DELETE RESTRICT,
 criterion_id uuid NOT NULL REFERENCES behavior_criteria(id) ON DELETE RESTRICT, score smallint NOT NULL CHECK(score BETWEEN 0 AND 3),
 UNIQUE(behavior_sheet_id,criterion_id)
);
CREATE TABLE behavior_permissions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), teacher_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 class_id uuid NOT NULL REFERENCES classes(id) ON DELETE RESTRICT, academic_year_id uuid NOT NULL REFERENCES academic_years(id) ON DELETE RESTRICT,
 can_score boolean NOT NULL DEFAULT false, UNIQUE(teacher_id,class_id,academic_year_id)
);
CREATE TABLE projects (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), school_id uuid NOT NULL REFERENCES schools(id) ON DELETE RESTRICT, name text NOT NULL, UNIQUE(school_id,name)
);
CREATE TABLE student_projects (
 student_id uuid NOT NULL REFERENCES students(id) ON DELETE RESTRICT, project_id uuid NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
 academic_year_id uuid NOT NULL REFERENCES academic_years(id) ON DELETE RESTRICT, PRIMARY KEY(student_id,project_id,academic_year_id)
);
CREATE TABLE invites (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), school_id uuid NOT NULL REFERENCES schools(id) ON DELETE RESTRICT,
 email citext NOT NULL, role_id smallint NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
 target_type text NOT NULL, target_id uuid, academic_year_id uuid REFERENCES academic_years(id) ON DELETE RESTRICT,
 token_hash text NOT NULL UNIQUE, expires_at timestamptz NOT NULL, used_at timestamptz,
 created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE refresh_sessions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 token_hash text NOT NULL UNIQUE, expires_at timestamptz NOT NULL, revoked_at timestamptz,
 ip inet, user_agent text, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE audit_log (
 id bigserial PRIMARY KEY, actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL, action text NOT NULL,
 entity text NOT NULL, entity_id text NOT NULL, old_value jsonb, new_value jsonb,
 ip inet, user_agent text, created_at timestamptz NOT NULL DEFAULT now()
);
-- Seeds for the 10 behavior criteria are inserted per school by POST /admin/behavior-criteria or SQL below.
-- Supabase RLS: enable RLS and deny all direct client roles. Only backend connects with DB credentials.
