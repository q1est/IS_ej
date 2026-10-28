package behavior

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"school-journal/internal/audit"
	"school-journal/internal/identity"
	"school-journal/internal/platform/httpx"
)

type Handler struct{ db *sql.DB }

func New(db *sql.DB) *Handler { return &Handler{db: db} }

func (h *Handler) Register(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("GET /behavior/criteria", protect(http.HandlerFunc(h.criteria)))
	mux.Handle("GET /behavior/contexts", protect(http.HandlerFunc(h.contexts)))
	mux.Handle("GET /behavior/sheets", protect(http.HandlerFunc(h.list)))
	mux.Handle("POST /behavior/sheets", protect(http.HandlerFunc(h.create)))
	mux.Handle("POST /behavior/sheets/generate", protect(http.HandlerFunc(h.generate)))
	mux.Handle("GET /behavior/sheets/{id}", protect(http.HandlerFunc(h.get)))
	mux.Handle("PATCH /behavior/sheets/{id}/scores", protect(http.HandlerFunc(h.updateScores)))
	mux.Handle("GET /behavior/sheets/{id}/export.csv", protect(http.HandlerFunc(h.exportCSV)))
}

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
		query = `SELECT c.id::text,c.name,t.id::text,t.name,c.academic_year_id::text,true
			FROM classes c JOIN terms t ON t.academic_year_id=c.academic_year_id
			WHERE c.school_id=$1 ORDER BY c.name,t.starts_on LIMIT 1000`
		args = []any{p.SchoolID}
	case identity.HasRole(p, "student"):
		query = `SELECT c.id::text,c.name,t.id::text,t.name,c.academic_year_id::text,false
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
		query = `SELECT DISTINCT c.id::text,c.name,t.id::text,t.name,c.academic_year_id::text,
			($3 OR ($4 AND EXISTS(SELECT 1 FROM behavior_permissions bp WHERE bp.teacher_id=$1 AND bp.class_id=c.id AND bp.academic_year_id=c.academic_year_id AND bp.school_id=$2 AND bp.can_score)))
			FROM classes c JOIN terms t ON t.academic_year_id=c.academic_year_id
			WHERE c.school_id=$2 AND (
				EXISTS(SELECT 1 FROM class_teacher_assignments ca WHERE ca.teacher_id=$1 AND ca.class_id=c.id AND ca.academic_year_id=c.academic_year_id AND ca.school_id=$2)
				OR ($4 AND EXISTS(SELECT 1 FROM behavior_permissions bp WHERE bp.teacher_id=$1 AND bp.class_id=c.id AND bp.academic_year_id=c.academic_year_id AND bp.school_id=$2 AND bp.can_score)))
			ORDER BY 2,4 LIMIT 1000`
		args = []any{p.UserID, p.SchoolID, identity.IsAdmin(p), identity.HasRole(p, "teacher")}
	}
	rows, err := h.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not list behavior contexts")
		return
	}
	defer rows.Close()
	result := make([]map[string]any, 0)
	for rows.Next() {
		var classID, className, termID, termName, yearID string
		var canEdit bool
		if err := rows.Scan(&classID, &className, &termID, &termName, &yearID, &canEdit); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read behavior contexts")
			return
		}
		result = append(result, map[string]any{"class_id": classID, "class_name": className, "term_id": termID, "term_name": termName, "academic_year_id": yearID, "can_edit": canEdit})
	}
	if err := rows.Err(); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read behavior contexts")
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (h *Handler) generate(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	if !identity.IsAdmin(p) {
		httpx.Error(w, http.StatusForbidden, "forbidden", "Administrator access required")
		return
	}
	var input struct {
		ClassID string `json:"class_id"`
		YearID  string `json:"academic_year_id"`
		TermID  string `json:"term_id"`
	}
	if httpx.Decode(w, r, &input) != nil || input.ClassID == "" || input.YearID == "" || input.TermID == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "class_id, academic_year_id, and term_id are required")
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create behavior sheets")
		return
	}
	defer tx.Rollback()
	var valid bool
	err = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM classes c JOIN academic_years y ON y.id=c.academic_year_id
		JOIN terms t ON t.academic_year_id=y.id WHERE c.id=$1 AND c.school_id=$2 AND y.id=$3 AND t.id=$4)`,
		input.ClassID, p.SchoolID, input.YearID, input.TermID).Scan(&valid)
	if err != nil || !valid {
		httpx.Error(w, http.StatusNotFound, "not_found", "Class, year, or term is not available")
		return
	}
	var criterionCount int
	if err := tx.QueryRowContext(r.Context(), `SELECT count(*) FROM behavior_criteria WHERE school_id=$1`, p.SchoolID).Scan(&criterionCount); err != nil || criterionCount != 10 {
		httpx.Error(w, http.StatusConflict, "criteria_incomplete", "Exactly 10 behavior criteria must be configured before creating sheets")
		return
	}
	rows, err := tx.QueryContext(r.Context(), `INSERT INTO behavior_sheets(class_id,student_id,academic_year_id,term_id,total_score,is_complete,verdict,updated_by,school_id)
		SELECT c.id,e.student_id,y.id,t.id,0,false,NULL,$4,c.school_id FROM enrollments e
		JOIN students s ON s.id=e.student_id AND s.school_id=e.school_id
		JOIN classes c ON c.id=e.class_id AND c.school_id=e.school_id
		JOIN academic_years y ON y.id=e.academic_year_id AND y.school_id=e.school_id
		JOIN terms t ON t.id=$3 AND t.academic_year_id=y.id
		WHERE e.class_id=$1 AND e.academic_year_id=$2 AND e.school_id=$5 AND e.status='active'
		ON CONFLICT(student_id,academic_year_id,term_id) DO NOTHING RETURNING id::text,student_id::text`,
		input.ClassID, input.YearID, input.TermID, p.UserID, p.SchoolID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not create behavior sheets")
		return
	}
	type createdSheet struct{ id, studentID string }
	createdSheets := make([]createdSheet, 0)
	for rows.Next() {
		var id, studentID string
		if err := rows.Scan(&id, &studentID); err != nil {
			_ = rows.Close()
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read created behavior sheets")
			return
		}
		createdSheets = append(createdSheets, createdSheet{id: id, studentID: studentID})
	}
	rowsErr := rows.Err()
	_ = rows.Close()
	if rowsErr != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read created behavior sheets")
		return
	}
	for _, sheet := range createdSheets {
		if err := audit.Write(r.Context(), tx, r, p, "create", "behavior_sheets", sheet.id, nil, map[string]string{
			"student_id": sheet.studentID, "class_id": input.ClassID, "academic_year_id": input.YearID, "term_id": input.TermID,
		}); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "audit_error", "Could not record behavior sheet creation")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create behavior sheets")
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]int{"created": len(createdSheets)})
}

