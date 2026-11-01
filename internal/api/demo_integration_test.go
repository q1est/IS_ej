package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"school-journal/internal/api"
	"school-journal/internal/config"
	"school-journal/internal/platform/postgres"
)

func TestDemoTeacherInvitationAssignmentAndGradeWorkflow(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set; requires an isolated database with demo seed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := postgres.Open(ctx, dsn, 4, 2)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `DELETE FROM grades WHERE school_id=(SELECT school_id FROM users WHERE email='admin@demo-school.local') AND left(grade_type,12)='integration_'`); err != nil {
		t.Fatalf("clean prior integration grades: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM behavior_sheets WHERE student_id IN (SELECT id FROM students WHERE external_id LIKE 'qa-student-%')`); err != nil {
		t.Fatalf("clean prior integration behavior sheets: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM enrollments WHERE student_id IN (SELECT id FROM students WHERE external_id LIKE 'qa-student-%')`); err != nil {
		t.Fatalf("clean prior integration enrollments: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM invites WHERE email LIKE 'qa-student-%@demo-school.local'`); err != nil {
		t.Fatalf("clean prior integration student invitations: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM refresh_sessions WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'qa-student-%@demo-school.local')`); err != nil {
		t.Fatalf("clean prior integration student sessions: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM user_roles WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'qa-student-%@demo-school.local')`); err != nil {
		t.Fatalf("clean prior integration student roles: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE students SET user_id=NULL WHERE external_id LIKE 'qa-student-%'`); err != nil {
		t.Fatalf("unlink prior integration student accounts: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM users WHERE email LIKE 'qa-student-%@demo-school.local'`); err != nil {
		t.Fatalf("clean prior integration student accounts: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM students WHERE external_id LIKE 'qa-student-%'`); err != nil {
		t.Fatalf("clean prior integration students: %v", err)
	}

	handler := api.New(config.Config{
		JWTSecret:       []byte("integration-test-secret-with-at-least-32-bytes"),
		AllowedOrigins:  []string{"http://localhost"},
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 14 * 24 * time.Hour,
	}, db)
	server := httptest.NewServer(handler)
	defer server.Close()
	client := server.Client()
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM grades WHERE school_id=(SELECT school_id FROM users WHERE email='admin@demo-school.local') AND left(grade_type,12)='integration_'`)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM invites WHERE email LIKE 'qa-teacher-%@demo-school.local'`)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM refresh_sessions WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'qa-teacher-%@demo-school.local')`)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM user_roles WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'qa-teacher-%@demo-school.local')`)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM users WHERE email LIKE 'qa-teacher-%@demo-school.local'`)
	}()

	admin := loginDemoUser(t, client, server.URL, "admin@demo-school.local", "DemoAdmin2026!")
	status, body := callJSON(t, client, server.URL, http.MethodPost, "/auth/refresh", map[string]string{"refresh_token": admin.RefreshToken}, "")
	if status != http.StatusOK {
		t.Fatalf("refresh admin session: status=%d body=%s", status, body)
	}
	var refreshed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	decodeJSON(t, body, &refreshed)
	if refreshed.AccessToken == "" || refreshed.RefreshToken == "" {
		t.Fatal("refresh endpoint did not rotate both tokens")
	}
	status, body = callJSON(t, client, server.URL, http.MethodGet, "/me", nil, refreshed.AccessToken)
	if status != http.StatusOK {
		t.Fatalf("read refreshed principal: status=%d body=%s", status, body)
	}
	var refreshedUser struct {
		Email string `json:"email"`
	}
	decodeJSON(t, body, &refreshedUser)
	if refreshedUser.Email != "admin@demo-school.local" {
		t.Fatalf("refreshed session belongs to %q", refreshedUser.Email)
	}
	admin.AccessToken = refreshed.AccessToken
	admin.RefreshToken = refreshed.RefreshToken
	status, body = callJSON(t, client, server.URL, http.MethodPatch, "/admin/users/"+url.PathEscape(admin.User.ID), map[string]any{
		"status": "disabled", "roles": []string{"admin"},
	}, admin.AccessToken)
	if status != http.StatusConflict {
		t.Fatalf("administrator cannot disable own account: status=%d body=%s", status, body)
	}

	inviteEmail := fmt.Sprintf("qa-teacher-%d@demo-school.local", time.Now().UnixNano())
	status, body = callJSON(t, client, server.URL, http.MethodPost, "/invites", map[string]string{
		"email": inviteEmail, "role": "teacher", "target_type": "", "target_id": "", "academic_year_id": "",
	}, admin.AccessToken)
	if status != http.StatusCreated {
		t.Fatalf("create teacher invitation: status=%d body=%s", status, body)
	}
	var invitation struct {
		Token string `json:"token"`
	}
	decodeJSON(t, body, &invitation)
	if invitation.Token == "" {
		t.Fatal("teacher invitation returned no token")
	}

	status, body = callJSON(t, client, server.URL, http.MethodPost, "/invites/accept", map[string]string{
		"token": invitation.Token, "full_name": "Проверочный преподаватель", "password": "IntegrationTeacher2026!",
	}, "")
	if status != http.StatusOK {
		t.Fatalf("accept teacher invitation: status=%d body=%s", status, body)
	}
	var teacher sessionResponse
	decodeJSON(t, body, &teacher)
	if teacher.AccessToken == "" || !contains(teacher.User.Roles, "teacher") {
		t.Fatalf("invited account is not an active teacher: %+v", teacher.User)
	}

	status, body = callJSON(t, client, server.URL, http.MethodGet, "/admin/users", nil, admin.AccessToken)
	if status != http.StatusOK {
		t.Fatalf("list users as admin: status=%d body=%s", status, body)
	}
	var users []struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Roles string `json:"roles"`
	}
	decodeJSON(t, body, &users)
	if !containsUser(users, inviteEmail, "teacher") {
		t.Fatalf("accepted teacher not found in admin user directory: %s", inviteEmail)
	}

	classID := findResourceID(t, client, server.URL, admin.AccessToken, "/admin/classes", "7А")
	subjectID := findResourceID(t, client, server.URL, admin.AccessToken, "/admin/subjects", "Английский язык")
	yearID := findResourceID(t, client, server.URL, admin.AccessToken, "/admin/academic-years", "2026/2027")
	status, body = callJSON(t, client, server.URL, http.MethodPost, "/admin/teacher-assignments", map[string]string{
		"teacher_id": teacher.User.ID, "class_id": classID, "subject_id": subjectID, "academic_year_id": yearID,
	}, admin.AccessToken)
	if status != http.StatusCreated {
		t.Fatalf("assign teacher to class and subject: status=%d body=%s", status, body)
	}
	var assignment struct {
		ID string `json:"id"`
	}
	decodeJSON(t, body, &assignment)
	if assignment.ID == "" {
		t.Fatal("teacher assignment returned no id")
	}
	defer func() {
		if _, err := db.ExecContext(context.Background(), `DELETE FROM teacher_assignments WHERE id=$1`, assignment.ID); err != nil {
			t.Errorf("remove integration assignment: %v", err)
		}
	}()

	status, body = callJSON(t, client, server.URL, http.MethodGet, "/grades/contexts", nil, teacher.AccessToken)
	if status != http.StatusOK {
		t.Fatalf("list teacher grade contexts: status=%d body=%s", status, body)
	}
	var contexts []struct {
		ClassID     string `json:"class_id"`
		ClassName   string `json:"class_name"`
		SubjectID   string `json:"subject_id"`
		SubjectName string `json:"subject_name"`
		TermID      string `json:"term_id"`
		TermName    string `json:"term_name"`
		CanEdit     bool   `json:"can_edit"`
	}
	decodeJSON(t, body, &contexts)
	var termID string
	for _, item := range contexts {
		if item.ClassID == classID && item.SubjectID == subjectID && item.TermName == "1 четверть" && item.CanEdit {
			termID = item.TermID
			break
		}
	}
	if termID == "" {
		t.Fatal("new teacher assignment did not provide an editable grade context")
	}

	studentPath := "/students?class_id=" + url.QueryEscape(classID)
	status, body = callJSON(t, client, server.URL, http.MethodGet, studentPath, nil, teacher.AccessToken)
	if status != http.StatusOK {
		t.Fatalf("list assigned class students: status=%d body=%s", status, body)
	}
	var students []struct {
		ID string `json:"id"`
	}
	decodeJSON(t, body, &students)
	if len(students) != 5 {
		t.Fatalf("expected 5 students in 7А, got %d", len(students))
	}

	// Administrators must receive the school-wide scope rather than being
	// routed through student-only restrictions in any journal endpoint.
	status, body = callJSON(t, client, server.URL, http.MethodGet, "/students?class_id="+url.QueryEscape(classID), nil, admin.AccessToken)
	if status != http.StatusOK {
		t.Fatalf("admin list class students: status=%d body=%s", status, body)
	}
	var adminStudents []struct {
		ID string `json:"id"`
	}
	decodeJSON(t, body, &adminStudents)
	if len(adminStudents) != 5 {
		t.Fatalf("admin should see all 5 students in 7А, got %d", len(adminStudents))
	}

	// Snapshot any pre-existing sheets so this test can clean up only rows it owns.
	preExistingSheetIDs := make(map[string]bool)
	baselineRows, err := db.QueryContext(ctx, `SELECT id::text FROM behavior_sheets WHERE class_id=$1 AND term_id=$2 AND deleted_at IS NULL`, classID, termID)
	if err != nil {
		t.Fatalf("snapshot existing behavior sheets: %v", err)
	}
	for baselineRows.Next() {
		var sheetID string
		if err := baselineRows.Scan(&sheetID); err != nil {
			_ = baselineRows.Close()
			t.Fatalf("read existing behavior sheet id: %v", err)
		}
		preExistingSheetIDs[sheetID] = true
	}
	if err := baselineRows.Err(); err != nil {
		_ = baselineRows.Close()
		t.Fatalf("read existing behavior sheets: %v", err)
	}
	if err := baselineRows.Close(); err != nil {
		t.Fatalf("close existing behavior sheets: %v", err)
	}
	// Establish the initial behavior roster before enrolling a new student.
	behaviorPath := "/behavior/sheets?class_id=" + url.QueryEscape(classID) + "&term_id=" + url.QueryEscape(termID)
	var initialBehaviorSheets []struct {
		ID        string `json:"id"`
		StudentID string `json:"student_id"`
	}
	status, body = callJSON(t, client, server.URL, http.MethodPost, "/behavior/sheets/generate", map[string]string{
		"class_id": classID, "academic_year_id": yearID, "term_id": termID,
	}, admin.AccessToken)
	if status != http.StatusCreated {
		t.Fatalf("generate initial behavior roster: status=%d body=%s", status, body)
	}
	status, body = callJSON(t, client, server.URL, http.MethodGet, behaviorPath, nil, admin.AccessToken)
	if status != http.StatusOK {
		t.Fatalf("read generated behavior roster: status=%d body=%s", status, body)
	}
	decodeJSON(t, body, &initialBehaviorSheets)
	if len(initialBehaviorSheets) != 5 {
		t.Fatalf("expected 5 initially enrolled behavior sheets, got %d", len(initialBehaviorSheets))
	}
	initialStudentIDs := make(map[string]bool, len(initialBehaviorSheets))
	createdBaselineSheetIDs := make([]string, 0)
	for _, sheet := range initialBehaviorSheets {
		initialStudentIDs[sheet.StudentID] = true
		if !preExistingSheetIDs[sheet.ID] {
			createdBaselineSheetIDs = append(createdBaselineSheetIDs, sheet.ID)
		}
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		for _, sheetID := range createdBaselineSheetIDs {
			_, _ = db.ExecContext(cleanupCtx, `DELETE FROM behavior_sheets WHERE id=$1`, sheetID)
		}
	}()

	// A student who accepts an invitation and is enrolled after the class
	// behavior sheets were first generated must appear on the next roster read.
	qaStudentEmail := fmt.Sprintf("qa-student-%d@demo-school.local", time.Now().UnixNano())
	qaExternalID := fmt.Sprintf("qa-student-%d", time.Now().UnixNano())
	status, body = callJSON(t, client, server.URL, http.MethodPost, "/admin/students", map[string]string{
		"external_id": qaExternalID, "full_name": "Проверочный ученик",
	}, admin.AccessToken)
	if status != http.StatusCreated {
		t.Fatalf("create behavior roster student: status=%d body=%s", status, body)
	}
	var qaStudent struct {
		ID string `json:"id"`
	}
	decodeJSON(t, body, &qaStudent)
	if qaStudent.ID == "" {
		t.Fatal("created behavior roster student returned no id")
	}
	if initialStudentIDs[qaStudent.ID] {
		t.Fatalf("new test student unexpectedly appeared in the initial roster: %s", qaStudent.ID)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM behavior_sheets WHERE student_id=$1`, qaStudent.ID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM enrollments WHERE student_id=$1`, qaStudent.ID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM invites WHERE email=$1`, qaStudentEmail)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM refresh_sessions WHERE user_id IN (SELECT id FROM users WHERE email=$1)`, qaStudentEmail)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM user_roles WHERE user_id IN (SELECT id FROM users WHERE email=$1)`, qaStudentEmail)
		_, _ = db.ExecContext(cleanupCtx, `UPDATE students SET user_id=NULL WHERE id=$1`, qaStudent.ID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM users WHERE email=$1`, qaStudentEmail)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM students WHERE id=$1`, qaStudent.ID)
	}()
	status, body = callJSON(t, client, server.URL, http.MethodPost, "/invites", map[string]string{
		"email": qaStudentEmail, "role": "student", "target_type": "student", "target_id": qaStudent.ID,
	}, admin.AccessToken)
	if status != http.StatusCreated {
		t.Fatalf("invite behavior roster student: status=%d body=%s", status, body)
	}
	var qaInvite struct {
		Token string `json:"token"`
	}
	decodeJSON(t, body, &qaInvite)
	status, body = callJSON(t, client, server.URL, http.MethodPost, "/invites/accept", map[string]string{
		"token": qaInvite.Token, "full_name": "Проверочный ученик", "password": "IntegrationStudent2026!",
	}, "")
	if status != http.StatusOK {
		t.Fatalf("accept behavior roster student invitation: status=%d body=%s", status, body)
	}
	status, body = callJSON(t, client, server.URL, http.MethodPost, "/admin/enrollments", map[string]string{
		"student_id": qaStudent.ID, "class_id": classID, "academic_year_id": yearID,
	}, admin.AccessToken)
	if status != http.StatusCreated {
		t.Fatalf("enroll behavior roster student: status=%d body=%s", status, body)
	}
	behaviorPath = "/behavior/sheets?class_id=" + url.QueryEscape(classID) + "&term_id=" + url.QueryEscape(termID)
	status, body = callJSON(t, client, server.URL, http.MethodGet, behaviorPath, nil, admin.AccessToken)
	if status != http.StatusOK {
		t.Fatalf("read behavior roster after enrollment: status=%d body=%s", status, body)
	}
	var behaviorSheets []struct {
		StudentID string `json:"student_id"`
	}
	decodeJSON(t, body, &behaviorSheets)
	foundNewStudent := false
	for _, sheet := range behaviorSheets {
		if sheet.StudentID == qaStudent.ID {
			foundNewStudent = true
			break
		}
	}
	if !foundNewStudent {
		t.Fatalf("newly enrolled student missing from behavior roster: %s", body)
	}

	status, body = callJSON(t, client, server.URL, http.MethodGet, "/grades/contexts", nil, admin.AccessToken)
	if status != http.StatusOK {
		t.Fatalf("admin grade contexts: status=%d body=%s", status, body)
	}
	var adminContexts []struct {
		ClassID   string `json:"class_id"`
		SubjectID string `json:"subject_id"`
		CanEdit   bool   `json:"can_edit"`
	}
	decodeJSON(t, body, &adminContexts)
	adminCanEdit := false
	adminSubjectContextCount := 0
	for _, item := range adminContexts {
		if item.ClassID == classID && item.CanEdit {
			adminCanEdit = true
			if item.SubjectID != "" {
				adminSubjectContextCount++
			}
		}
	}
	if !adminCanEdit {
		t.Fatalf("admin should receive editable grade context for 7А: %s", body)
	}
	if adminSubjectContextCount == 0 {
		t.Fatalf("admin should receive subject-specific grade contexts for 7А: %s", body)
	}
	adminSubjectID := ""
	for _, item := range adminContexts {
		if item.ClassID == classID && item.CanEdit && item.SubjectID != "" {
			adminSubjectID = item.SubjectID
			break
		}
	}
	if adminSubjectID == "" {
		t.Fatalf("admin should receive subject-specific grade contexts for 7А: %s", body)
	}
	adminGradePath := "/grades?class_id=" + url.QueryEscape(classID) + "&term_id=" + url.QueryEscape(termID) + "&subject_id=" + url.QueryEscape(adminSubjectID)
	status, body = callJSON(t, client, server.URL, http.MethodGet, adminGradePath, nil, admin.AccessToken)
	if status != http.StatusOK {
		t.Fatalf("admin list grades: status=%d body=%s", status, body)
	}

	status, body = callJSON(t, client, server.URL, http.MethodGet, "/attendance/contexts", nil, admin.AccessToken)
	if status != http.StatusOK {
		t.Fatalf("admin attendance contexts: status=%d body=%s", status, body)
	}
	var attendanceContexts []struct {
		ClassID string `json:"class_id"`
		CanEdit bool   `json:"can_edit"`
	}
	decodeJSON(t, body, &attendanceContexts)
	adminAttendanceCanEdit := false
	for _, item := range attendanceContexts {
		if item.ClassID == classID && item.CanEdit {
			adminAttendanceCanEdit = true
			break
		}
	}
	if !adminAttendanceCanEdit {
		t.Fatalf("admin should receive editable attendance context for 7А: %s", body)
	}
	attendanceDate := "2026-10-31"
	attendanceNote := fmt.Sprintf("integration-test-%d", time.Now().UnixNano())
	status, body = callJSON(t, client, server.URL, http.MethodGet,
		"/attendance?class_id="+url.QueryEscape(classID)+"&date="+attendanceDate, nil, admin.AccessToken)
	if status != http.StatusOK {
		t.Fatalf("admin read attendance: status=%d body=%s", status, body)
	}
	var attendance struct {
		Students []struct {
			ID string `json:"student_id"`
		} `json:"students"`
	}
	decodeJSON(t, body, &attendance)
	if len(attendance.Students) != 6 {
		t.Fatalf("admin should see all 6 enrolled students in attendance after integration enrollment, got %d", len(attendance.Students))
	}
	status, body = callJSON(t, client, server.URL, http.MethodPut, "/attendance", map[string]any{
		"class_id": classID, "date": attendanceDate,
		"records": []map[string]string{{"student_id": attendance.Students[0].ID, "status": "present", "note": attendanceNote}},
	}, admin.AccessToken)
	if status != http.StatusOK {
		t.Fatalf("admin save attendance: status=%d body=%s", status, body)
	}
	var savedAttendance struct {
		Saved int `json:"saved"`
	}
	decodeJSON(t, body, &savedAttendance)
	if savedAttendance.Saved != 1 {
		t.Fatalf("admin save attendance count = %d, want 1", savedAttendance.Saved)
	}
	defer func() {
		if _, err := db.ExecContext(context.Background(), `DELETE FROM attendance_records WHERE class_id=$1 AND attendance_date=$2 AND note=$3`, classID, attendanceDate, attendanceNote); err != nil {
			t.Errorf("remove integration attendance: %v", err)
		}
	}()

	gradeType := fmt.Sprintf("integration_%d", time.Now().UnixNano())
	gradeBody := map[string]any{
		"student_id": students[len(students)-1].ID, "class_id": classID, "subject_id": subjectID, "term_id": termID,
		"work_date": "2026-10-31", "value": 4, "grade_type": gradeType, "comment": "QA create",
	}
	status, body = callJSON(t, client, server.URL, http.MethodPost, "/grades", gradeBody, teacher.AccessToken)
	if status != http.StatusCreated {
		t.Fatalf("create grade: status=%d body=%s", status, body)
	}
	var grade struct {
		ID string `json:"id"`
	}
	decodeJSON(t, body, &grade)
	if grade.ID == "" {
		t.Fatal("grade creation returned no id")
	}
	defer func() {
		if _, err := db.ExecContext(context.Background(), `DELETE FROM grades WHERE id=$1`, grade.ID); err != nil {
			t.Errorf("remove teacher integration grade: %v", err)
		}
	}()
	adminGradeBody := map[string]any{
		"student_id": students[0].ID, "class_id": classID, "subject_id": subjectID, "term_id": termID,
		"work_date": "2026-10-30", "value": 5, "grade_type": fmt.Sprintf("integration_admin_%d", time.Now().UnixNano()),
	}
	status, body = callJSON(t, client, server.URL, http.MethodPost, "/grades", adminGradeBody, admin.AccessToken)
	if status != http.StatusCreated {
		t.Fatalf("admin create grade: status=%d body=%s", status, body)
	}
	var adminGrade struct {
		ID string `json:"id"`
	}
	decodeJSON(t, body, &adminGrade)
	if adminGrade.ID == "" {
		t.Fatal("admin grade creation returned no id")
	}
	if _, err := db.ExecContext(context.Background(), `DELETE FROM grades WHERE id=$1`, adminGrade.ID); err != nil {
		t.Fatalf("remove admin integration grade: %v", err)
	}
	status, body = callJSON(t, client, server.URL, http.MethodPost, "/grades", gradeBody, teacher.AccessToken)
	if status != http.StatusConflict {
		t.Fatalf("duplicate grade should conflict: status=%d body=%s", status, body)
	}

	status, body = callJSON(t, client, server.URL, http.MethodPatch, "/grades/"+url.PathEscape(grade.ID), map[string]any{
		"value": 5, "comment": "QA updated",
	}, teacher.AccessToken)
	if status != http.StatusOK {
		t.Fatalf("edit grade: status=%d body=%s", status, body)
	}
	status, body = callJSON(t, client, server.URL, http.MethodGet,
		"/grades?class_id="+url.QueryEscape(classID)+"&subject_id="+url.QueryEscape(subjectID)+"&term_id="+url.QueryEscape(termID), nil, teacher.AccessToken)
	if status != http.StatusOK {
		t.Fatalf("verify edited grade: status=%d body=%s", status, body)
	}
	var editedGrades []struct {
		ID      string `json:"id"`
		Value   int    `json:"value"`
		Comment string `json:"comment"`
	}
	decodeJSON(t, body, &editedGrades)
	updated := false
	for _, item := range editedGrades {
		if item.ID == grade.ID && item.Value == 5 && item.Comment == "QA updated" {
			updated = true
			break
		}
	}
	if !updated {
		t.Fatalf("edited grade was not returned with new values: %s", body)
	}

	student := loginDemoUser(t, client, server.URL, "student01@demo-school.local", "Student2026!")
	studentGradesPath := "/grades?term_id=" + url.QueryEscape(termID)
	status, body = callJSON(t, client, server.URL, http.MethodGet, studentGradesPath, nil, student.AccessToken)
	if status != http.StatusOK {
		t.Fatalf("student grade list: status=%d body=%s", status, body)
	}
	var studentGrades []struct {
		StudentID string `json:"student_id"`
		ID        string `json:"id"`
	}
	decodeJSON(t, body, &studentGrades)
	if len(studentGrades) != 16 {
		t.Fatalf("student should see 16 own grades, got %d", len(studentGrades))
	}
	for _, item := range studentGrades {
		if item.StudentID != student.User.StudentID {
			t.Fatalf("student received another student's grade %s", item.ID)
		}
	}

	status, body = callJSON(t, client, server.URL, http.MethodPatch, "/grades/"+url.PathEscape(grade.ID), map[string]any{"value": 2}, student.AccessToken)
	if status != http.StatusForbidden {
		t.Fatalf("student must not edit grades: status=%d body=%s", status, body)
	}

	status, body = callJSON(t, client, server.URL, http.MethodDelete, "/grades/"+url.PathEscape(grade.ID), nil, teacher.AccessToken)
	if status != http.StatusNoContent {
		t.Fatalf("delete grade: status=%d body=%s", status, body)
	}
	status, body = callJSON(t, client, server.URL, http.MethodGet,
		"/grades?class_id="+url.QueryEscape(classID)+"&subject_id="+url.QueryEscape(subjectID)+"&term_id="+url.QueryEscape(termID), nil, teacher.AccessToken)
	if status != http.StatusOK {
		t.Fatalf("verify deleted grade: status=%d body=%s", status, body)
	}
	var remaining []map[string]any
	decodeJSON(t, body, &remaining)
	if len(remaining) != 0 {
		t.Fatalf("soft-deleted grade is still visible; remaining=%d", len(remaining))
	}

	status, body = callJSON(t, client, server.URL, http.MethodPatch, "/admin/users/"+url.PathEscape(admin.User.ID), map[string]any{
		"status": "active", "roles": []string{"teacher"},
	}, admin.AccessToken)
	if status != http.StatusConflict {
		t.Fatalf("administrator cannot remove own administrator role: status=%d body=%s", status, body)
	}

	status, body = callJSON(t, client, server.URL, http.MethodPatch, "/admin/users/"+url.PathEscape(teacher.User.ID), map[string]any{
		"status": "active", "roles": []string{"class_teacher"},
	}, admin.AccessToken)
	if status != http.StatusOK {
		t.Fatalf("change teacher role: status=%d body=%s", status, body)
	}
	status, body = callJSON(t, client, server.URL, http.MethodGet, "/grades/contexts", nil, teacher.AccessToken)
	if status != http.StatusOK {
		t.Fatalf("read contexts after role change: status=%d body=%s", status, body)
	}
	var changedContexts []map[string]any
	decodeJSON(t, body, &changedContexts)
	if len(changedContexts) != 0 {
		t.Fatalf("removed teacher role still exposes subject contexts: %s", body)
	}

	status, body = callJSON(t, client, server.URL, http.MethodPatch, "/admin/users/"+url.PathEscape(teacher.User.ID), map[string]any{
		"status": "disabled", "roles": []string{"teacher"},
	}, admin.AccessToken)
	if status != http.StatusOK {
		t.Fatalf("disable teacher account: status=%d body=%s", status, body)
	}
	var disabledUser struct {
		RevokedSessions int64 `json:"revoked_sessions"`
	}
	decodeJSON(t, body, &disabledUser)
	if disabledUser.RevokedSessions == 0 {
		t.Fatal("disabling the teacher did not revoke its refresh session")
	}
	status, body = callJSON(t, client, server.URL, http.MethodGet, "/grades/contexts", nil, teacher.AccessToken)
	if status != http.StatusUnauthorized {
		t.Fatalf("disabled teacher must immediately lose API access: status=%d body=%s", status, body)
	}

	status, body = callJSON(t, client, server.URL, http.MethodPatch, "/admin/users/"+url.PathEscape(teacher.User.ID), map[string]any{
		"status": "active", "roles": []string{"teacher"},
	}, admin.AccessToken)
	if status != http.StatusOK {
		t.Fatalf("reenable teacher account: status=%d body=%s", status, body)
	}
	status, body = callJSON(t, client, server.URL, http.MethodGet, "/grades/contexts", nil, teacher.AccessToken)
	if status != http.StatusOK {
		t.Fatalf("verify teacher access after reactivation: status=%d body=%s", status, body)
	}
	decodeJSON(t, body, &contexts)
	if len(contexts) == 0 || !contexts[0].CanEdit {
		t.Fatalf("teacher assignment scope was not preserved after role change: %s", body)
	}
	status, body = callJSON(t, client, server.URL, http.MethodPost, "/auth/refresh", map[string]string{"refresh_token": teacher.RefreshToken}, "")
	if status != http.StatusUnauthorized {
		t.Fatalf("disabled teacher refresh token must stay revoked after reactivation: status=%d body=%s", status, body)
	}
}

type sessionResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	User         struct {
		ID        string   `json:"id"`
		StudentID string   `json:"student_id"`
		Email     string   `json:"email"`
		Roles     []string `json:"roles"`
	} `json:"user"`
}

