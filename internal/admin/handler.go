package admin

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	mux.Handle("POST /invites", protect(http.HandlerFunc(h.createInvite)))
	mux.Handle("GET /admin/users", protect(http.HandlerFunc(h.listUsers)))
	mux.Handle("GET /admin/invites", protect(http.HandlerFunc(h.listInvites)))
	mux.Handle("POST /admin/invites/{id}/revoke", protect(http.HandlerFunc(h.revokeInvite)))
	mux.Handle("POST /admin/users/{id}/revoke-sessions", protect(http.HandlerFunc(h.revokeSessions)))
	mux.Handle("GET /admin/audit", protect(http.HandlerFunc(h.auditLog)))
	mux.Handle("GET /admin/excel", protect(http.HandlerFunc(h.exportExcel)))
	mux.Handle("POST /admin/excel", protect(http.HandlerFunc(h.importExcel)))
	resources := []string{"classes", "subjects", "academic-years", "terms", "teacher-assignments", "class-teacher-assignments", "behavior-criteria", "behavior-permissions", "students", "enrollments", "projects", "student-projects"}
	for _, resource := range resources {
		mux.Handle("POST /admin/"+resource, protect(h.createResource(resource)))
		mux.Handle("GET /admin/"+resource, protect(h.listResource(resource)))
	}
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok || !identity.IsAdmin(p) {
		httpx.Error(w, http.StatusForbidden, "forbidden", "Administrator access required")
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `SELECT u.id::text,u.email::text,u.full_name,u.status,
		COALESCE(string_agg(DISTINCT roles.code,', ' ORDER BY roles.code),'')
		FROM users u LEFT JOIN user_roles ur ON ur.user_id=u.id AND ur.school_id=u.school_id
		LEFT JOIN roles ON roles.id=ur.role_id WHERE u.school_id=$1
		GROUP BY u.id ORDER BY u.full_name LIMIT 500`, p.SchoolID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not list users")
		return
	}
	defer rows.Close()
	result := make([]map[string]string, 0)
	for rows.Next() {
		var id, email, fullName, status, roles string
		if err := rows.Scan(&id, &email, &fullName, &status, &roles); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read users")
			return
		}
		result = append(result, map[string]string{"id": id, "email": email, "full_name": fullName, "status": status, "roles": roles})
	}
	if err := rows.Err(); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read users")
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (h *Handler) listInvites(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok || !identity.IsAdmin(p) {
		httpx.Error(w, http.StatusForbidden, "forbidden", "Administrator access required")
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `SELECT i.id::text,i.email::text,roles.code,i.target_type,
		COALESCE(i.target_id::text,''),i.expires_at,i.used_at
		FROM invites i JOIN roles ON roles.id=i.role_id WHERE i.school_id=$1
		ORDER BY i.created_at DESC LIMIT 500`, p.SchoolID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not list invitations")
		return
	}
	defer rows.Close()
	result := make([]map[string]any, 0)
	for rows.Next() {
		var id, email, role, targetType, targetID string
		var expiresAt time.Time
		var usedAt sql.NullTime
		if err := rows.Scan(&id, &email, &role, &targetType, &targetID, &expiresAt, &usedAt); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read invitations")
			return
		}
		var usedAtValue any
		if usedAt.Valid {
			usedAtValue = usedAt.Time
		}
		result = append(result, map[string]any{"id": id, "email": email, "role": role, "target_type": targetType, "target_id": targetID, "expires_at": expiresAt, "used_at": usedAtValue})
	}
	if err := rows.Err(); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read invitations")
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (h *Handler) revokeInvite(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok || !identity.IsAdmin(p) {
		httpx.Error(w, http.StatusForbidden, "forbidden", "Administrator access required")
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not revoke invitation")
		return
	}
	defer tx.Rollback()
	id := r.PathValue("id")
	var inviteID string
	err = tx.QueryRowContext(r.Context(), `UPDATE invites SET expires_at=LEAST(expires_at,now())
		WHERE id=$1 AND school_id=$2 AND used_at IS NULL RETURNING id::text`, id, p.SchoolID).Scan(&inviteID)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Invitation not found or already used")
		return
	}
	if err := audit.Write(r.Context(), tx, r, p, "revoke", "invites", inviteID, nil, map[string]string{"status": "revoked"}); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "audit_error", "Could not record invitation revocation")
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not revoke invitation")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) revokeSessions(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok || !identity.IsAdmin(p) {
		httpx.Error(w, http.StatusForbidden, "forbidden", "Administrator access required")
		return
	}
	userID := r.PathValue("id")
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not revoke sessions")
		return
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND school_id=$2)`, userID, p.SchoolID).Scan(&exists); err != nil || !exists {
		httpx.Error(w, http.StatusNotFound, "not_found", "User not found")
		return
	}
	result, err := tx.ExecContext(r.Context(), `UPDATE refresh_sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, userID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not revoke sessions")
		return
	}
	count, err := result.RowsAffected()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not count revoked sessions")
		return
	}
	if err := audit.Write(r.Context(), tx, r, p, "revoke_sessions", "users", userID, nil, map[string]int64{"revoked_sessions": count}); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "audit_error", "Could not record session revocation")
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not revoke sessions")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]int64{"revoked_sessions": count})
}

