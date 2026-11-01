package attendance

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"school-journal/internal/audit"
	"school-journal/internal/identity"
	"school-journal/internal/platform/httpx"
)

type Handler struct{ db *sql.DB }

func New(db *sql.DB) *Handler { return &Handler{db: db} }

func (h *Handler) Register(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("GET /attendance/contexts", protect(http.HandlerFunc(h.contexts)))
	mux.Handle("GET /attendance", protect(http.HandlerFunc(h.listDay)))
	mux.Handle("PUT /attendance", protect(http.HandlerFunc(h.saveDay)))
}

type contextRow struct {
	classID, className, yearID, yearName string
	canEdit                              bool
}

func (h *Handler) contexts(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	var rows *sql.Rows
	var err error
	if identity.IsAdmin(p) {
		rows, err = h.db.QueryContext(r.Context(), `SELECT c.id::text,c.name,y.id::text,y.name,true
			FROM classes c JOIN academic_years y ON y.id=c.academic_year_id AND y.school_id=c.school_id
			WHERE c.school_id=$1 ORDER BY y.is_current DESC,y.starts_on DESC,c.name LIMIT 1000`, p.SchoolID)
	} else if identity.IsStudent(p) {
		rows, err = h.db.QueryContext(r.Context(), `SELECT DISTINCT c.id::text,c.name,y.id::text,y.name,false
			FROM students s JOIN enrollments e ON e.student_id=s.id AND e.school_id=s.school_id
			JOIN classes c ON c.id=e.class_id AND c.school_id=e.school_id
			JOIN academic_years y ON y.id=e.academic_year_id AND y.school_id=e.school_id
			WHERE s.user_id=$1 AND s.school_id=$2 AND (e.from_date IS NULL OR e.from_date<=CURRENT_DATE) AND (e.to_date IS NULL OR e.to_date>=CURRENT_DATE) ORDER BY y.name DESC,c.name LIMIT 1000`, p.UserID, p.SchoolID)
	} else if identity.HasRole(p, "teacher") || identity.HasRole(p, "class_teacher") {
		rows, err = h.db.QueryContext(r.Context(), `SELECT DISTINCT c.id::text,c.name,y.id::text,y.name,
			(EXISTS(SELECT 1 FROM teacher_assignments ta WHERE ta.teacher_id=$1 AND ta.class_id=c.id AND ta.academic_year_id=y.id AND ta.school_id=$2)
			 OR EXISTS(SELECT 1 FROM class_teacher_assignments ca WHERE ca.teacher_id=$1 AND ca.class_id=c.id AND ca.academic_year_id=y.id AND ca.school_id=$2))
			FROM classes c JOIN academic_years y ON y.id=c.academic_year_id AND y.school_id=c.school_id
			WHERE c.school_id=$2 AND (
				EXISTS(SELECT 1 FROM teacher_assignments ta WHERE ta.teacher_id=$1 AND ta.class_id=c.id AND ta.academic_year_id=y.id AND ta.school_id=$2)
				OR EXISTS(SELECT 1 FROM class_teacher_assignments ca WHERE ca.teacher_id=$1 AND ca.class_id=c.id AND ca.academic_year_id=y.id AND ca.school_id=$2))
			ORDER BY y.is_current DESC,y.starts_on DESC,c.name LIMIT 1000`, p.UserID, p.SchoolID)
	} else {
		httpx.Error(w, http.StatusForbidden, "forbidden", "Insufficient role")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not list attendance contexts")
		return
	}
	defer rows.Close()
	result := make([]map[string]any, 0)
	for rows.Next() {
		var item contextRow
		if err := rows.Scan(&item.classID, &item.className, &item.yearID, &item.yearName, &item.canEdit); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read attendance contexts")
			return
		}
		result = append(result, map[string]any{"class_id": item.classID, "class_name": item.className, "academic_year_id": item.yearID, "academic_year_name": item.yearName, "can_edit": item.canEdit})
	}
	if err := rows.Err(); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read attendance contexts")
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (h *Handler) listDay(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	classID, date := strings.TrimSpace(r.URL.Query().Get("class_id")), strings.TrimSpace(r.URL.Query().Get("date"))
	parsedDate, err := time.Parse("2006-01-02", date)
	if classID == "" || err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "class_id and a valid ISO date are required")
		return
	}
	yearID, canEdit, allowed := h.authorizeClass(r, p, classID, parsedDate)
	if !allowed {
		httpx.Error(w, http.StatusForbidden, "forbidden", "No access to this class and date")
		return
	}
	query := `SELECT s.id::text,s.full_name,COALESCE(a.status,''),COALESCE(a.note,'')
		FROM enrollments e JOIN students s ON s.id=e.student_id AND s.school_id=e.school_id
		LEFT JOIN attendance_records a ON a.student_id=s.id AND a.class_id=e.class_id AND a.attendance_date=$3 AND a.school_id=e.school_id
		WHERE e.class_id=$1 AND e.academic_year_id=$2 AND (e.from_date IS NULL OR e.from_date<=$3) AND (e.to_date IS NULL OR e.to_date>=$3) AND e.school_id=$4`
	args := []any{classID, yearID, parsedDate, p.SchoolID}
	if identity.IsStudent(p) {
		query += " AND s.user_id=$5"
		args = append(args, p.UserID)
	}
	query += " ORDER BY s.full_name LIMIT 500"
	rows, err := h.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read attendance")
		return
	}
	defer rows.Close()
	result := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, status, note string
		if err := rows.Scan(&id, &name, &status, &note); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read attendance")
			return
		}
		result = append(result, map[string]any{"student_id": id, "student_name": name, "status": status, "note": note})
	}
	if err := rows.Err(); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read attendance")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"class_id": classID, "date": date, "can_edit": canEdit, "students": result})
}

