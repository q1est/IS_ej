-- Tenant integrity, stable student identity, and an explicit incomplete state.
-- Apply after 001_init.sql. Foreign keys deliberately reject inconsistent
-- cross-school relations rather than relying only on request-handler checks.

ALTER TABLE users ADD CONSTRAINT users_id_school_unique UNIQUE (id, school_id);
ALTER TABLE academic_years ADD CONSTRAINT academic_years_id_school_unique UNIQUE (id, school_id);
ALTER TABLE classes ADD CONSTRAINT classes_id_school_unique UNIQUE (id, school_id);
ALTER TABLE classes ADD CONSTRAINT classes_id_year_unique UNIQUE (id, academic_year_id);
ALTER TABLE subjects ADD CONSTRAINT subjects_id_school_unique UNIQUE (id, school_id);
ALTER TABLE students ADD CONSTRAINT students_id_school_unique UNIQUE (id, school_id);
ALTER TABLE behavior_criteria ADD CONSTRAINT behavior_criteria_id_school_unique UNIQUE (id, school_id);
ALTER TABLE projects ADD CONSTRAINT projects_id_school_unique UNIQUE (id, school_id);

ALTER TABLE classes
  ADD CONSTRAINT classes_year_school_fk FOREIGN KEY (academic_year_id, school_id)
  REFERENCES academic_years(id, school_id) ON DELETE RESTRICT;

-- The invite target is a stable record ID. A student's display name is never
-- used to decide which student's grades or behavior sheet may be read.
ALTER TABLE students ADD COLUMN user_id uuid;
ALTER TABLE students ADD CONSTRAINT students_user_id_unique UNIQUE (user_id);
ALTER TABLE students ADD CONSTRAINT students_user_school_fk FOREIGN KEY (user_id, school_id)
  REFERENCES users(id, school_id) ON DELETE RESTRICT;

ALTER TABLE terms ADD COLUMN school_id uuid;
UPDATE terms t SET school_id = y.school_id FROM academic_years y WHERE y.id = t.academic_year_id;
ALTER TABLE terms ALTER COLUMN school_id SET NOT NULL;
ALTER TABLE terms ADD CONSTRAINT terms_id_school_unique UNIQUE (id, school_id);
ALTER TABLE terms ADD CONSTRAINT terms_id_year_unique UNIQUE (id, academic_year_id);
ALTER TABLE terms ADD CONSTRAINT terms_year_school_fk FOREIGN KEY (academic_year_id, school_id)
  REFERENCES academic_years(id, school_id) ON DELETE RESTRICT;

ALTER TABLE enrollments ADD COLUMN school_id uuid;
UPDATE enrollments e SET school_id = s.school_id FROM students s WHERE s.id = e.student_id;
ALTER TABLE enrollments ALTER COLUMN school_id SET NOT NULL;
ALTER TABLE enrollments ADD COLUMN created_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE enrollments
  ADD CONSTRAINT enrollments_student_school_fk FOREIGN KEY (student_id, school_id) REFERENCES students(id, school_id) ON DELETE RESTRICT,
  ADD CONSTRAINT enrollments_class_school_fk FOREIGN KEY (class_id, school_id) REFERENCES classes(id, school_id) ON DELETE RESTRICT,
  ADD CONSTRAINT enrollments_year_school_fk FOREIGN KEY (academic_year_id, school_id) REFERENCES academic_years(id, school_id) ON DELETE RESTRICT,
  ADD CONSTRAINT enrollments_class_year_fk FOREIGN KEY (class_id, academic_year_id) REFERENCES classes(id, academic_year_id) ON DELETE RESTRICT;