func (h *Handler) criteria(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `SELECT id::text,code,title,max_score,sort_order FROM behavior_criteria WHERE school_id=$1 ORDER BY sort_order`, p.SchoolID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not list criteria")
		return
	}
	defer rows.Close()
	result := make([]map[string]any, 0)
	for rows.Next() {
		var id, code, title string
		var maxScore, order int
		if err := rows.Scan(&id, &code, &title, &maxScore, &order); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read criteria")
			return
		}
		result = append(result, map[string]any{"id": id, "code": code, "title": title, "max_score": maxScore, "sort_order": order})
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	classID, termID := strings.TrimSpace(r.URL.Query().Get("class_id")), strings.TrimSpace(r.URL.Query().Get("term_id"))
	if classID != "" && !identity.IsAdmin(p) && !identity.HasRole(p, "student") && !h.canAccessContext(r.Context(), p, classID, termID) {
		httpx.Error(w, http.StatusForbidden, "forbidden", "No access to this class and period")
		return
	}
	query := `SELECT bs.id::text,bs.student_id::text,s.full_name,bs.class_id::text,c.name,bs.term_id::text,t.name,bs.academic_year_id::text,bs.total_score,
		bs.is_complete,bs.verdict,(SELECT count(*) FROM behavior_scores sc WHERE sc.behavior_sheet_id=bs.id),COALESCE((SELECT jsonb_object_agg(sc.criterion_id::text,sc.score) FROM behavior_scores sc WHERE sc.behavior_sheet_id=bs.id),'{}'::jsonb),COALESCE((SELECT p.name FROM student_projects sp JOIN projects p ON p.id=sp.project_id
		WHERE sp.student_id=bs.student_id AND sp.academic_year_id=bs.academic_year_id AND p.school_id=$1 ORDER BY p.name LIMIT 1),'')
		FROM behavior_sheets bs JOIN students s ON s.id=bs.student_id JOIN classes c ON c.id=bs.class_id JOIN terms t ON t.id=bs.term_id
		WHERE bs.deleted_at IS NULL AND c.school_id=$1`
	args := []any{p.SchoolID}
	if classID != "" {
		args = append(args, classID)
		query += " AND bs.class_id=$" + strconv.Itoa(len(args))
	}
	if termID != "" {
		args = append(args, termID)
		query += " AND bs.term_id=$" + strconv.Itoa(len(args))
	}
	if identity.HasRole(p, "student") {
		if p.StudentID == "" {
			httpx.JSON(w, http.StatusOK, []any{})
			return
		}
		args = append(args, p.StudentID)
		query += " AND bs.student_id=$" + strconv.Itoa(len(args))
	} else if !identity.IsAdmin(p) {
		if !identity.HasRole(p, "teacher") && !identity.HasRole(p, "class_teacher") {
			httpx.Error(w, http.StatusForbidden, "forbidden", "Insufficient role")
			return
		}
		args = append(args, p.UserID)
		index := strconv.Itoa(len(args))
		query += ` AND (EXISTS(SELECT 1 FROM class_teacher_assignments ca WHERE ca.teacher_id=$` + index + ` AND ca.class_id=bs.class_id AND ca.academic_year_id=bs.academic_year_id)
			OR EXISTS(SELECT 1 FROM behavior_permissions bp WHERE bp.teacher_id=$` + index + ` AND bp.class_id=bs.class_id AND bp.academic_year_id=bs.academic_year_id AND bp.can_score))`
	}
	query += " ORDER BY s.full_name LIMIT 500"
	rows, err := h.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not list behavior sheets")
		return
	}
	defer rows.Close()
	result := make([]map[string]any, 0)
	for rows.Next() {
		var id, studentID, name, classID, className, sheetTerm, termName, yearID, project string
		var score, scoreCount int
		var complete bool
		var verdict sql.NullString
		var scoreJSON []byte
		if err := rows.Scan(&id, &studentID, &name, &classID, &className, &sheetTerm, &termName, &yearID, &score, &complete, &verdict, &scoreCount, &scoreJSON, &project); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read behavior sheets")
			return
		}
		var scoreValues map[string]int
		if err := json.Unmarshal(scoreJSON, &scoreValues); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read behavior scores")
			return
		}
		var verdictValue any
		if complete && verdict.Valid {
			verdictValue = verdict.String
		}
		result = append(result, map[string]any{"id": id, "student_id": studentID, "student_name": name, "class_id": classID, "class_name": className, "term_id": sheetTerm, "term_name": termName, "project": project, "total_score": score, "scores_count": scoreCount, "scores": scoreValues, "is_complete": complete, "verdict": verdictValue, "can_edit": h.canEdit(r.Context(), h.db, p, classID, yearID)})
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	if !identity.IsAdmin(p) {
		httpx.Error(w, http.StatusForbidden, "forbidden", "Administrator access required to create a sheet")
		return
	}
	var input struct {
		StudentID string `json:"student_id"`
		ClassID   string `json:"class_id"`
		YearID    string `json:"academic_year_id"`
		TermID    string `json:"term_id"`
	}
	if httpx.Decode(w, r, &input) != nil || input.StudentID == "" || input.ClassID == "" || input.YearID == "" || input.TermID == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "student_id, class_id, academic_year_id, and term_id are required")
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create behavior sheet")
		return
	}
	defer tx.Rollback()
	var valid bool
	err = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM students s JOIN classes c ON c.id=$2 AND c.school_id=s.school_id
		JOIN academic_years y ON y.id=$3 AND y.school_id=s.school_id AND y.id=c.academic_year_id
		JOIN terms t ON t.id=$4 AND t.academic_year_id=y.id
		JOIN enrollments e ON e.student_id=s.id AND e.class_id=c.id AND e.academic_year_id=y.id AND e.school_id=s.school_id AND e.status='active'
		WHERE s.id=$1 AND s.school_id=$5)`, input.StudentID, input.ClassID, input.YearID, input.TermID, p.SchoolID).Scan(&valid)
	if err != nil || !valid {
		httpx.Error(w, http.StatusNotFound, "not_found", "Student, class, year, or term is not available")
		return
	}
	var criterionCount int
	err = tx.QueryRowContext(r.Context(), `SELECT count(*) FROM behavior_criteria WHERE school_id=$1`, p.SchoolID).Scan(&criterionCount)
	if err != nil || criterionCount != 10 {
		httpx.Error(w, http.StatusConflict, "criteria_incomplete", "Exactly 10 behavior criteria must be configured before creating sheets")
		return
	}
	var id string
	err = tx.QueryRowContext(r.Context(), `INSERT INTO behavior_sheets(class_id,student_id,academic_year_id,term_id,total_score,is_complete,verdict,updated_by,school_id)
		VALUES($1,$2,$3,$4,0,false,NULL,$5,$6) RETURNING id::text`, input.ClassID, input.StudentID, input.YearID, input.TermID, p.UserID, p.SchoolID).Scan(&id)
	if err == nil {
		err = audit.Write(r.Context(), tx, r, p, "create", "behavior_sheets", id, nil, input)
	}
	if err != nil || tx.Commit() != nil {
		httpx.Error(w, http.StatusConflict, "conflict", "Could not create behavior sheet; it may already exist")
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	id := r.PathValue("id")
	var sheet struct {
		ClassID, StudentID, YearID, TermID string
		Score                              int
		Complete                           bool
		Verdict                            sql.NullString
	}
	var studentName string
	err := h.db.QueryRowContext(r.Context(), `SELECT bs.class_id::text,bs.student_id::text,bs.academic_year_id::text,bs.term_id::text,bs.total_score,bs.is_complete,bs.verdict,s.full_name
		FROM behavior_sheets bs JOIN classes c ON c.id=bs.class_id JOIN students s ON s.id=bs.student_id
		WHERE bs.id=$1 AND bs.deleted_at IS NULL AND c.school_id=$2`, id, p.SchoolID).
		Scan(&sheet.ClassID, &sheet.StudentID, &sheet.YearID, &sheet.TermID, &sheet.Score, &sheet.Complete, &sheet.Verdict, &studentName)
	if err != nil || !h.canView(r.Context(), p, sheet.ClassID, sheet.StudentID, sheet.YearID) {
		httpx.Error(w, http.StatusNotFound, "not_found", "Behavior sheet not found")
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `SELECT bc.id::text,bc.code,bc.title,bc.max_score,sc.score FROM behavior_criteria bc
		LEFT JOIN behavior_scores sc ON sc.behavior_sheet_id=$1 AND sc.criterion_id=bc.id WHERE bc.school_id=$2 ORDER BY bc.sort_order`, id, p.SchoolID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read behavior scores")
		return
	}
	defer rows.Close()
	scores := make([]map[string]any, 0)
	for rows.Next() {
		var criterionID, code, title string
		var maxScore int
		var score sql.NullInt64
		if err := rows.Scan(&criterionID, &code, &title, &maxScore, &score); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read behavior scores")
			return
		}
		var scoreValue any
		if score.Valid {
			scoreValue = score.Int64
		}
		scores = append(scores, map[string]any{"criterion_id": criterionID, "code": code, "title": title, "max_score": maxScore, "score": scoreValue})
	}
	var project string
	_ = h.db.QueryRowContext(r.Context(), `SELECT p.name FROM student_projects sp JOIN projects p ON p.id=sp.project_id WHERE sp.student_id=$1 AND sp.academic_year_id=$2 AND p.school_id=$3 ORDER BY p.name LIMIT 1`, sheet.StudentID, sheet.YearID, p.SchoolID).Scan(&project)
	var verdict any
	if sheet.Complete && sheet.Verdict.Valid {
		verdict = sheet.Verdict.String
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"id": id, "student_id": sheet.StudentID, "student_name": studentName, "class_id": sheet.ClassID, "term_id": sheet.TermID, "project": project, "total_score": sheet.Score, "is_complete": sheet.Complete, "verdict": verdict, "can_edit": h.canEdit(r.Context(), h.db, p, sheet.ClassID, sheet.YearID), "scores": scores})
}

func (h *Handler) updateScores(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	var input struct {
		Scores []struct {
			CriterionID string `json:"criterion_id"`
			Score       json.RawMessage `json:"score"`
		} `json:"scores"`
	}
	if httpx.Decode(w, r, &input) != nil || len(input.Scores) == 0 || len(input.Scores) > 10 {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "Provide between 1 and 10 scores")
		return
	}
	sheetID := r.PathValue("id")
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not update behavior scores")
		return
	}
	defer tx.Rollback()
	var classID, yearID string
	err = tx.QueryRowContext(r.Context(), `SELECT bs.class_id::text,bs.academic_year_id::text FROM behavior_sheets bs JOIN classes c ON c.id=bs.class_id
		WHERE bs.id=$1 AND bs.deleted_at IS NULL AND c.school_id=$2 FOR UPDATE OF bs`, sheetID, p.SchoolID).Scan(&classID, &yearID)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Behavior sheet not found")
		return
	}
	if !h.canEditTx(r.Context(), tx, p, classID, yearID) {
		httpx.Error(w, http.StatusForbidden, "forbidden", "No permission to score behavior")
		return
	}
	oldScores := map[string]int{}
	rows, err := tx.QueryContext(r.Context(), `SELECT criterion_id::text,score FROM behavior_scores WHERE behavior_sheet_id=$1`, sheetID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read previous scores")
		return
	}
	for rows.Next() {
		var id string
		var score int
		if rows.Scan(&id, &score) == nil {
			oldScores[id] = score
		}
	}
	rowsErr := rows.Err()
	_ = rows.Close()
	if rowsErr != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read previous scores")
		return
	}
	newScores := make(map[string]int, len(input.Scores))
	for criterionID, score := range oldScores {
		newScores[criterionID] = score
	}
	for _, item := range input.Scores {
		if item.CriterionID == "" || len(item.Score) == 0 {
			httpx.Error(w, http.StatusBadRequest, "invalid_score", "Each score must be an integer or null to clear it")
			return
		}
		var score *int
		if string(item.Score) != "null" {
			var value int
			if json.Unmarshal(item.Score, &value) != nil {
				httpx.Error(w, http.StatusBadRequest, "invalid_score", "Scores must be integers or null")
				return
			}
			score = &value
		}
		if score != nil && (*score < 0 || *score > 3) {
			httpx.Error(w, http.StatusBadRequest, "invalid_score", "Scores must be in the range 0..3 or null to clear a score")
			return
		}
		if _, exists := newScores[item.CriterionID]; exists {
			httpx.Error(w, http.StatusBadRequest, "duplicate_criterion", "A criterion may appear only once")
			return
		}
		var maxScore int
		err = tx.QueryRowContext(r.Context(), `SELECT bc.max_score FROM behavior_criteria bc JOIN classes c ON c.school_id=bc.school_id WHERE bc.id=$1 AND c.id=$2`, item.CriterionID, classID).Scan(&maxScore)
		if err != nil || (score != nil && *score > maxScore) {
			httpx.Error(w, http.StatusBadRequest, "invalid_criterion", "Criterion or score is invalid")
			return
		}
		if score == nil {
			_, err = tx.ExecContext(r.Context(), `DELETE FROM behavior_scores WHERE behavior_sheet_id=$1 AND criterion_id=$2`, sheetID, item.CriterionID)
			delete(newScores, item.CriterionID)
		} else {
			_, err = tx.ExecContext(r.Context(), `INSERT INTO behavior_scores(behavior_sheet_id,criterion_id,score,school_id) VALUES($1,$2,$3,$4)
				ON CONFLICT(behavior_sheet_id,criterion_id) DO UPDATE SET score=EXCLUDED.score`, sheetID, item.CriterionID, *score, p.SchoolID)
			newScores[item.CriterionID] = *score
		}
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not save behavior score")
			return
		}
	}
	var total, scoreCount, criterionCount int
	err = tx.QueryRowContext(r.Context(), `SELECT COALESCE(sum(sc.score),0),count(sc.id),count(bc.id) FROM behavior_criteria bc
		JOIN classes c ON c.school_id=bc.school_id LEFT JOIN behavior_scores sc ON sc.criterion_id=bc.id AND sc.behavior_sheet_id=$1
		WHERE c.id=$2`, sheetID, classID).Scan(&total, &scoreCount, &criterionCount)
	if err != nil || criterionCount != 10 {
		httpx.Error(w, http.StatusConflict, "criteria_incomplete", "Exactly 10 behavior criteria must be configured")
		return
	}
	complete := scoreCount == criterionCount
	var verdict any
	if complete {
		verdict = Verdict(total)
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE behavior_sheets SET total_score=$2,is_complete=$3,verdict=$4,updated_by=$5,updated_at=now() WHERE id=$1`, sheetID, total, complete, verdict, p.UserID)
	if err == nil {
		err = audit.Write(r.Context(), tx, r, p, "update", "behavior_sheets", sheetID, oldScores, newScores)
	}
	if err != nil || tx.Commit() != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not update behavior scores")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"total_score": total, "is_complete": complete, "verdict": verdict})
}

