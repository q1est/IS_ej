package grades

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"school-journal/internal/audit"
	"school-journal/internal/identity"
	"school-journal/internal/platform/httpx"
)

type Handler struct{ db *sql.DB }

type Grade struct {
	StudentID string `json:"student_id"`
	SubjectID string `json:"subject_id"`
	ClassID   string `json:"class_id"`
	TermID    string `json:"term_id"`
	Value     int    `json:"value"`
	GradeType string `json:"grade_type"`
	WorkDate  string `json:"work_date"`
	Comment   string `json:"comment"`
}

func New(db *sql.DB) *Handler { return &Handler{db: db} }

func (h *Handler) Register(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("GET /grades/contexts", protect(http.HandlerFunc(h.contexts)))
	mux.Handle("GET /grades/assignments", protect(http.HandlerFunc(h.assignments)))
	mux.Handle("GET /grades", protect(http.HandlerFunc(h.list)))
	mux.Handle("POST /grades", protect(http.HandlerFunc(h.create)))
	mux.Handle("PATCH /grades/{id}", protect(http.HandlerFunc(h.update)))
	mux.Handle("DELETE /grades/{id}", protect(http.HandlerFunc(h.delete)))
	mux.Handle("GET /grades/class-summary", protect(http.HandlerFunc(h.classSummary)))
}

// contexts returns only the class/subject/term combinations visible to the
// authenticated user. It keeps the UI from needing to guess or expose UUIDs.
func (h *Handler) contexts(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	var query string
	var args []any
	switch {
	case identity.IsAdmin(p):
		query = `SELECT c.id::text,c.name,t.id::text,t.name,su.id::text,su.name,true,c.academic_year_id::text
			FROM classes c JOIN terms t ON t.academic_year_id=c.academic_year_id
			JOIN subjects su ON su.school_id=c.school_id WHERE c.school_id=$1
			ORDER BY c.name,su.name,t.starts_on LIMIT 1000`
		args = []any{p.SchoolID}
	case identity.HasRole(p, "student"):
		query = `SELECT c.id::text,c.name,t.id::text,t.name,NULL::text,NULL::text,false,c.academic_year_id::text
			FROM students s JOIN enrollments e ON e.student_id=s.id AND e.status='active'
			JOIN classes c ON c.id=e.class_id AND c.school_id=e.school_id
			JOIN terms t ON t.academic_year_id=e.academic_year_id
			WHERE s.user_id=$1 AND e.school_id=$2 ORDER BY c.name,t.starts_on LIMIT 1000`
		args = []any{p.UserID, p.SchoolID}
	default:
		if !identity.HasRole(p, "teacher") && !identity.HasRole(p, "class_teacher") {
			httpx.Error(w, http.StatusForbidden, "forbidden", "Insufficient role")
			return
		}
		query = `SELECT c.id::text,c.name,t.id::text,t.name,su.id::text,su.name,true,c.academic_year_id::text
			FROM teacher_assignments ta JOIN classes c ON c.id=ta.class_id
			JOIN subjects su ON su.id=ta.subject_id JOIN terms t ON t.academic_year_id=ta.academic_year_id
			WHERE ta.teacher_id=$1 AND ta.school_id=$2
			UNION ALL
		SELECT c.id::text,c.name,t.id::text,t.name,NULL::text,NULL::text,false,c.academic_year_id::text
			FROM class_teacher_assignments ca JOIN classes c ON c.id=ca.class_id
			JOIN terms t ON t.academic_year_id=ca.academic_year_id
			WHERE ca.teacher_id=$1 AND ca.school_id=$2
			ORDER BY 2,6,4 LIMIT 1000`
		args = []any{p.UserID, p.SchoolID}
	}
	rows, err := h.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not list grade contexts")
		return
	}
	defer rows.Close()
	result := make([]map[string]any, 0)
	for rows.Next() {
		var classID, className, termID, termName string
		var subjectID, subjectName sql.NullString
		var canEdit bool
		var yearID string
		if err := rows.Scan(&classID, &className, &termID, &termName, &subjectID, &subjectName, &canEdit, &yearID); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read grade contexts")
			return
		}
		var subjectIDValue, subjectNameValue any
		if subjectID.Valid {
			subjectIDValue, subjectNameValue = subjectID.String, subjectName.String
		}
		result = append(result, map[string]any{
			"class_id": classID, "class_name": className, "term_id": termID,
			"term_name": termName, "subject_id": subjectIDValue,
			"subject_name": subjectNameValue, "can_edit": canEdit, "academic_year_id": yearID,
		})
	}
	if err := rows.Err(); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read grade contexts")
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (h *Handler) assignments(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	if identity.IsAdmin(p) {
		rows, err := h.db.QueryContext(r.Context(), `SELECT ta.id::text,ta.class_id::text,c.name,ta.subject_id::text,s.name,
			ta.academic_year_id::text,t.id::text,t.name FROM teacher_assignments ta JOIN classes c ON c.id=ta.class_id
			JOIN subjects s ON s.id=ta.subject_id JOIN terms t ON t.academic_year_id=ta.academic_year_id
			WHERE ta.school_id=$1 ORDER BY c.name,s.name,t.starts_on`, p.SchoolID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not list assignments")
			return
		}
		writeAssignments(w, rows)
		return
	}
	if !identity.HasRole(p, "teacher") {
		httpx.Error(w, http.StatusForbidden, "forbidden", "Teacher role required")
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `SELECT ta.id::text,ta.class_id::text,c.name,ta.subject_id::text,s.name,
		ta.academic_year_id::text,t.id::text,t.name FROM teacher_assignments ta JOIN classes c ON c.id=ta.class_id
		JOIN subjects s ON s.id=ta.subject_id JOIN terms t ON t.academic_year_id=ta.academic_year_id
		WHERE ta.teacher_id=$1 AND ta.school_id=$2 ORDER BY c.name,s.name,t.starts_on`, p.UserID, p.SchoolID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not list assignments")
		return
	}
	writeAssignments(w, rows)
}