type attendanceInput struct {
	StudentID string `json:"student_id"`
	Status    string `json:"status"`
	Note      string `json:"note"`
}

func (h *Handler) saveDay(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	var input struct {
		ClassID string            `json:"class_id"`
		Date    string            `json:"date"`
		Records []attendanceInput `json:"records"`
	}
	if httpx.Decode(w, r, &input) != nil || strings.TrimSpace(input.ClassID) == "" || len(input.Records) == 0 || len(input.Records) > 500 {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "class_id and 1..500 attendance records are required")
		return
	}
	workDate, err := time.Parse("2006-01-02", input.Date)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "date must use YYYY-MM-DD")
		return
	}
	yearID, _, allowed := h.authorizeClass(r, p, input.ClassID, workDate)
	if !allowed || identity.IsStudent(p) {
		httpx.Error(w, http.StatusForbidden, "forbidden", "You cannot record attendance for this class and date")
		return
	}
	seen := make(map[string]bool, len(input.Records))
	for i := range input.Records {
		input.Records[i].StudentID = strings.TrimSpace(input.Records[i].StudentID)
		input.Records[i].Status = strings.TrimSpace(input.Records[i].Status)
		if input.Records[i].StudentID == "" || seen[input.Records[i].StudentID] || (input.Records[i].Status != "" && !validStatus(input.Records[i].Status)) || len(input.Records[i].Note) > 500 {
			httpx.Error(w, http.StatusBadRequest, "invalid_record", "Every record needs a unique student, valid status, and note no longer than 500 characters")
			return
		}
		seen[input.Records[i].StudentID] = true
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not save attendance")
		return
	}
	defer tx.Rollback()
	for _, record := range input.Records {
		var valid bool
		if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM enrollments e WHERE e.student_id=$1 AND e.class_id=$2 AND e.academic_year_id=$3 AND e.school_id=$4 AND e.status IN ('active','transferred','completed') AND (e.from_date IS NULL OR e.from_date<=$5) AND (e.to_date IS NULL OR e.to_date>=$5))`, record.StudentID, input.ClassID, yearID, p.SchoolID, workDate).Scan(&valid); err != nil || !valid {
			httpx.Error(w, http.StatusBadRequest, "student_not_enrolled", "One or more students are not actively enrolled in this class")
			return
		}
		var oldStatus, oldNote string
		err := tx.QueryRowContext(r.Context(), `SELECT status,note FROM attendance_records WHERE student_id=$1 AND class_id=$2 AND attendance_date=$3 AND school_id=$4 FOR UPDATE`, record.StudentID, input.ClassID, workDate, p.SchoolID).Scan(&oldStatus, &oldNote)
		if err != nil && err != sql.ErrNoRows {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read existing attendance")
			return
		}
		if record.Status == "" {
			if oldStatus == "" {
				record.Status = "present"
			} else {
				record.Status = oldStatus
			}
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO attendance_records(school_id,student_id,class_id,attendance_date,status,note,created_by)
			VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(student_id,class_id,attendance_date)
			DO UPDATE SET status=excluded.status,note=excluded.note,updated_at=now()`, p.SchoolID, record.StudentID, input.ClassID, workDate, record.Status, record.Note, p.UserID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not save attendance")
			return
		}
		oldValue := any(nil)
		if oldStatus != "" {
			oldValue = map[string]string{"status": oldStatus, "note": oldNote}
		}
		if err := audit.Write(r.Context(), tx, r, p, "upsert", "attendance", record.StudentID+":"+input.ClassID+":"+input.Date, oldValue, map[string]string{"status": record.Status, "note": record.Note}); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "audit_error", "Could not record attendance change")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not save attendance")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"saved": len(input.Records)})
}