func (h *Handler) createInvite(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok || !identity.IsAdmin(p) {
		httpx.Error(w, http.StatusForbidden, "forbidden", "Administrator access required")
		return
	}
	var input struct {
		Email          string `json:"email"`
		Role           string `json:"role"`
		TargetType     string `json:"target_type"`
		TargetID       string `json:"target_id"`
		AcademicYearID string `json:"academic_year_id"`
	}
	if httpx.Decode(w, r, &input) != nil || strings.TrimSpace(input.Email) == "" || input.Role == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "email and role are required")
		return
	}
	if input.Role != "student" && input.Role != "teacher" && input.Role != "class_teacher" && input.Role != "admin" {
		httpx.Error(w, http.StatusBadRequest, "invalid_role", "Invalid role")
		return
	}
	if input.Role == "student" && (input.TargetType != "student" || input.TargetID == "") {
		httpx.Error(w, http.StatusBadRequest, "invalid_target", "Student invitations require a student target")
		return
	}
	if input.Role != "student" && !validInviteTarget(input.Role, input.TargetType) {
		httpx.Error(w, http.StatusBadRequest, "invalid_target", "Invite target type must match the role")
		return
	}
	if input.TargetType == "class" && input.TargetID == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_target", "Class-targeted invitations require a class_id")
		return
	}
	token, err := randomToken(32)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create invitation")
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create invitation")
		return
	}
	defer tx.Rollback()
	var inviteID string
	err = tx.QueryRowContext(r.Context(), `INSERT INTO invites(school_id,email,role_id,target_type,target_id,academic_year_id,token_hash,expires_at,created_by)
		SELECT $1,$2,r.id,$3,NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,$6,$7,$8 FROM roles r WHERE r.code=$9 RETURNING id::text`,
		p.SchoolID, strings.TrimSpace(input.Email), input.TargetType, input.TargetID, input.AcademicYearID, tokenHash(token), time.Now().Add(7*24*time.Hour), p.UserID, input.Role).Scan(&inviteID)
	if err == nil && input.Role == "student" {
		var valid bool
		err = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM students WHERE id=$1 AND school_id=$2 AND user_id IS NULL)`, input.TargetID, p.SchoolID).Scan(&valid)
		if err == nil && !valid {
			err = sql.ErrNoRows
		}
	}
	if err == nil && input.TargetType == "class" {
		var valid bool
		err = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM classes c WHERE c.id=$1 AND c.school_id=$2
			AND ($3='' OR c.academic_year_id::text=$3))`, input.TargetID, p.SchoolID, input.AcademicYearID).Scan(&valid)
		if err == nil && !valid {
			err = sql.ErrNoRows
		}
	}
	if err == nil {
		err = audit.Write(r.Context(), tx, r, p, "create", "invites", inviteID, nil, map[string]any{"email": input.Email, "role": input.Role})
	}
	if err != nil || tx.Commit() != nil {
		httpx.Error(w, http.StatusConflict, "conflict", "Could not create invitation; verify target and references")
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]string{"id": inviteID, "token": token, "warning": "Deliver this one-time token to the intended user using a private channel."})
}