func loginDemoUser(t *testing.T, client *http.Client, baseURL, email, password string) sessionResponse {
	t.Helper()
	status, body := callJSON(t, client, baseURL, http.MethodPost, "/auth/login", map[string]string{
		"email": email, "password": password,
	}, "")
	if status != http.StatusOK {
		t.Fatalf("login %s: status=%d body=%s", email, status, body)
	}
	var session sessionResponse
	decodeJSON(t, body, &session)
	if session.AccessToken == "" {
		t.Fatalf("login %s returned no access token", email)
	}
	return session
}

func findResourceID(t *testing.T, client *http.Client, baseURL, token, path, name string) string {
	t.Helper()
	status, body := callJSON(t, client, baseURL, http.MethodGet, path, nil, token)
	if status != http.StatusOK {
		t.Fatalf("list %s: status=%d body=%s", path, status, body)
	}
	var resources []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	decodeJSON(t, body, &resources)
	for _, resource := range resources {
		if resource.Name == name {
			return resource.ID
		}
	}
	t.Fatalf("%s %q not found in %s", name, path, body)
	return ""
}

func containsUser(users []struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Roles string `json:"roles"`
}, email, role string) bool {
	for _, user := range users {
		if user.Email == email && contains(strings.Split(user.Roles, ", "), role) {
			return true
		}
	}
	return false
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func callJSON(t *testing.T, client *http.Client, baseURL, method, path string, payload any, accessToken string) (int, []byte) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("encode request: %v", err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, baseURL+path, body)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if accessToken != "" {
		request.Header.Set("Authorization", "Bearer "+accessToken)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s %s: %v", method, path, err)
	}
	return response.StatusCode, responseBody
}

func decodeJSON(t *testing.T, body []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, body)
	}
}
