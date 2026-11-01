-- Daily attendance records for the electronic class journal.
CREATE TABLE attendance_records (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  school_id uuid NOT NULL REFERENCES schools(id) ON DELETE RESTRICT,
  student_id uuid NOT NULL,
  class_id uuid NOT NULL,
  attendance_date date NOT NULL,
  status text NOT NULL CHECK (status IN ('present','absent','late','excused')),
  note text NOT NULL DEFAULT '' CHECK (length(note) <= 500),
  created_by uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (student_id, class_id, attendance_date),
  FOREIGN KEY (student_id, school_id) REFERENCES students(id, school_id) ON DELETE RESTRICT,
  FOREIGN KEY (class_id, school_id) REFERENCES classes(id, school_id) ON DELETE RESTRICT,
  FOREIGN KEY (created_by, school_id) REFERENCES users(id, school_id) ON DELETE RESTRICT
);
CREATE INDEX attendance_class_date_lookup ON attendance_records(school_id, class_id, attendance_date);
ALTER TABLE attendance_records ENABLE ROW LEVEL SECURITY;
