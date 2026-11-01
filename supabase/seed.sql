-- Complete, repeatable demo dataset. Requires all schema migrations first.
-- Passwords below are public demo credentials; use only in an isolated demo tenant.
BEGIN;

INSERT INTO roles(code,name) VALUES
  ('student','Ученик'),('teacher','Учитель'),
  ('class_teacher','Классный руководитель'),('admin','Администратор')
ON CONFLICT(code) DO UPDATE SET name=excluded.name;

DO $$
DECLARE
  v_school_id uuid;
  v_year_id uuid;
  v_previous_year_id uuid;
  v_term1_id uuid;
  v_term2_id uuid;
  v_term3_id uuid;
  v_term4_id uuid;
  v_previous_term4_id uuid;
  v_class_a_id uuid;
  v_class_b_id uuid;
  v_previous_class_a_id uuid;
  v_previous_class_b_id uuid;
  v_math_id uuid;
  v_russian_id uuid;
  v_english_id uuid;
  v_physics_id uuid;
  v_history_id uuid;
  v_informatics_id uuid;
  v_admin_id uuid;
  v_teacher1_id uuid;
  v_teacher2_id uuid;
  v_teacher3_id uuid;
  v_student_id uuid;
  v_student_user_id uuid;
  v_sheet_id uuid;
  v_existing_id uuid;
  v_student_names text[] := ARRAY[
    'Александра Морозова','Михаил Кузнецов','София Смирнова','Артём Попов','Виктория Васильева',
    'Даниил Петров','Мария Соколова','Кирилл Михайлов','Елизавета Новикова','Алексей Фёдоров'
  ];
  v_grade_types text[] := ARRAY['homework','classwork','quiz','test'];
  v_subject_ids uuid[];
  v_subject_id uuid;
  v_teacher_id uuid;
  v_work_date date;
  i int;
  v_grade_n int;
  v_class_id uuid;
  v_previous_class_id uuid;
  v_sheet_year_id uuid;
  v_sheet_term_id uuid;
