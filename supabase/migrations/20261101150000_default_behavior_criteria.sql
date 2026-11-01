-- Initialize the standard behavior rubric for schools that have none.
-- Existing school criteria (including Excel-customized titles and codes) are preserved.
-- This is a normal schema migration; it does not create demo users or student data.
WITH defaults(code,title,sort_order) AS (
  VALUES
    ('schedule_rules','Расписание, правила и нормы поведения в учебной деятельности (в том числе внеурочной).',1),
    ('appearance','Одежда обучающегося и требования к внешнему виду.',2),
    ('civic_actions','Нормы поведения при гражданско-патриотических акциях.',3),
    ('safety','Правила пожарной безопасности, электробезопасности, поведения при угрозе ЧС.',4),
    ('peer_communication','Конструктивное взаимодействие со сверстниками, общепринятые нормы коммуникации.',5),
    ('conflict_prevention','Отсутствие умышленных конфликтов и агрессии (травли).',6),
    ('teacher_communication','Уважительное взаимодействие с педагогами, нормы коммуникации.',7),
    ('responsibility','Ответственное отношение к поручениям и рекомендациям педагогов.',8),
    ('lawful_behavior','Отсутствие фактов антиобщественных действий и противоправного поведения.',9),
    ('prevention_records','Отсутствие индивидуальной профилактической работы / учёта.',10)
)
INSERT INTO behavior_criteria(id,school_id,code,title,description,max_score,sort_order)
SELECT gen_random_uuid(),s.id,d.code,d.title,NULL,3,d.sort_order
FROM schools s CROSS JOIN defaults d
WHERE NOT EXISTS (SELECT 1 FROM behavior_criteria bc WHERE bc.school_id=s.id)
ON CONFLICT(school_id,code) DO NOTHING;