func (h *Handler) createResource(resource string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := identity.FromContext(r.Context())
		if !ok || !identity.IsAdmin(p) {
			httpx.Error(w, http.StatusForbidden, "forbidden", "Administrator access required")
			return
		}
		var input map[string]json.RawMessage
		if httpx.Decode(w, r, &input) != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid_input", "Invalid JSON body")
			return
		}
		query, args, err := resourceInsert(resource, input, p)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid_input", err.Error())
			return
		}
		tx, err := h.db.BeginTx(r.Context(), nil)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create resource")
			return
		}
		defer tx.Rollback()
		var id string
		err = tx.QueryRowContext(r.Context(), query, args...).Scan(&id)
		if err == nil && resource == "students" {
			var classID string
			_ = json.Unmarshal(input["class_id"], &classID)
			classID = strings.TrimSpace(classID)
			if classID != "" {
				var enrollmentID string
				err = tx.QueryRowContext(r.Context(), `INSERT INTO enrollments(student_id,class_id,academic_year_id,status,school_id)
					SELECT $1,c.id,c.academic_year_id,'active',c.school_id FROM classes c
					WHERE c.id=$2 AND c.school_id=$3 RETURNING id::text`, id, classID, p.SchoolID).Scan(&enrollmentID)
			}
		}
		if err == nil {
			newValue := map[string]any{"id": id}
			if resource == "students" {
				var classID string
				_ = json.Unmarshal(input["class_id"], &classID)
				if classID = strings.TrimSpace(classID); classID != "" {
					newValue["class_id"] = classID
				}
			}
			err = audit.Write(r.Context(), tx, r, p, "create", resource, id, nil, newValue)
		}
		if err != nil || tx.Commit() != nil {
			httpx.Error(w, http.StatusConflict, "conflict", "Could not create resource; check fields, school scope, and uniqueness")
			return
		}
		httpx.JSON(w, http.StatusCreated, map[string]string{"id": id})
	})
}