ALTER TABLE teacher_assignments ADD COLUMN school_id uuid;
UPDATE teacher_assignments ta SET school_id = u.school_id FROM users u WHERE u.id = ta.teacher_id;
ALTER TABLE teacher_assignments ALTER COLUMN school_id SET NOT NULL;
ALTER TABLE teacher_assignments
  ADD CONSTRAINT teacher_assignments_teacher_school_fk FOREIGN KEY (teacher_id, school_id) REFERENCES users(id, school_id) ON DELETE RESTRICT,
  ADD CONSTRAINT teacher_assignments_class_school_fk FOREIGN KEY (class_id, school_id) REFERENCES classes(id, school_id) ON DELETE RESTRICT,
  ADD CONSTRAINT teacher_assignments_subject_school_fk FOREIGN KEY (subject_id, school_id) REFERENCES subjects(id, school_id) ON DELETE RESTRICT,
  ADD CONSTRAINT teacher_assignments_year_school_fk FOREIGN KEY (academic_year_id, school_id) REFERENCES academic_years(id, school_id) ON DELETE RESTRICT,
  ADD CONSTRAINT teacher_assignments_class_year_fk FOREIGN KEY (class_id, academic_year_id) REFERENCES classes(id, academic_year_id) ON DELETE RESTRICT;

ALTER TABLE class_teacher_assignments ADD COLUMN school_id uuid;
UPDATE class_teacher_assignments ca SET school_id = u.school_id FROM users u WHERE u.id = ca.teacher_id;
ALTER TABLE class_teacher_assignments ALTER COLUMN school_id SET NOT NULL;
ALTER TABLE class_teacher_assignments
  ADD CONSTRAINT class_teacher_teacher_school_fk FOREIGN KEY (teacher_id, school_id) REFERENCES users(id, school_id) ON DELETE RESTRICT,
  ADD CONSTRAINT class_teacher_class_school_fk FOREIGN KEY (class_id, school_id) REFERENCES classes(id, school_id) ON DELETE RESTRICT,
  ADD CONSTRAINT class_teacher_year_school_fk FOREIGN KEY (academic_year_id, school_id) REFERENCES academic_years(id, school_id) ON DELETE RESTRICT,
  ADD CONSTRAINT class_teacher_class_year_fk FOREIGN KEY (class_id, academic_year_id) REFERENCES classes(id, academic_year_id) ON DELETE RESTRICT;

ALTER TABLE behavior_permissions ADD COLUMN school_id uuid;
UPDATE behavior_permissions bp SET school_id = u.school_id FROM users u WHERE u.id = bp.teacher_id;
ALTER TABLE behavior_permissions ALTER COLUMN school_id SET NOT NULL;
ALTER TABLE behavior_permissions
  ADD CONSTRAINT behavior_permissions_teacher_school_fk FOREIGN KEY (teacher_id, school_id) REFERENCES users(id, school_id) ON DELETE RESTRICT,
  ADD CONSTRAINT behavior_permissions_class_school_fk FOREIGN KEY (class_id, school_id) REFERENCES classes(id, school_id) ON DELETE RESTRICT,
  ADD CONSTRAINT behavior_permissions_year_school_fk FOREIGN KEY (academic_year_id, school_id) REFERENCES academic_years(id, school_id) ON DELETE RESTRICT,
  ADD CONSTRAINT behavior_permissions_class_year_fk FOREIGN KEY (class_id, academic_year_id) REFERENCES classes(id, academic_year_id) ON DELETE RESTRICT;

ALTER TABLE grades ADD COLUMN school_id uuid;
ALTER TABLE grades ADD COLUMN academic_year_id uuid;
UPDATE grades g SET school_id = c.school_id, academic_year_id = t.academic_year_id
  FROM classes c, terms t WHERE c.id = g.class_id AND t.id = g.term_id;
