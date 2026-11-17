package admin

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"school-journal/internal/audit"
	"school-journal/internal/identity"
	"school-journal/internal/platform/httpx"
)

type classStudentInput struct {
	StudentID  string `json:"student_id"`
	FullName   string `json:"full_name"`
	ExternalID string `json:"external_id"`
	BirthDate  string `json:"birth_date"`
}

func normalizeClassStudents(students []classStudentInput) error {
	if len(students) == 0 || len(students) > 100 {
		return fmt.Errorf("Добавьте от 1 до 100 учеников")
	}
	names, numbers, ids := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for i := range students {
		s := &students[i]
		s.StudentID = strings.TrimSpace(s.StudentID)
		s.FullName = strings.Join(strings.Fields(s.FullName), " ")
		s.ExternalID = strings.TrimSpace(s.ExternalID)
		s.BirthDate = strings.TrimSpace(s.BirthDate)
		if s.StudentID != "" {
			if ids[s.StudentID] || s.FullName != "" || s.ExternalID != "" || s.BirthDate != "" {
				return fmt.Errorf("Строка %d: выберите существующего ученика без данных новой карточки", i+1)
			}
			ids[s.StudentID] = true
			continue
		}
		name := strings.ToLower(s.FullName)
		if name == "" || utf8.RuneCountInString(s.FullName) > 200 {
			return fmt.Errorf("Строка %d: укажите ФИО до 200 символов", i+1)
		}
		if names[name] {
			return fmt.Errorf("Строка %d: ФИО повторяется в списке", i+1)
		}
		names[name] = true
		if utf8.RuneCountInString(s.ExternalID) > 64 || s.ExternalID != "" && numbers[s.ExternalID] {
			return fmt.Errorf("Строка %d: табельный номер повторяется или длиннее 64 символов", i+1)
		}
		if s.ExternalID != "" {
			numbers[s.ExternalID] = true
		}
		if s.BirthDate != "" {
			if _, err := time.Parse("2006-01-02", s.BirthDate); err != nil {
				return fmt.Errorf("Строка %d: некорректная дата рождения", i+1)
			}
		}
	}
	return nil
}

func studentNumber() (string, error) {
	token, err := randomToken(12)
	if err != nil {
		return "", err
	}
	return "ST-" + token, nil
}

func (h *Handler) addClassStudents(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok || !identity.IsAdmin(p) {
		httpx.Error(w, 403, "forbidden", "Administrator access required")
		return
	}
	var input struct {
		FromDate string              `json:"from_date"`
		Students []classStudentInput `json:"students"`
	}
	if httpx.Decode(w, r, &input) != nil {
		httpx.Error(w, 400, "invalid_input", "Некорректные данные списка")
		return
	}
	if err := normalizeClassStudents(input.Students); err != nil {
		httpx.Error(w, 400, "invalid_students", err.Error())
		return
	}
	input.FromDate = strings.TrimSpace(input.FromDate)
	if input.FromDate != "" {
		if _, err := time.Parse("2006-01-02", input.FromDate); err != nil {
			httpx.Error(w, 400, "invalid_date", "Укажите дату зачисления в формате ГГГГ-ММ-ДД")
			return
		}
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Error(w, 500, "db_error", "Не удалось начать зачисление")
		return
	}
	defer tx.Rollback()
	var yearID string
	var start, end time.Time
	err = tx.QueryRowContext(r.Context(), `SELECT y.id::text,y.starts_on,y.ends_on FROM classes c JOIN academic_years y ON y.id=c.academic_year_id AND y.school_id=c.school_id WHERE c.id=$1 AND c.school_id=$2 FOR SHARE OF c,y`, r.PathValue("id"), p.SchoolID).Scan(&yearID, &start, &end)
	if err == sql.ErrNoRows {
		httpx.Error(w, 404, "not_found", "Класс не найден")
		return
	}
	if err != nil {
		httpx.Error(w, 500, "db_error", "Не удалось получить учебный год класса")
		return
	}
	if input.FromDate == "" {
		input.FromDate = start.Format("2006-01-02")
	}
	date, _ := time.Parse("2006-01-02", input.FromDate)
	if date.Before(start) || date.After(end) {
		httpx.Error(w, 400, "invalid_date", "Дата зачисления должна входить в учебный год класса")
		return
	}
	// Serialize bulk additions within a school to avoid concurrent name duplicates.
	if _, err = tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, p.SchoolID); err != nil {
		httpx.Error(w, 500, "db_error", "Не удалось начать зачисление")
		return
	}
	created := 0
	for i, item := range input.Students {
		fail := func(status int, code, message string) {
			httpx.JSON(w, status, httpx.ErrorResponse{Code: code, Message: fmt.Sprintf("Строка %d: %s", i+1, message), Details: []map[string]any{{"row": i + 1, "reason": message}}})
		}
		id := item.StudentID
		if id == "" {
			var duplicate bool
			err = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM students WHERE school_id=$1 AND lower(regexp_replace(trim(full_name),'\s+',' ','g'))=lower($2))`, p.SchoolID, item.FullName).Scan(&duplicate)
			if err != nil {
				fail(500, "db_error", "Не удалось проверить карточки учеников")
				return
			}
			if duplicate {
				fail(409, "student_exists", "Карточка с таким ФИО уже существует. Выберите существующего ученика.")
				return
			}
			number := item.ExternalID
			if number == "" {
				number, err = studentNumber()
				if err != nil {
					fail(500, "internal", "Не удалось создать табельный номер")
					return
				}
			}
			err = tx.QueryRowContext(r.Context(), `INSERT INTO students(school_id,external_id,full_name,birth_date) VALUES($1,$2,$3,NULLIF($4,'')::date) RETURNING id::text`, p.SchoolID, number, item.FullName, item.BirthDate).Scan(&id)
			if err != nil {
				fail(409, "student_conflict", "Не удалось создать ученика. Проверьте уникальность табельного номера.")
				return
			}
			created++
		} else {
			var exists bool
			err = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM students WHERE id=$1 AND school_id=$2)`, id, p.SchoolID).Scan(&exists)
			if err != nil || !exists {
				fail(404, "not_found", "Ученик не найден")
				return
			}
		}
		var enrollmentID string
		err = tx.QueryRowContext(r.Context(), `INSERT INTO enrollments(student_id,class_id,academic_year_id,status,from_date,school_id) VALUES($1,$2,$3,'active',$4,$5) RETURNING id::text`, id, r.PathValue("id"), yearID, input.FromDate, p.SchoolID).Scan(&enrollmentID)
		if err != nil {
			fail(409, "enrollment_conflict", "Ученик уже зачислен в этом учебном году или данные зачисления некорректны")
			return
		}
		err = audit.Write(r.Context(), tx, r, p, "create", "enrollments", enrollmentID, nil, map[string]any{"student_id": id, "class_id": r.PathValue("id"), "academic_year_id": yearID, "from_date": input.FromDate, "student_created": item.StudentID == ""})
		if err != nil {
			fail(500, "audit_error", "Не удалось записать изменение в аудит")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		httpx.Error(w, 500, "db_error", "Не удалось сохранить зачисления")
		return
	}
	httpx.JSON(w, 201, map[string]int{"created": created, "enrolled": len(input.Students)})
}