func resourceInsert(resource string, body map[string]json.RawMessage, p identity.Principal) (string, []any, error) {
	get := func(key string) string {
		var value string
		_ = json.Unmarshal(body[key], &value)
		return strings.TrimSpace(value)
	}
	getInt := func(key string) int { var value int; _ = json.Unmarshal(body[key], &value); return value }
	getBool := func(key string) bool { var value bool; _ = json.Unmarshal(body[key], &value); return value }
	missing := func(keys ...string) error {
		for _, key := range keys {
			if get(key) == "" {
				return httpError("%s is required", key)
			}
		}
		return nil
	}
	switch resource {
	case "classes":
		if err := missing("name", "academic_year_id"); err != nil {
			return "", nil, err
		}
		return `INSERT INTO classes(school_id,name,academic_year_id) SELECT $1,$2,id FROM academic_years WHERE id=$3 AND school_id=$1 RETURNING id::text`, []any{p.SchoolID, get("name"), get("academic_year_id")}, nil
	case "subjects":
		if err := missing("name"); err != nil {
			return "", nil, err
		}
		return `INSERT INTO subjects(school_id,name) VALUES($1,$2) RETURNING id::text`, []any{p.SchoolID, get("name")}, nil
	case "academic-years":
		if err := missing("name", "starts_on", "ends_on"); err != nil {
			return "", nil, err
		}
		return `INSERT INTO academic_years(school_id,name,starts_on,ends_on,is_current) VALUES($1,$2,$3,$4,$5) RETURNING id::text`, []any{p.SchoolID, get("name"), get("starts_on"), get("ends_on"), getBool("is_current")}, nil
	case "terms":
		if err := missing("name", "starts_on", "ends_on", "academic_year_id"); err != nil {
			return "", nil, err
		}
		return `INSERT INTO terms(academic_year_id,name,starts_on,ends_on,school_id) SELECT id,$2,$3,$4,school_id FROM academic_years WHERE id=$1 AND school_id=$5 RETURNING id::text`, []any{get("academic_year_id"), get("name"), get("starts_on"), get("ends_on"), p.SchoolID}, nil
	case "teacher-assignments":
		if err := missing("teacher_id", "class_id", "subject_id", "academic_year_id"); err != nil {
			return "", nil, err
		}
		return `INSERT INTO teacher_assignments(teacher_id,class_id,subject_id,academic_year_id,school_id)
			SELECT u.id,c.id,s.id,y.id,u.school_id FROM users u JOIN classes c ON c.id=$2 AND c.school_id=u.school_id
			JOIN subjects s ON s.id=$3 AND s.school_id=u.school_id JOIN academic_years y ON y.id=$4 AND y.school_id=u.school_id AND y.id=c.academic_year_id
			JOIN user_roles ur ON ur.user_id=u.id AND ur.school_id=u.school_id JOIN roles r ON r.id=ur.role_id AND r.code='teacher'
			WHERE u.id=$1 RETURNING id::text`, []any{get("teacher_id"), get("class_id"), get("subject_id"), get("academic_year_id")}, nil
	case "class-teacher-assignments":
		if err := missing("teacher_id", "class_id", "academic_year_id"); err != nil {
			return "", nil, err
		}
		return `INSERT INTO class_teacher_assignments(teacher_id,class_id,academic_year_id,school_id)
			SELECT u.id,c.id,y.id,u.school_id FROM users u JOIN classes c ON c.id=$2 AND c.school_id=u.school_id
			JOIN academic_years y ON y.id=$3 AND y.school_id=u.school_id AND y.id=c.academic_year_id
			JOIN user_roles ur ON ur.user_id=u.id AND ur.school_id=u.school_id JOIN roles r ON r.id=ur.role_id AND r.code='class_teacher'
			WHERE u.id=$1 RETURNING id::text`, []any{get("teacher_id"), get("class_id"), get("academic_year_id")}, nil
	case "behavior-criteria":
		if err := missing("code", "title"); err != nil {
			return "", nil, err
		}
		max, order := getInt("max_score"), getInt("sort_order")
		if max < 0 || max > 3 || order < 1 || order > 10 {
			return "", nil, httpError("max_score must be 0..3 and sort_order must be 1..10")
		}
		return `INSERT INTO behavior_criteria(school_id,code,title,description,max_score,sort_order) VALUES($1,$2,$3,NULLIF($4,''),$5,$6) RETURNING id::text`, []any{p.SchoolID, get("code"), get("title"), get("description"), max, order}, nil
	case "behavior-permissions":
		if err := missing("teacher_id", "class_id", "academic_year_id"); err != nil {
			return "", nil, err
		}
		return `INSERT INTO behavior_permissions(teacher_id,class_id,academic_year_id,can_score,school_id)
			SELECT u.id,c.id,y.id,$4,u.school_id FROM users u JOIN classes c ON c.id=$2 AND c.school_id=u.school_id
			JOIN academic_years y ON y.id=$3 AND y.school_id=u.school_id AND y.id=c.academic_year_id
			JOIN user_roles ur ON ur.user_id=u.id AND ur.school_id=u.school_id JOIN roles r ON r.id=ur.role_id AND r.code='teacher'
			WHERE u.id=$1 RETURNING id::text`, []any{get("teacher_id"), get("class_id"), get("academic_year_id"), getBool("can_score")}, nil
	case "students":
		if err := missing("external_id", "full_name"); err != nil {
			return "", nil, err
		}
		return `INSERT INTO students(school_id,external_id,full_name,birth_date) VALUES($1,$2,$3,NULLIF($4,'')::date) RETURNING id::text`, []any{p.SchoolID, get("external_id"), get("full_name"), get("birth_date")}, nil
	case "enrollments":
		if err := missing("student_id", "class_id", "academic_year_id"); err != nil {
			return "", nil, err
		}
		return `INSERT INTO enrollments(student_id,class_id,academic_year_id,status,from_date,school_id)
			SELECT s.id,c.id,y.id,'active',NULLIF($4,'')::date,s.school_id FROM students s JOIN classes c ON c.id=$2 AND c.school_id=s.school_id
			JOIN academic_years y ON y.id=$3 AND y.school_id=s.school_id AND y.id=c.academic_year_id
			WHERE s.id=$1 AND s.school_id=$5 RETURNING id::text`, []any{get("student_id"), get("class_id"), get("academic_year_id"), get("from_date"), p.SchoolID}, nil
	case "projects":
		if err := missing("name"); err != nil {
			return "", nil, err
		}
		return `INSERT INTO projects(school_id,name) VALUES($1,$2) RETURNING id::text`, []any{p.SchoolID, get("name")}, nil
	case "student-projects":
		if err := missing("student_id", "project_id", "academic_year_id"); err != nil {
			return "", nil, err
		}
		return `INSERT INTO student_projects(student_id,project_id,academic_year_id,school_id)
			SELECT s.id,p.id,y.id,s.school_id FROM students s JOIN projects p ON p.id=$2 AND p.school_id=s.school_id
			JOIN academic_years y ON y.id=$3 AND y.school_id=s.school_id WHERE s.id=$1 AND s.school_id=$4 RETURNING student_id::text`, []any{get("student_id"), get("project_id"), get("academic_year_id"), p.SchoolID}, nil
	default:
		return "", nil, httpError("Unknown admin resource")
	}
}