ALTER TABLE grades ALTER COLUMN school_id SET NOT NULL;
ALTER TABLE grades ALTER COLUMN academic_year_id SET NOT NULL;
ALTER TABLE grades
  ADD CONSTRAINT grades_student_school_fk FOREIGN KEY (student_id, school_id) REFERENCES students(id, school_id) ON DELETE RESTRICT,
  ADD CONSTRAINT grades_subject_school_fk FOREIGN KEY (subject_id, school_id) REFERENCES subjects(id, school_id) ON DELETE RESTRICT,
  ADD CONSTRAINT grades_class_school_fk FOREIGN KEY (class_id, school_id) REFERENCES classes(id, school_id) ON DELETE RESTRICT,
  ADD CONSTRAINT grades_class_year_fk FOREIGN KEY (class_id, academic_year_id) REFERENCES classes(id, academic_year_id) ON DELETE RESTRICT,
  ADD CONSTRAINT grades_term_year_fk FOREIGN KEY (term_id, academic_year_id) REFERENCES terms(id, academic_year_id) ON DELETE RESTRICT,
  ADD CONSTRAINT grades_creator_school_fk FOREIGN KEY (created_by, school_id) REFERENCES users(id, school_id) ON DELETE RESTRICT;
CREATE UNIQUE INDEX grades_idempotency_active ON grades(student_id, subject_id, term_id, grade_type, work_date) WHERE deleted_at IS NULL;

ALTER TABLE behavior_sheets ADD COLUMN school_id uuid;
UPDATE behavior_sheets bs SET school_id = s.school_id FROM students s WHERE s.id = bs.student_id;
ALTER TABLE behavior_sheets ALTER COLUMN school_id SET NOT NULL;
ALTER TABLE behavior_sheets ADD COLUMN is_complete boolean NOT NULL DEFAULT false;
ALTER TABLE behavior_sheets ALTER COLUMN verdict DROP NOT NULL;
ALTER TABLE behavior_sheets ALTER COLUMN verdict DROP DEFAULT;
UPDATE behavior_sheets bs SET
  total_score = COALESCE((SELECT sum(sc.score) FROM behavior_scores sc WHERE sc.behavior_sheet_id=bs.id),0),
  is_complete = (SELECT count(*)=10 FROM behavior_scores sc WHERE sc.behavior_sheet_id=bs.id)
                AND (SELECT count(*)=10 FROM behavior_criteria bc WHERE bc.school_id=bs.school_id),
  verdict = CASE
    WHEN (SELECT count(*)=10 FROM behavior_scores sc WHERE sc.behavior_sheet_id=bs.id)
         AND (SELECT count(*)=10 FROM behavior_criteria bc WHERE bc.school_id=bs.school_id)
    THEN CASE
      WHEN COALESCE((SELECT sum(sc.score) FROM behavior_scores sc WHERE sc.behavior_sheet_id=bs.id),0)>=24 THEN 'образцовое'
      WHEN COALESCE((SELECT sum(sc.score) FROM behavior_scores sc WHERE sc.behavior_sheet_id=bs.id),0)>=10 THEN 'допустимое'
      ELSE 'недопустимое'
    END
    ELSE NULL
  END;
ALTER TABLE behavior_sheets ADD CONSTRAINT behavior_sheets_id_school_unique UNIQUE (id, school_id);
ALTER TABLE behavior_sheets
  ADD CONSTRAINT behavior_sheets_student_school_fk FOREIGN KEY (student_id, school_id) REFERENCES students(id, school_id) ON DELETE RESTRICT,
  ADD CONSTRAINT behavior_sheets_class_school_fk FOREIGN KEY (class_id, school_id) REFERENCES classes(id, school_id) ON DELETE RESTRICT,
  ADD CONSTRAINT behavior_sheets_year_school_fk FOREIGN KEY (academic_year_id, school_id) REFERENCES academic_years(id, school_id) ON DELETE RESTRICT,
  ADD CONSTRAINT behavior_sheets_class_year_fk FOREIGN KEY (class_id, academic_year_id) REFERENCES classes(id, academic_year_id) ON DELETE RESTRICT,
  ADD CONSTRAINT behavior_sheets_term_year_fk FOREIGN KEY (term_id, academic_year_id) REFERENCES terms(id, academic_year_id) ON DELETE RESTRICT,
  ADD CONSTRAINT behavior_sheets_updater_school_fk FOREIGN KEY (updated_by, school_id) REFERENCES users(id, school_id) ON DELETE RESTRICT;