BEGIN
  SELECT sc.id INTO v_school_id FROM schools sc WHERE sc.name='Демо-школа «Сириус»' LIMIT 1;
  IF v_school_id IS NULL THEN
    INSERT INTO schools(name) VALUES ('Демо-школа «Сириус»') RETURNING id INTO v_school_id;
  END IF;

  UPDATE academic_years SET is_current=false WHERE school_id=v_school_id;
  INSERT INTO academic_years(id,school_id,name,starts_on,ends_on,is_current)
    VALUES(gen_random_uuid(),v_school_id,'2025/2026','2025-09-01','2026-08-31',false)
    ON CONFLICT(school_id,name) DO UPDATE SET starts_on=excluded.starts_on,ends_on=excluded.ends_on,is_current=false
    RETURNING id INTO v_previous_year_id;
  INSERT INTO academic_years(id,school_id,name,starts_on,ends_on,is_current)
    VALUES(gen_random_uuid(),v_school_id,'2026/2027','2026-09-01','2027-08-31',true)
    ON CONFLICT(school_id,name) DO UPDATE SET starts_on=excluded.starts_on,ends_on=excluded.ends_on,is_current=true
    RETURNING id INTO v_year_id;

  INSERT INTO terms(id,academic_year_id,name,starts_on,ends_on,school_id) VALUES
    (gen_random_uuid(),v_previous_year_id,'1 четверть','2025-09-01','2025-10-31',v_school_id),
    (gen_random_uuid(),v_previous_year_id,'2 четверть','2025-11-01','2025-12-29',v_school_id),
    (gen_random_uuid(),v_previous_year_id,'3 четверть','2026-01-12','2026-03-24',v_school_id),
    (gen_random_uuid(),v_previous_year_id,'4 четверть','2026-04-01','2026-05-31',v_school_id),
    (gen_random_uuid(),v_year_id,'1 четверть','2026-09-01','2026-10-31',v_school_id),
    (gen_random_uuid(),v_year_id,'2 четверть','2026-11-01','2026-12-29',v_school_id),
    (gen_random_uuid(),v_year_id,'3 четверть','2027-01-12','2027-03-24',v_school_id),
    (gen_random_uuid(),v_year_id,'4 четверть','2027-04-01','2027-05-31',v_school_id)
    ON CONFLICT(academic_year_id,name) DO UPDATE SET starts_on=excluded.starts_on,ends_on=excluded.ends_on,school_id=excluded.school_id;
  SELECT tr.id INTO v_term1_id FROM terms tr WHERE tr.academic_year_id=v_year_id AND tr.name='1 четверть';
  SELECT tr.id INTO v_term2_id FROM terms tr WHERE tr.academic_year_id=v_year_id AND tr.name='2 четверть';
  SELECT tr.id INTO v_term3_id FROM terms tr WHERE tr.academic_year_id=v_year_id AND tr.name='3 четверть';
  SELECT tr.id INTO v_term4_id FROM terms tr WHERE tr.academic_year_id=v_year_id AND tr.name='4 четверть';
  SELECT tr.id INTO v_previous_term4_id FROM terms tr WHERE tr.academic_year_id=v_previous_year_id AND tr.name='4 четверть';

  INSERT INTO classes(id,school_id,name,academic_year_id) VALUES
    (gen_random_uuid(),v_school_id,'6А',v_previous_year_id),(gen_random_uuid(),v_school_id,'7Б',v_previous_year_id),
    (gen_random_uuid(),v_school_id,'7А',v_year_id),(gen_random_uuid(),v_school_id,'8Б',v_year_id)
    ON CONFLICT(academic_year_id,name) DO NOTHING;
  SELECT cl.id INTO v_class_a_id FROM classes cl WHERE cl.school_id=v_school_id AND cl.name='7А' AND cl.academic_year_id=v_year_id;
  SELECT cl.id INTO v_class_b_id FROM classes cl WHERE cl.school_id=v_school_id AND cl.name='8Б' AND cl.academic_year_id=v_year_id;
  SELECT cl.id INTO v_previous_class_a_id FROM classes cl WHERE cl.school_id=v_school_id AND cl.name='6А' AND cl.academic_year_id=v_previous_year_id;
  SELECT cl.id INTO v_previous_class_b_id FROM classes cl WHERE cl.school_id=v_school_id AND cl.name='7Б' AND cl.academic_year_id=v_previous_year_id;

  INSERT INTO subjects(school_id,name) VALUES
    (v_school_id,'Математика'),(v_school_id,'Русский язык'),(v_school_id,'Английский язык'),
    (v_school_id,'Физика'),(v_school_id,'История'),(v_school_id,'Информатика')
    ON CONFLICT(school_id,name) DO NOTHING;
  SELECT su.id INTO v_math_id FROM subjects su WHERE su.school_id=v_school_id AND su.name='Математика';
  SELECT su.id INTO v_russian_id FROM subjects su WHERE su.school_id=v_school_id AND su.name='Русский язык';
  SELECT su.id INTO v_english_id FROM subjects su WHERE su.school_id=v_school_id AND su.name='Английский язык';
  SELECT su.id INTO v_physics_id FROM subjects su WHERE su.school_id=v_school_id AND su.name='Физика';
  SELECT su.id INTO v_history_id FROM subjects su WHERE su.school_id=v_school_id AND su.name='История';
  SELECT su.id INTO v_informatics_id FROM subjects su WHERE su.school_id=v_school_id AND su.name='Информатика';

  INSERT INTO users(school_id,email,password_hash,full_name,status)
    VALUES(v_school_id,'admin@demo-school.local',crypt('DemoAdmin2026!',gen_salt('bf',12)),'Анна Петрова','active')
    ON CONFLICT(school_id,email) DO UPDATE SET status='active', full_name=excluded.full_name, password_hash=excluded.password_hash
    RETURNING id INTO v_admin_id;
  IF v_admin_id IS NULL THEN SELECT u.id INTO v_admin_id FROM users u WHERE u.school_id=v_school_id AND u.email='admin@demo-school.local'; END IF;
  INSERT INTO user_roles(id,user_id,role_id,school_id,academic_year_id,scope_id)
    SELECT gen_random_uuid(),v_admin_id,id,v_school_id,NULL,NULL FROM roles WHERE code='admin' ON CONFLICT DO NOTHING;

  INSERT INTO users(school_id,email,password_hash,full_name,status) VALUES
    (v_school_id,'math.teacher@demo-school.local',crypt('Teacher2026!',gen_salt('bf',12)),'Иван Сергеевич Волков','active'),
    (v_school_id,'language.teacher@demo-school.local',crypt('Teacher2026!',gen_salt('bf',12)),'Елена Андреевна Орлова','active'),
    (v_school_id,'class.teacher@demo-school.local',crypt('Teacher2026!',gen_salt('bf',12)),'Максим Олегович Соколов','active')
    ON CONFLICT(school_id,email) DO UPDATE SET status='active', full_name=excluded.full_name, password_hash=excluded.password_hash;
  SELECT u.id INTO v_teacher1_id FROM users u WHERE u.school_id=v_school_id AND u.email='math.teacher@demo-school.local';
  SELECT u.id INTO v_teacher2_id FROM users u WHERE u.school_id=v_school_id AND u.email='language.teacher@demo-school.local';
  SELECT u.id INTO v_teacher3_id FROM users u WHERE u.school_id=v_school_id AND u.email='class.teacher@demo-school.local';
  INSERT INTO user_roles(id,user_id,role_id,school_id,academic_year_id,scope_id)
    SELECT gen_random_uuid(),x,id,v_school_id,NULL,NULL FROM (VALUES(v_teacher1_id),(v_teacher2_id),(v_teacher3_id)) q(x)
    CROSS JOIN roles WHERE code='teacher' ON CONFLICT DO NOTHING;
  INSERT INTO user_roles(id,user_id,role_id,school_id,academic_year_id,scope_id)
    SELECT gen_random_uuid(),v_teacher3_id,id,v_school_id,NULL,NULL FROM roles WHERE code='class_teacher' ON CONFLICT DO NOTHING;

  INSERT INTO teacher_assignments(id,teacher_id,class_id,subject_id,academic_year_id,school_id) VALUES
    (gen_random_uuid(),v_teacher1_id,v_previous_class_a_id,v_math_id,v_previous_year_id,v_school_id),
    (gen_random_uuid(),v_teacher1_id,v_previous_class_b_id,v_math_id,v_previous_year_id,v_school_id),
    (gen_random_uuid(),v_teacher2_id,v_previous_class_a_id,v_russian_id,v_previous_year_id,v_school_id),
    (gen_random_uuid(),v_teacher2_id,v_previous_class_b_id,v_english_id,v_previous_year_id,v_school_id),
    (gen_random_uuid(),v_teacher1_id,v_previous_class_a_id,v_physics_id,v_previous_year_id,v_school_id),
    (gen_random_uuid(),v_teacher1_id,v_previous_class_b_id,v_physics_id,v_previous_year_id,v_school_id),
    (gen_random_uuid(),v_teacher3_id,v_previous_class_a_id,v_history_id,v_previous_year_id,v_school_id),
    (gen_random_uuid(),v_teacher3_id,v_previous_class_b_id,v_informatics_id,v_previous_year_id,v_school_id),
    (gen_random_uuid(),v_teacher1_id,v_class_a_id,v_math_id,v_year_id,v_school_id),(gen_random_uuid(),v_teacher1_id,v_class_b_id,v_math_id,v_year_id,v_school_id),(gen_random_uuid(),v_teacher2_id,v_class_a_id,v_russian_id,v_year_id,v_school_id),
    (gen_random_uuid(),v_teacher2_id,v_class_b_id,v_english_id,v_year_id,v_school_id),(gen_random_uuid(),v_teacher1_id,v_class_a_id,v_physics_id,v_year_id,v_school_id),(gen_random_uuid(),v_teacher1_id,v_class_b_id,v_physics_id,v_year_id,v_school_id),
    (gen_random_uuid(),v_teacher3_id,v_class_a_id,v_history_id,v_year_id,v_school_id),(gen_random_uuid(),v_teacher3_id,v_class_b_id,v_informatics_id,v_year_id,v_school_id)
    ON CONFLICT DO NOTHING;
  INSERT INTO class_teacher_assignments(id,teacher_id,class_id,academic_year_id,school_id) VALUES
    (gen_random_uuid(),v_teacher3_id,v_previous_class_a_id,v_previous_year_id,v_school_id),
    (gen_random_uuid(),v_teacher3_id,v_previous_class_b_id,v_previous_year_id,v_school_id),
    (gen_random_uuid(),v_teacher3_id,v_class_a_id,v_year_id,v_school_id),(gen_random_uuid(),v_teacher3_id,v_class_b_id,v_year_id,v_school_id)
    ON CONFLICT(class_id,academic_year_id) DO UPDATE SET teacher_id=excluded.teacher_id, school_id=excluded.school_id;

  FOR i IN 1..10 LOOP
    v_class_id := CASE WHEN i<=5 THEN v_class_a_id ELSE v_class_b_id END;
    v_existing_id := NULL;
    v_previous_class_id := CASE WHEN i<=5 THEN v_previous_class_a_id ELSE v_previous_class_b_id END;
    INSERT INTO students(id,school_id,external_id,full_name,birth_date,user_id)
      VALUES(gen_random_uuid(),v_school_id,'DEMO-'||lpad(i::text,3,'0'), v_student_names[i],
        (CASE WHEN i<=5 THEN '2013-01-01'::date ELSE '2012-01-01'::date END + (i*37)),NULL)
      ON CONFLICT(school_id,external_id) DO UPDATE SET full_name=excluded.full_name,birth_date=excluded.birth_date
      RETURNING id INTO v_existing_id;
    IF v_existing_id IS NULL THEN SELECT st.id INTO v_existing_id FROM students st WHERE st.school_id=v_school_id AND st.external_id='DEMO-'||lpad(i::text,3,'0'); END IF;
    v_student_id := v_existing_id;
    INSERT INTO enrollments(id,student_id,class_id,academic_year_id,status,from_date,to_date,school_id,created_at)
      VALUES(gen_random_uuid(),v_student_id,v_previous_class_id,v_previous_year_id,'completed','2025-09-01','2026-05-31',v_school_id,'2025-09-01 09:00:00+03')
      ON CONFLICT(class_id,student_id,academic_year_id) DO UPDATE SET status=excluded.status,from_date=excluded.from_date,to_date=excluded.to_date,school_id=excluded.school_id;
    INSERT INTO enrollments(id,student_id,class_id,academic_year_id,status,from_date,to_date,school_id,created_at)
      VALUES(gen_random_uuid(),v_student_id,v_class_id,v_year_id,'active','2026-09-01',NULL,v_school_id,'2026-09-01 09:00:00+03')
      ON CONFLICT(class_id,student_id,academic_year_id) DO UPDATE SET status=excluded.status,from_date=excluded.from_date,to_date=excluded.to_date,school_id=excluded.school_id;
    v_existing_id := NULL;
    INSERT INTO users(id,school_id,email,password_hash,full_name,status,created_at)
      VALUES(gen_random_uuid(),v_school_id,'student'||lpad(i::text,2,'0')||'@demo-school.local',crypt('Student2026!',gen_salt('bf',12)),v_student_names[i],'active','2026-08-25 09:00:00+03')
      ON CONFLICT(school_id,email) DO UPDATE SET status='active',full_name=excluded.full_name,password_hash=excluded.password_hash RETURNING id INTO v_existing_id;
    IF v_existing_id IS NULL THEN SELECT u.id INTO v_existing_id FROM users u WHERE u.school_id=v_school_id AND u.email='student'||lpad(i::text,2,'0')||'@demo-school.local'; END IF;
    v_student_user_id := v_existing_id;
    DELETE FROM user_roles ur USING roles r WHERE ur.role_id=r.id AND ur.user_id=v_student_user_id AND ur.school_id=v_school_id AND r.code='student';
    INSERT INTO user_roles(id,user_id,role_id,school_id,academic_year_id,scope_id)
      SELECT gen_random_uuid(),v_student_user_id,id,v_school_id,v_year_id,v_student_id FROM roles WHERE code='student' ON CONFLICT DO NOTHING;
    UPDATE students st SET user_id=v_student_user_id WHERE st.id=v_student_id AND (st.user_id IS NULL OR st.user_id=v_student_user_id);
    v_subject_ids := CASE WHEN v_class_id=v_class_a_id
      THEN ARRAY[v_math_id,v_russian_id,v_physics_id,v_history_id]
      ELSE ARRAY[v_math_id,v_english_id,v_physics_id,v_informatics_id] END;
    FOREACH v_subject_id IN ARRAY v_subject_ids LOOP
      v_teacher_id := CASE WHEN v_subject_id IN (v_math_id,v_physics_id) THEN v_teacher1_id
        WHEN v_subject_id IN (v_russian_id,v_english_id) THEN v_teacher2_id ELSE v_teacher3_id END;
      FOR v_grade_n IN 1..4 LOOP
        v_work_date := DATE '2026-10-03' + ((i-1)*2) + ((v_grade_n-1)*2);
        INSERT INTO grades(id,student_id,subject_id,class_id,term_id,school_id,academic_year_id,value,grade_type,work_date,comment,created_by,created_at,updated_at,deleted_at)
          VALUES(gen_random_uuid(),v_student_id,v_subject_id,v_class_id,v_term1_id,v_school_id,v_year_id,
            2+((i+array_position(v_subject_ids,v_subject_id)+v_grade_n)%4),v_grade_types[v_grade_n],v_work_date,
            CASE v_grade_n WHEN 1 THEN 'Домашнее задание по теме четверти' WHEN 2 THEN 'Работа на уроке'
              WHEN 3 THEN 'Самостоятельная работа' ELSE 'Контрольная работа по теме четверти' END,v_teacher_id,
            v_work_date::timestamptz + interval '15 hours',v_work_date::timestamptz + interval '15 hours',NULL)
          ON CONFLICT DO NOTHING;

        v_work_date := DATE '2026-04-03' + ((i-1)*2) + ((v_grade_n-1)*2);
        INSERT INTO grades(id,student_id,subject_id,class_id,term_id,school_id,academic_year_id,value,grade_type,work_date,comment,created_by,created_at,updated_at,deleted_at)
          VALUES(gen_random_uuid(),v_student_id,v_subject_id,v_previous_class_id,v_previous_term4_id,v_school_id,v_previous_year_id,
            2+((i+array_position(v_subject_ids,v_subject_id)+v_grade_n+1)%4),v_grade_types[v_grade_n],v_work_date,
            CASE v_grade_n WHEN 1 THEN 'Домашнее задание по теме четверти' WHEN 2 THEN 'Работа на уроке'
              WHEN 3 THEN 'Самостоятельная работа' ELSE 'Итоговая контрольная работа' END,v_teacher_id,
            v_work_date::timestamptz + interval '15 hours',v_work_date::timestamptz + interval '15 hours',NULL)
          ON CONFLICT DO NOTHING;
      END LOOP;
    END LOOP;
  END LOOP;

  INSERT INTO behavior_criteria(id,school_id,code,title,description,max_score,sort_order)
    SELECT gen_random_uuid(),v_school_id,code,title,description,3,sort_order FROM (VALUES
      ('schedule_rules','Расписание, правила и нормы поведения в учебной деятельности (в том числе внеурочной).','Соблюдает учебный распорядок и правила занятий.',1),
      ('appearance','Одежда обучающегося и требования к внешнему виду.','Соблюдает требования школьной формы и опрятного внешнего вида.',2),
      ('civic_actions','Нормы поведения при гражданско-патриотических акциях.','Участвует уважительно и ответственно в общешкольных мероприятиях.',3),
      ('safety','Правила пожарной безопасности, электробезопасности, поведения при угрозе ЧС.','Знает и соблюдает правила безопасности.',4),
      ('peer_communication','Конструктивное взаимодействие со сверстниками, общепринятые нормы коммуникации.','Уважительно общается и сотрудничает с одноклассниками.',5),
      ('conflict_prevention','Отсутствие умышленных конфликтов и агрессии (травли).','Решает разногласия без агрессии и травли.',6),
      ('teacher_communication','Уважительное взаимодействие с педагогами, нормы коммуникации.','Вежливо общается с педагогами и сотрудниками школы.',7),
      ('responsibility','Ответственное отношение к поручениям и рекомендациям педагогов.','Выполняет поручения и учитывает рекомендации.',8),
      ('lawful_behavior','Отсутствие фактов антиобщественных действий и противоправного поведения.','Бережно относится к имуществу и соблюдает правила школы.',9),
      ('prevention_records','Отсутствие индивидуальной профилактической работы / учёта.','Своевременно выполняет индивидуальные рекомендации.',10)
    ) v(code,title,description,sort_order) ON CONFLICT(school_id,code) DO UPDATE
      SET title=excluded.title,description=excluded.description,max_score=excluded.max_score,sort_order=excluded.sort_order;
  INSERT INTO behavior_permissions(id,teacher_id,class_id,academic_year_id,can_score,school_id) VALUES
    (gen_random_uuid(),v_teacher3_id,v_previous_class_a_id,v_previous_year_id,true,v_school_id),
    (gen_random_uuid(),v_teacher3_id,v_previous_class_b_id,v_previous_year_id,true,v_school_id),
    (gen_random_uuid(),v_teacher3_id,v_class_a_id,v_year_id,true,v_school_id),
    (gen_random_uuid(),v_teacher3_id,v_class_b_id,v_year_id,true,v_school_id)
    ON CONFLICT(teacher_id,class_id,academic_year_id) DO UPDATE SET can_score=excluded.can_score, school_id=excluded.school_id;

  FOR v_student_id, v_class_id, v_sheet_year_id, v_sheet_term_id IN
    SELECT en.student_id,en.class_id,en.academic_year_id,
      CASE WHEN en.academic_year_id=v_year_id THEN v_term1_id ELSE v_previous_term4_id END
    FROM enrollments en JOIN students st ON st.id=en.student_id
    WHERE st.school_id=v_school_id AND en.academic_year_id IN (v_previous_year_id,v_year_id)
  LOOP
    v_sheet_id := NULL;
    v_existing_id := NULL;
    INSERT INTO behavior_sheets(id,class_id,student_id,academic_year_id,term_id,total_score,verdict,updated_by,updated_at,deleted_at,is_complete,school_id)
      VALUES(gen_random_uuid(),v_class_id,v_student_id,v_sheet_year_id,v_sheet_term_id,24,'образцовое',v_teacher3_id,
        CASE WHEN v_sheet_year_id=v_year_id THEN '2026-10-31 15:00:00+03'::timestamptz ELSE '2026-05-31 15:00:00+03'::timestamptz END,
        NULL,true,v_school_id)
      ON CONFLICT DO NOTHING RETURNING id INTO v_existing_id;
    IF v_existing_id IS NULL THEN SELECT bs.id INTO v_existing_id FROM behavior_sheets bs WHERE bs.student_id=v_student_id AND bs.academic_year_id=v_sheet_year_id AND bs.term_id=v_sheet_term_id; END IF;
    v_sheet_id := v_existing_id;
    INSERT INTO behavior_scores(id,behavior_sheet_id,criterion_id,score,school_id)
      SELECT gen_random_uuid(),v_sheet_id,bc.id,CASE WHEN bc.sort_order<=4 THEN 3 ELSE 2 END,v_school_id
      FROM behavior_criteria bc WHERE bc.school_id=v_school_id ON CONFLICT DO NOTHING;
  END LOOP;
  INSERT INTO projects(id,school_id,name) VALUES
    (gen_random_uuid(),v_school_id,'Научная лаборатория'),(gen_random_uuid(),v_school_id,'Читающий класс')
    ON CONFLICT(school_id,name) DO NOTHING;
  INSERT INTO student_projects(student_id,project_id,academic_year_id,school_id)
    SELECT st.id,p.id,v_year_id,v_school_id FROM students st CROSS JOIN projects p
    WHERE st.school_id=v_school_id AND p.school_id=v_school_id AND p.name='Научная лаборатория'
    ON CONFLICT(student_id,project_id,academic_year_id) DO NOTHING;
  INSERT INTO student_projects(student_id,project_id,academic_year_id,school_id)
    SELECT st.id,p.id,v_year_id,v_school_id FROM students st CROSS JOIN projects p
    WHERE st.school_id=v_school_id AND st.external_id IN ('DEMO-001','DEMO-003','DEMO-005','DEMO-007','DEMO-009')
      AND p.school_id=v_school_id AND p.name='Читающий класс'
    ON CONFLICT(student_id,project_id,academic_year_id) DO NOTHING;
  INSERT INTO invites(id,school_id,email,role_id,target_type,target_id,academic_year_id,token_hash,expires_at,used_at,created_by,created_at)
    SELECT gen_random_uuid(),v_school_id,'former.class.teacher@demo-school.local',r.id,'class',v_class_a_id,v_year_id,
      encode(digest('demo-invite-used-history-only','sha256'),'hex'),'2027-01-01 00:00:00+03','2026-10-15 12:00:00+03',v_admin_id,'2026-10-08 12:00:00+03'
    FROM roles r WHERE r.code='class_teacher'
    ON CONFLICT(token_hash) DO UPDATE SET email=excluded.email,role_id=excluded.role_id,target_type=excluded.target_type,
      target_id=excluded.target_id,academic_year_id=excluded.academic_year_id,expires_at=excluded.expires_at,used_at=excluded.used_at,
      created_by=excluded.created_by,created_at=excluded.created_at;
  INSERT INTO refresh_sessions(id,user_id,token_hash,expires_at,revoked_at,ip,user_agent,created_at)
    VALUES(gen_random_uuid(),v_admin_id,encode(digest('demo-revoked-session-history-only','sha256'),'hex'),
      '2026-10-20 12:00:00+03','2026-10-18 12:00:00+03','127.0.0.1'::inet,'Demo seed: historical session, intentionally revoked','2026-10-01 12:00:00+03')
    ON CONFLICT(token_hash) DO UPDATE SET user_id=excluded.user_id,expires_at=excluded.expires_at,revoked_at=excluded.revoked_at,
      ip=excluded.ip,user_agent=excluded.user_agent,created_at=excluded.created_at;
  INSERT INTO audit_log(id,actor_user_id,action,entity,entity_id,old_value,new_value,ip,user_agent,created_at)
    SELECT nextval(pg_get_serial_sequence('audit_log','id')),v_admin_id,'seed_demo','schools',v_school_id::text,'{}'::jsonb,
      '{"dataset":"demo-school-2026-full"}'::jsonb,'127.0.0.1'::inet,'Demo data seed','2026-09-01 09:00:00+03'
    WHERE NOT EXISTS(SELECT 1 FROM audit_log WHERE action='seed_demo' AND entity='schools' AND entity_id=v_school_id::text AND new_value->>'dataset'='demo-school-2026-full');
END $$;

COMMIT;