func (h *Handler) listResource(resource string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := identity.FromContext(r.Context())
		if !ok || !identity.IsAdmin(p) {
			httpx.Error(w, http.StatusForbidden, "forbidden", "Administrator access required")
			return
		}
		query := resourceQuery(resource)
		if query == "" {
			httpx.Error(w, http.StatusNotFound, "not_found", "Unknown admin resource")
			return
		}
		rows, err := h.db.QueryContext(r.Context(), query, p.SchoolID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not list admin resource")
			return
		}
		defer rows.Close()
		columns, err := rows.Columns()
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not list admin resource")
			return
		}
		result := make([]map[string]any, 0)
		for rows.Next() {
			values := make([]any, len(columns))
			destinations := make([]any, len(columns))
			for i := range values {
				destinations[i] = &values[i]
			}
			if err := rows.Scan(destinations...); err != nil {
				httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read admin resource")
				return
			}
			item := make(map[string]any, len(columns))
			for i, key := range columns {
				if bytes, ok := values[i].([]byte); ok {
					item[key] = string(bytes)
				} else {
					item[key] = values[i]
				}
			}
			result = append(result, item)
		}
		if err := rows.Err(); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read admin resource")
			return
		}
		httpx.JSON(w, http.StatusOK, result)
	})
}