ALTER TABLE behavior_scores ADD COLUMN school_id uuid;
UPDATE behavior_scores sc SET school_id = bs.school_id FROM behavior_sheets bs WHERE bs.id = sc.behavior_sheet_id;
ALTER TABLE behavior_scores ALTER COLUMN school_id SET NOT NULL;
ALTER TABLE behavior_scores
  ADD CONSTRAINT behavior_scores_sheet_school_fk FOREIGN KEY (behavior_sheet_id, school_id) REFERENCES behavior_sheets(id, school_id) ON DELETE RESTRICT,
  ADD CONSTRAINT behavior_scores_criterion_school_fk FOREIGN KEY (criterion_id, school_id) REFERENCES behavior_criteria(id, school_id) ON DELETE RESTRICT;

ALTER TABLE behavior_permissions ADD CONSTRAINT behavior_permissions_id_school_unique UNIQUE (id, school_id);
ALTER TABLE student_projects ADD COLUMN school_id uuid;
UPDATE student_projects sp SET school_id = s.school_id FROM students s WHERE s.id = sp.student_id;
ALTER TABLE student_projects ALTER COLUMN school_id SET NOT NULL;
ALTER TABLE student_projects
  ADD CONSTRAINT student_projects_student_school_fk FOREIGN KEY (student_id, school_id) REFERENCES students(id, school_id) ON DELETE RESTRICT,
  ADD CONSTRAINT student_projects_project_school_fk FOREIGN KEY (project_id, school_id) REFERENCES projects(id, school_id) ON DELETE RESTRICT,
  ADD CONSTRAINT student_projects_year_school_fk FOREIGN KEY (academic_year_id, school_id) REFERENCES academic_years(id, school_id) ON DELETE RESTRICT;

ALTER TABLE user_roles ADD CONSTRAINT user_roles_user_school_fk FOREIGN KEY (user_id, school_id) REFERENCES users(id, school_id) ON DELETE RESTRICT;
ALTER TABLE user_roles ADD CONSTRAINT user_roles_year_school_fk FOREIGN KEY (academic_year_id, school_id) REFERENCES academic_years(id, school_id) ON DELETE RESTRICT;
ALTER TABLE invites ADD CONSTRAINT invites_creator_school_fk FOREIGN KEY (created_by, school_id) REFERENCES users(id, school_id) ON DELETE RESTRICT;
ALTER TABLE invites ADD CONSTRAINT invites_year_school_fk FOREIGN KEY (academic_year_id, school_id) REFERENCES academic_years(id, school_id) ON DELETE RESTRICT;

CREATE FUNCTION enforce_behavior_score_maximum() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE criterion_max smallint;
BEGIN
  SELECT max_score INTO criterion_max FROM behavior_criteria WHERE id = NEW.criterion_id AND school_id = NEW.school_id;
  IF criterion_max IS NULL OR NEW.score < 0 OR NEW.score > criterion_max THEN
    RAISE EXCEPTION 'behavior score is outside criterion range';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER behavior_score_maximum_check BEFORE INSERT OR UPDATE ON behavior_scores
  FOR EACH ROW EXECUTE FUNCTION enforce_behavior_score_maximum();

-- RLS is enabled without client-facing policies: anon/authenticated roles have
-- no direct table access. The API uses a server-only database credential.
DO $$
DECLARE table_name text;
BEGIN
  FOREACH table_name IN ARRAY ARRAY[
    'schools','users','roles','user_roles','academic_years','classes','subjects','students','enrollments',
    'teacher_assignments','class_teacher_assignments','terms','grades','behavior_criteria','behavior_sheets',
    'behavior_scores','behavior_permissions','projects','student_projects','invites','refresh_sessions','audit_log'
  ] LOOP
    EXECUTE format('ALTER TABLE public.%I ENABLE ROW LEVEL SECURITY', table_name);
  END LOOP;
END;
$$;