func (h *Handler) exportCSV(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	id := r.PathValue("id")
	var classID, studentID, yearID, studentName, project string
	var total int
	var complete bool
	var verdict sql.NullString
	err := h.db.QueryRowContext(r.Context(), `SELECT bs.class_id::text,bs.student_id::text,bs.academic_year_id::text,s.full_name,
		COALESCE((SELECT pr.name FROM student_projects sp JOIN projects pr ON pr.id=sp.project_id AND pr.school_id=sp.school_id
			WHERE sp.student_id=bs.student_id AND sp.academic_year_id=bs.academic_year_id AND sp.school_id=bs.school_id ORDER BY pr.name LIMIT 1),''),
		bs.total_score,bs.is_complete,bs.verdict
		FROM behavior_sheets bs JOIN classes c ON c.id=bs.class_id JOIN students s ON s.id=bs.student_id
		WHERE bs.id=$1 AND bs.deleted_at IS NULL AND c.school_id=$2`, id, p.SchoolID).
		Scan(&classID, &studentID, &yearID, &studentName, &project, &total, &complete, &verdict)
	if err != nil || !h.canView(r.Context(), p, classID, studentID, yearID) {
		httpx.Error(w, http.StatusNotFound, "not_found", "Behavior sheet not found")
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `SELECT bc.title,sc.score FROM behavior_criteria bc
		LEFT JOIN behavior_scores sc ON sc.criterion_id=bc.id AND sc.behavior_sheet_id=$1 WHERE bc.school_id=$2 ORDER BY bc.sort_order`, id, p.SchoolID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not export behavior sheet")
		return
	}
	defer rows.Close()
	criteria := make([]string, 0, 10)
	scores := make([]string, 0, 10)
	for rows.Next() {
		var title string
		var score sql.NullInt64
		if err := rows.Scan(&title, &score); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "db_error", "Could not read behavior sheet")
			return
		}
		criteria = append(criteria, title)
		value := ""
		if score.Valid {
			value = strconv.FormatInt(score.Int64, 10)
		}
		scores = append(scores, value)
	}
	if err := rows.Err(); err != nil || len(criteria) != 10 {
		httpx.Error(w, http.StatusConflict, "criteria_incomplete", "Exactly 10 criteria are required to export")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="behavior.csv"`)
	_, _ = w.Write([]byte("\xEF\xBB\xBF"))
	csvWriter := csv.NewWriter(w)
	csvWriter.Comma = ';'
	header := []string{"ФИО", "Проект"}
	for _, title := range criteria {
		header = append(header, title)
	}
	header = append(header, "Итого баллов", "Оценка")
	for i := range header {
		header[i] = spreadsheetSafe(header[i])
	}
	_ = csvWriter.Write(header)
	row := []string{spreadsheetSafe(studentName), spreadsheetSafe(project)}
	for _, score := range scores {
		row = append(row, score)
	}
	if complete {
		row = append(row, strconv.Itoa(total))
		if verdict.Valid {
			row = append(row, verdict.String)
		} else {
			row = append(row, "")
		}
	} else {
		row = append(row, "", "Не заполнено")
	}
	_ = csvWriter.Write(row)
	csvWriter.Flush()
	_ = csvWriter.Error()
}

func (h *Handler) canView(ctx context.Context, p identity.Principal, classID, studentID, yearID string) bool {
	if classID == "" || yearID == "" {
		return false
	}
	if identity.IsAdmin(p) {
		return true
	}
	if identity.HasRole(p, "student") {
		return p.StudentID != "" && p.StudentID == studentID
	}
	if !identity.HasRole(p, "teacher") && !identity.HasRole(p, "class_teacher") {
		return false
	}
	var allowed bool
	err := h.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM classes c WHERE c.id=$1 AND c.school_id=$4 AND (
		EXISTS(SELECT 1 FROM class_teacher_assignments ca WHERE ca.teacher_id=$2 AND ca.class_id=c.id AND ca.academic_year_id=$3 AND ca.school_id=$4)
		OR EXISTS(SELECT 1 FROM behavior_permissions bp WHERE bp.teacher_id=$2 AND bp.class_id=c.id AND bp.academic_year_id=$3 AND bp.school_id=$4 AND bp.can_score)))`, classID, p.UserID, yearID, p.SchoolID).Scan(&allowed)
	return err == nil && allowed
}