func resourceQuery(resource string) string {
	queries := map[string]string{
		"classes":                   `SELECT id::text,name,academic_year_id::text FROM classes WHERE school_id=$1 ORDER BY name LIMIT 500`,
		"subjects":                  `SELECT id::text,name FROM subjects WHERE school_id=$1 ORDER BY name LIMIT 500`,
		"academic-years":            `SELECT id::text,name,starts_on::text,ends_on::text,is_current FROM academic_years WHERE school_id=$1 ORDER BY starts_on DESC LIMIT 100`,
		"terms":                     `SELECT t.id::text,t.name,t.starts_on::text,t.ends_on::text,t.academic_year_id::text FROM terms t JOIN academic_years y ON y.id=t.academic_year_id WHERE y.school_id=$1 ORDER BY t.starts_on LIMIT 100`,
		"teacher-assignments":       `SELECT ta.id::text,ta.teacher_id::text,ta.class_id::text,ta.subject_id::text,ta.academic_year_id::text FROM teacher_assignments ta JOIN classes c ON c.id=ta.class_id WHERE c.school_id=$1 LIMIT 500`,
		"class-teacher-assignments": `SELECT ca.id::text,ca.teacher_id::text,ca.class_id::text,ca.academic_year_id::text FROM class_teacher_assignments ca JOIN classes c ON c.id=ca.class_id WHERE c.school_id=$1 LIMIT 500`,
		"behavior-criteria":         `SELECT id::text,code,title,max_score,sort_order FROM behavior_criteria WHERE school_id=$1 ORDER BY sort_order`,
		"behavior-permissions":      `SELECT bp.id::text,bp.teacher_id::text,bp.class_id::text,bp.academic_year_id::text,bp.can_score FROM behavior_permissions bp JOIN classes c ON c.id=bp.class_id WHERE c.school_id=$1 LIMIT 500`,
		"students":                  `SELECT id::text,external_id,full_name,birth_date::text FROM students WHERE school_id=$1 ORDER BY full_name LIMIT 500`,
		"enrollments":               `SELECT e.id::text,e.student_id::text,e.class_id::text,e.academic_year_id::text,e.status FROM enrollments e JOIN students s ON s.id=e.student_id WHERE s.school_id=$1 ORDER BY e.created_at DESC LIMIT 500`,
		"projects":                  `SELECT id::text,name FROM projects WHERE school_id=$1 ORDER BY name LIMIT 500`,
		"student-projects":          `SELECT sp.student_id::text,sp.project_id::text,sp.academic_year_id::text FROM student_projects sp JOIN students s ON s.id=sp.student_id WHERE s.school_id=$1 LIMIT 500`,
	}
	return queries[resource]
}

func (h *Handler) auditLog(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok || !identity.IsAdmin(p) {
		httpx.Error(w, http.StatusForbidden, "forbidden", "Administrator access required")
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `SELECT a.id,a.actor_user_id::text,a.action,a.entity,a.entity_id,a.old_value,a.new_value,a.ip::text,a.user_agent,a.created_at
		FROM audit_log a JOIN users u ON u.id=a.actor_user_id WHERE u.school_id=$1 ORDER BY a.created_at DESC LIMIT 200`, p.SchoolID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read audit log")
		return
	}
	defer rows.Close()
	result := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var actor, ip, userAgent sql.NullString
		var action, entity, entityID string
		var oldValue, newValue []byte
		var created time.Time
		if err := rows.Scan(&id, &actor, &action, &entity, &entityID, &oldValue, &newValue, &ip, &userAgent, &created); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read audit log")
			return
		}
		var oldDecoded, newDecoded any
		if len(oldValue) > 0 {
			_ = json.Unmarshal(oldValue, &oldDecoded)
		}
		if len(newValue) > 0 {
			_ = json.Unmarshal(newValue, &newDecoded)
		}
		result = append(result, map[string]any{"id": id, "actor_user_id": nullable(actor), "action": action, "entity": entity, "entity_id": entityID, "old_value": oldDecoded, "new_value": newDecoded, "ip": nullable(ip), "user_agent": nullable(userAgent), "created_at": created})
	}
	httpx.JSON(w, http.StatusOK, result)
}

func nullable(value sql.NullString) any {
	if value.Valid {
		return value.String
	}
	return nil
}
func randomToken(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
func tokenHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
func httpError(format string, args ...any) error { return fmt.Errorf(format, args...) }

func validInviteTarget(role, targetType string) bool {
	if targetType == "" {
		return role == "admin" || role == "teacher" || role == "class_teacher"
	}
	switch role {
	case "teacher":
		return targetType == "teacher" || targetType == "class"
	case "class_teacher":
		return targetType == "class_teacher" || targetType == "class"
	default:
		return false
	}
}
