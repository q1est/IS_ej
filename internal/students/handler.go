package students

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"school-journal/internal/identity"
	"school-journal/internal/platform/httpx"
)

type Handler struct{ db *sql.DB }

func New(db *sql.DB) *Handler { return &Handler{db: db} }

func (h *Handler) Register(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("GET /students", protect(http.HandlerFunc(h.list)))
	mux.Handle("GET /students/{id}", protect(http.HandlerFunc(h.get)))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	limit, offset, err := pagination(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_pagination", "Limit must be 1..500 and offset must be non-negative")
		return
	}
	classID := strings.TrimSpace(r.URL.Query().Get("class_id"))
	if identity.HasRole(p, "student") {
		rows, err := h.db.QueryContext(r.Context(), `SELECT id::text,external_id,full_name FROM students
			WHERE school_id=$1 AND user_id=$2 ORDER BY full_name LIMIT $3 OFFSET $4`, p.SchoolID, p.UserID, limit, offset)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not list students")
			return
		}
		writeStudents(w, rows)
		return
	}
	if !identity.IsAdmin(p) && !identity.HasRole(p, "teacher") && !identity.HasRole(p, "class_teacher") {
		httpx.Error(w, http.StatusForbidden, "forbidden", "Insufficient role")
		return
	}
	query := `SELECT DISTINCT s.id::text,s.external_id,s.full_name FROM students s
		JOIN enrollments e ON e.student_id=s.id AND e.status='active' JOIN classes c ON c.id=e.class_id
		WHERE s.school_id=$1 AND c.school_id=$1`
	args := []any{p.SchoolID}
	if classID != "" {
		args = append(args, classID)
		query += " AND c.id=$" + strconv.Itoa(len(args))
	}
	if !identity.IsAdmin(p) {
		args = append(args, p.UserID)
		arg := strconv.Itoa(len(args))
		query += " AND (EXISTS(SELECT 1 FROM teacher_assignments ta WHERE ta.teacher_id=$" + arg + " AND ta.class_id=c.id) OR EXISTS(SELECT 1 FROM class_teacher_assignments ca WHERE ca.teacher_id=$" + arg + " AND ca.class_id=c.id))"
	}
	args = append(args, limit, offset)
	query += " ORDER BY s.full_name LIMIT $" + strconv.Itoa(len(args)-1) + " OFFSET $" + strconv.Itoa(len(args))
	rows, err := h.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not list students")
		return
	}
	writeStudents(w, rows)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	id := r.PathValue("id")
	if identity.HasRole(p, "student") {
		if p.StudentID == "" || id != p.StudentID {
			httpx.Error(w, http.StatusNotFound, "not_found", "Student not found")
			return
		}
	} else if !identity.IsAdmin(p) {
		if !identity.HasRole(p, "teacher") && !identity.HasRole(p, "class_teacher") {
			httpx.Error(w, http.StatusNotFound, "not_found", "Student not found")
			return
		}
		var allowed bool
		err := h.db.QueryRowContext(r.Context(), `SELECT EXISTS(
			SELECT 1 FROM enrollments e JOIN classes c ON c.id=e.class_id WHERE e.student_id=$1 AND c.school_id=$2 AND
			(EXISTS(SELECT 1 FROM teacher_assignments ta WHERE ta.teacher_id=$3 AND ta.class_id=c.id) OR
			 EXISTS(SELECT 1 FROM class_teacher_assignments ca WHERE ca.teacher_id=$3 AND ca.class_id=c.id)))`, id, p.SchoolID, p.UserID).Scan(&allowed)
		if err != nil || !allowed {
			httpx.Error(w, http.StatusNotFound, "not_found", "Student not found")
			return
		}
	}
	var student struct {
		ID         string `json:"id"`
		ExternalID string `json:"external_id"`
		FullName   string `json:"full_name"`
	}
	err := h.db.QueryRowContext(r.Context(), `SELECT id::text,external_id,full_name FROM students WHERE id=$1 AND school_id=$2`, id, p.SchoolID).
		Scan(&student.ID, &student.ExternalID, &student.FullName)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Student not found")
		return
	}
	httpx.JSON(w, http.StatusOK, student)
}

func writeStudents(w http.ResponseWriter, rows *sql.Rows) {
	defer rows.Close()
	students := make([]map[string]string, 0)
	for rows.Next() {
		var id, externalID, fullName string
		if err := rows.Scan(&id, &externalID, &fullName); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read students")
			return
		}
		students = append(students, map[string]string{"id": id, "external_id": externalID, "full_name": fullName})
	}
	if err := rows.Err(); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read students")
		return
	}
	httpx.JSON(w, http.StatusOK, students)
}

func pagination(r *http.Request) (int, int, error) {
	limit, offset := 100, 0
	var err error
	if value := r.URL.Query().Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 500 {
			return 0, 0, errInvalidPagination
		}
	}
	if value := r.URL.Query().Get("offset"); value != "" {
		offset, err = strconv.Atoi(value)
		if err != nil || offset < 0 {
			return 0, 0, errInvalidPagination
		}
	}
	return limit, offset, nil
}

var errInvalidPagination = strconv.ErrSyntax