func (h *Handler) canEditTx(ctx context.Context, tx *sql.Tx, p identity.Principal, classID, yearID string) bool {
	return h.canEdit(ctx, tx, p, classID, yearID)
}

func (h *Handler) canEdit(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, p identity.Principal, classID, yearID string) bool {
	if identity.IsAdmin(p) {
		return true
	}
	if !identity.HasRole(p, "teacher") {
		return false
	}
	var allowed bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM behavior_permissions bp JOIN classes c ON c.id=bp.class_id
		WHERE bp.teacher_id=$1 AND bp.class_id=$2 AND bp.academic_year_id=$3 AND bp.can_score AND c.school_id=$4 AND bp.school_id=$4)`, p.UserID, classID, yearID, p.SchoolID).Scan(&allowed)
	return err == nil && allowed
}

func (h *Handler) canAccessContext(ctx context.Context, p identity.Principal, classID, termID string) bool {
	if !identity.HasRole(p, "teacher") && !identity.HasRole(p, "class_teacher") {
		return false
	}
	var allowed bool
	err := h.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM classes c JOIN terms t ON t.academic_year_id=c.academic_year_id
		WHERE c.id=$1 AND t.id=$2 AND c.school_id=$4 AND (
			EXISTS(SELECT 1 FROM class_teacher_assignments ca WHERE ca.teacher_id=$3 AND ca.class_id=c.id AND ca.academic_year_id=c.academic_year_id AND ca.school_id=$4)
			OR EXISTS(SELECT 1 FROM behavior_permissions bp WHERE bp.teacher_id=$3 AND bp.class_id=c.id AND bp.academic_year_id=c.academic_year_id AND bp.school_id=$4)))`,
		classID, termID, p.UserID, p.SchoolID).Scan(&allowed)
	return err == nil && allowed
}

func spreadsheetSafe(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}