func (h *Handler) authorizeClass(r *http.Request, p identity.Principal, classID string, date time.Time) (string, bool, bool) {
	if !identity.IsAdmin(p) && !identity.HasRole(p, "teacher") && !identity.HasRole(p, "class_teacher") && !identity.IsStudent(p) {
		return "", false, false
	}
	var yearID string
	var canEdit bool
	err := h.db.QueryRowContext(r.Context(), `SELECT y.id::text,
		($3 OR EXISTS(SELECT 1 FROM teacher_assignments ta WHERE ta.teacher_id=$4 AND ta.class_id=c.id AND ta.academic_year_id=y.id AND ta.school_id=$1)
		 OR EXISTS(SELECT 1 FROM class_teacher_assignments ca WHERE ca.teacher_id=$4 AND ca.class_id=c.id AND ca.academic_year_id=y.id AND ca.school_id=$1))
		FROM classes c JOIN academic_years y ON y.id=c.academic_year_id AND y.school_id=c.school_id
		WHERE c.id=$2 AND c.school_id=$1 AND $5 BETWEEN y.starts_on AND y.ends_on`, p.SchoolID, classID, identity.IsAdmin(p), p.UserID, date).Scan(&yearID, &canEdit)
	if err != nil {
		return "", false, false
	}
	if identity.IsStudent(p) {
		var enrollmentStatus string
		var fromDate, toDate sql.NullTime
		err = h.db.QueryRowContext(r.Context(), `SELECT e.status,e.from_date,e.to_date FROM students s JOIN enrollments e ON e.student_id=s.id AND e.school_id=s.school_id WHERE s.user_id=$1 AND s.school_id=$2 AND e.class_id=$3 AND e.academic_year_id=$4`, p.UserID, p.SchoolID, classID, yearID).Scan(&enrollmentStatus, &fromDate, &toDate)
		enrolledOnDate := err == nil && (!fromDate.Valid || !date.Before(fromDate.Time)) && (!toDate.Valid || !date.After(toDate.Time)) && enrollmentStatus != ""
		return yearID, false, enrolledOnDate
	}
	return yearID, canEdit, err == nil && canEdit
}

func validStatus(status string) bool {
	switch status {
	case "present", "absent", "late", "excused":
		return true
	default:
		return false
	}
}