func writeAssignments(w http.ResponseWriter, rows *sql.Rows) {
	defer rows.Close()
	result := make([]map[string]string, 0)
	for rows.Next() {
		var assignmentID, classID, className, subjectID, subjectName, yearID, termID, termName string
		if err := rows.Scan(&assignmentID, &classID, &className, &subjectID, &subjectName, &yearID, &termID, &termName); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read assignments")
			return
		}
		result = append(result, map[string]string{"assignment_id": assignmentID, "class_id": classID, "class_name": className, "subject_id": subjectID, "subject_name": subjectName, "academic_year_id": yearID, "term_id": termID, "term_name": termName})
	}
	if err := rows.Err(); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read assignments")
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	classID := strings.TrimSpace(r.URL.Query().Get("class_id"))
	termID := strings.TrimSpace(r.URL.Query().Get("term_id"))
	subjectID := strings.TrimSpace(r.URL.Query().Get("subject_id"))
	if termID == "" || (!identity.HasRole(p, "student") && classID == "") {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "term_id and class_id are required")
		return
	}
	query := `SELECT g.id::text,g.student_id::text,s.full_name,g.subject_id::text,g.class_id::text,g.term_id::text,
		g.value,g.grade_type,g.work_date::text,COALESCE(g.comment,'') FROM grades g JOIN students s ON s.id=g.student_id
		JOIN classes c ON c.id=g.class_id JOIN terms t ON t.id=g.term_id WHERE g.deleted_at IS NULL AND c.school_id=$1 AND g.term_id=$2`
	args := []any{p.SchoolID, termID}
	if !identity.IsAdmin(p) && !identity.HasRole(p, "student") {
		switch {
		case identity.HasRole(p, "teacher"):
			var allowed bool
			err := h.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM teacher_assignments ta JOIN terms t ON t.id=$3 AND t.academic_year_id=ta.academic_year_id
				WHERE ta.teacher_id=$1 AND ta.class_id=$2 AND ta.school_id=$4 AND ($5='' OR ta.subject_id::text=$5))`,
				p.UserID, classID, termID, p.SchoolID, subjectID).Scan(&allowed)
			if err != nil || !allowed {
				httpx.Error(w, http.StatusForbidden, "forbidden", "No assignment for this class, subject, and period")
				return
			}
		case identity.HasRole(p, "class_teacher"):
			var allowed bool
			err := h.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM class_teacher_assignments ca JOIN terms t ON t.id=$3 AND t.academic_year_id=ca.academic_year_id
				WHERE ca.teacher_id=$1 AND ca.class_id=$2 AND ca.school_id=$4)`, p.UserID, classID, termID, p.SchoolID).Scan(&allowed)
			if err != nil || !allowed {
				httpx.Error(w, http.StatusForbidden, "forbidden", "Not the class teacher of this class")
				return
			}
		default:
			httpx.Error(w, http.StatusForbidden, "forbidden", "Insufficient role")
			return
		}
	}
	if classID != "" {
		args = append(args, classID)
		query += " AND g.class_id=$" + strconv.Itoa(len(args))
	}
	if subjectID != "" {
		args = append(args, subjectID)
		query += " AND g.subject_id=$" + strconv.Itoa(len(args))
	}
	if identity.HasRole(p, "student") {
		if p.StudentID == "" {
			httpx.JSON(w, http.StatusOK, []any{})
			return
		}
		args = append(args, p.StudentID)
		query += " AND g.student_id=$" + strconv.Itoa(len(args))
	} else if identity.IsAdmin(p) {
		// Full school scope.
	} else if identity.HasRole(p, "teacher") {
		args = append(args, p.UserID)
		query += " AND EXISTS(SELECT 1 FROM teacher_assignments ta WHERE ta.teacher_id=$" + strconv.Itoa(len(args)) + " AND ta.class_id=g.class_id AND ta.subject_id=g.subject_id AND ta.academic_year_id=t.academic_year_id AND ta.school_id=$1)"
	} else if identity.HasRole(p, "class_teacher") {
		args = append(args, p.UserID)
		query += " AND EXISTS(SELECT 1 FROM class_teacher_assignments ca WHERE ca.teacher_id=$" + strconv.Itoa(len(args)) + " AND ca.class_id=g.class_id AND ca.academic_year_id=t.academic_year_id AND ca.school_id=$1)"
	} else {
		httpx.Error(w, http.StatusForbidden, "forbidden", "Insufficient role")
		return
	}
	query += " ORDER BY s.full_name,g.work_date,g.id LIMIT 1000"
	rows, err := h.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not list grades")
		return
	}
	defer rows.Close()
	result := make([]map[string]any, 0)
	for rows.Next() {
		var id string
		var grade Grade
		var name string
		if err := rows.Scan(&id, &grade.StudentID, &name, &grade.SubjectID, &grade.ClassID, &grade.TermID, &grade.Value, &grade.GradeType, &grade.WorkDate, &grade.Comment); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read grades")
			return
		}
		item := map[string]any{"id": id, "student_id": grade.StudentID, "student_name": name, "subject_id": grade.SubjectID, "class_id": grade.ClassID, "term_id": grade.TermID, "value": grade.Value, "grade_type": grade.GradeType, "work_date": grade.WorkDate, "comment": grade.Comment}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read grades")
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	var input Grade
	if httpx.Decode(w, r, &input) != nil || input.StudentID == "" || input.SubjectID == "" || input.ClassID == "" || input.TermID == "" || input.Value < 1 || input.Value > 5 || !validWorkDate(input.WorkDate) {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "Student, class, subject, term, value, and work_date are required")
		return
	}
	if input.GradeType == "" {
		input.GradeType = "regular"
	}
	if !h.canTeach(r.Context(), p, input.ClassID, input.SubjectID, input.TermID) {
		httpx.Error(w, http.StatusForbidden, "forbidden", "No assignment for this class and subject")
		return
	}
	var gradeID string
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create grade")
		return
	}
	defer tx.Rollback()
	var valid bool
	err = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM enrollments e JOIN classes c ON c.id=e.class_id
		JOIN terms t ON t.id=$3 AND t.academic_year_id=e.academic_year_id JOIN students s ON s.id=e.student_id
		JOIN subjects su ON su.id=$5 AND su.school_id=c.school_id WHERE e.student_id=$1 AND e.class_id=$2 AND e.status='active'
		AND c.school_id=$4 AND e.school_id=$4 AND s.school_id=$4 AND c.academic_year_id=e.academic_year_id)`, input.StudentID, input.ClassID, input.TermID, p.SchoolID, input.SubjectID).Scan(&valid)
	if err != nil || !valid {
		httpx.Error(w, http.StatusNotFound, "not_found", "Student is not enrolled in this class")
		return
	}
	err = tx.QueryRowContext(r.Context(), `INSERT INTO grades(student_id,subject_id,class_id,term_id,school_id,academic_year_id,value,grade_type,work_date,comment,created_by)
		SELECT $1,$2,c.id,$4,c.school_id,c.academic_year_id,$5,$6,$7,NULLIF($8,''),$9 FROM classes c
		JOIN terms t ON t.id=$4 AND t.academic_year_id=c.academic_year_id WHERE c.id=$3 AND c.school_id=$10 RETURNING id::text`,
		input.StudentID, input.SubjectID, input.ClassID, input.TermID, input.Value, input.GradeType, input.WorkDate, input.Comment, p.UserID, p.SchoolID).Scan(&gradeID)
	if err == nil {
		err = audit.Write(r.Context(), tx, r, p, "create", "grades", gradeID, nil, input)
	}
	if err != nil || tx.Commit() != nil {
		if isUniqueViolation(err) {
			httpx.Error(w, http.StatusConflict, "conflict", "A grade with the same work key already exists")
		} else {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create grade")
		}
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]string{"id": gradeID})
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	id := r.PathValue("id")
	var input struct {
		Value     *int    `json:"value"`
		GradeType *string `json:"grade_type"`
		WorkDate  *string `json:"work_date"`
		Comment   *string `json:"comment"`
	}
	if httpx.Decode(w, r, &input) != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "Invalid grade body")
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not update grade")
		return
	}
	defer tx.Rollback()
	var old Grade
	err = tx.QueryRowContext(r.Context(), `SELECT student_id::text,subject_id::text,class_id::text,term_id::text,value,grade_type,work_date::text,COALESCE(comment,'') FROM grades WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL FOR UPDATE`, id, p.SchoolID).
		Scan(&old.StudentID, &old.SubjectID, &old.ClassID, &old.TermID, &old.Value, &old.GradeType, &old.WorkDate, &old.Comment)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Grade not found")
		return
	}
	if !h.canTeachTx(r.Context(), tx, p, old.ClassID, old.SubjectID, old.TermID) {
		httpx.Error(w, http.StatusForbidden, "forbidden", "No assignment for this class and subject")
		return
	}
	updated := old
	if input.Value != nil {
		updated.Value = *input.Value
	}
	if input.GradeType != nil {
		updated.GradeType = strings.TrimSpace(*input.GradeType)
	}
	if input.WorkDate != nil {
		updated.WorkDate = strings.TrimSpace(*input.WorkDate)
	}
	if input.Comment != nil {
		updated.Comment = *input.Comment
	}
	if updated.Value < 1 || updated.Value > 5 || !validWorkDate(updated.WorkDate) || updated.GradeType == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "Invalid grade value, type, or date")
		return
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE grades SET value=$2,grade_type=$3,work_date=$4,comment=NULLIF($5,''),updated_at=now() WHERE id=$1`, id, updated.Value, updated.GradeType, updated.WorkDate, updated.Comment)
	if err == nil {
		err = audit.Write(r.Context(), tx, r, p, "update", "grades", id, old, updated)
	}
	if err != nil || tx.Commit() != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not update grade")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	id := r.PathValue("id")
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not delete grade")
		return
	}
	defer tx.Rollback()
	var old Grade
	err = tx.QueryRowContext(r.Context(), `SELECT student_id::text,subject_id::text,class_id::text,term_id::text,value,grade_type,work_date::text,COALESCE(comment,'') FROM grades WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL FOR UPDATE`, id, p.SchoolID).
		Scan(&old.StudentID, &old.SubjectID, &old.ClassID, &old.TermID, &old.Value, &old.GradeType, &old.WorkDate, &old.Comment)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Grade not found")
		return
	}
	if !h.canTeachTx(r.Context(), tx, p, old.ClassID, old.SubjectID, old.TermID) {
		httpx.Error(w, http.StatusForbidden, "forbidden", "No assignment for this class and subject")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE grades SET deleted_at=now(),updated_at=now() WHERE id=$1`, id); err == nil {
		err = audit.Write(r.Context(), tx, r, p, "soft_delete", "grades", id, old, nil)
	}
	if err != nil || tx.Commit() != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not delete grade")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) classSummary(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	classID, termID := strings.TrimSpace(r.URL.Query().Get("class_id")), strings.TrimSpace(r.URL.Query().Get("term_id"))
	if classID == "" || termID == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "class_id and term_id are required")
		return
	}
	if !identity.IsAdmin(p) {
		var allowed bool
		if err := h.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM class_teacher_assignments ca JOIN classes c ON c.id=ca.class_id JOIN terms t ON t.academic_year_id=ca.academic_year_id WHERE ca.teacher_id=$1 AND ca.class_id=$2 AND t.id=$3 AND c.school_id=$4 AND ca.school_id=$4 AND c.academic_year_id=ca.academic_year_id)`, p.UserID, classID, termID, p.SchoolID).Scan(&allowed); err != nil || !allowed {
			httpx.Error(w, http.StatusForbidden, "forbidden", "Not the class teacher of this class")
			return
		}
	}
	rows, err := h.db.QueryContext(r.Context(), `SELECT s.id::text,s.full_name,su.id::text,su.name,round(avg(g.value)::numeric,2),count(g.id)
		FROM grades g JOIN students s ON s.id=g.student_id JOIN subjects su ON su.id=g.subject_id JOIN classes c ON c.id=g.class_id
		WHERE g.class_id=$1 AND g.term_id=$2 AND g.deleted_at IS NULL AND c.school_id=$3 GROUP BY s.id,s.full_name,su.id,su.name ORDER BY s.full_name,su.name`, classID, termID, p.SchoolID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read class summary")
		return
	}
	defer rows.Close()
	result := make([]map[string]any, 0)
	for rows.Next() {
		var studentID, name, subjectID, subject string
		var average sql.NullFloat64
		var count int
		if err := rows.Scan(&studentID, &name, &subjectID, &subject, &average, &count); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read class summary")
			return
		}
		var averageValue any
		if average.Valid {
			averageValue = average.Float64
		}
		result = append(result, map[string]any{"student_id": studentID, "student": name, "subject_id": subjectID, "subject": subject, "average": averageValue, "count": count})
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (h *Handler) canTeach(ctx context.Context, p identity.Principal, classID, subjectID, termID string) bool {
	return h.canTeachTx(ctx, h.db, p, classID, subjectID, termID)
}

func (h *Handler) canTeachTx(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, p identity.Principal, classID, subjectID, termID string) bool {
	if identity.IsAdmin(p) {
		return true
	}
	var allowed bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM teacher_assignments ta JOIN terms t ON t.id=$4 AND t.academic_year_id=ta.academic_year_id JOIN classes c ON c.id=ta.class_id AND c.school_id=$5 AND c.academic_year_id=ta.academic_year_id JOIN subjects su ON su.id=ta.subject_id AND su.school_id=$5 WHERE ta.teacher_id=$1 AND ta.class_id=$2 AND ta.subject_id=$3 AND ta.school_id=$5)`, p.UserID, classID, subjectID, termID, p.SchoolID).Scan(&allowed)
	return err == nil && allowed
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate key")
}

func validWorkDate(value string) bool {
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}
