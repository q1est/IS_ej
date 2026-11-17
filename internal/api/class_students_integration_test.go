package api_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"school-journal/internal/api"
	"school-journal/internal/config"
	"school-journal/internal/platform/postgres"
)

func TestClassStudentBulkWorkflow(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set; requires an isolated database with demo seed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := postgres.Open(ctx, dsn, 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	server := httptest.NewServer(api.New(config.Config{JWTSecret: []byte("integration-test-secret-at-least-32-bytes"), AccessTokenTTL: 15 * time.Minute, RefreshTokenTTL: 24 * time.Hour, AllowedOrigins: []string{"http://localhost"}}, db))
	defer server.Close()
	client := server.Client()
	admin := loginDemoUser(t, client, server.URL, "admin@demo-school.local", "DemoAdmin2026!")
	yearID := findResourceID(t, client, server.URL, admin.AccessToken, "/admin/academic-years", "2026/2027")
	prefix := fmt.Sprintf("qa-bulk-%d", time.Now().UnixNano())
	status, body := callJSON(t, client, server.URL, http.MethodPost, "/admin/classes", map[string]string{"name": prefix, "academic_year_id": yearID}, admin.AccessToken)
	if status != 201 {
		t.Fatalf("create class: %d %s", status, body)
	}
	var schoolClass struct {
		ID string `json:"id"`
	}
	decodeJSON(t, body, &schoolClass)
	var schoolID string
	if err = db.QueryRowContext(ctx, `SELECT school_id::text FROM classes WHERE id=$1`, schoolClass.ID).Scan(&schoolID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM enrollments WHERE class_id=$1`, schoolClass.ID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM students WHERE school_id=$1 AND full_name LIKE $2`, schoolID, prefix+"%")
		_, _ = db.ExecContext(context.Background(), `DELETE FROM classes WHERE id=$1`, schoolClass.ID)
	}()
	path := "/admin/classes/" + schoolClass.ID + "/students"
	status, body = callJSON(t, client, server.URL, http.MethodPost, path, map[string]any{"students": []any{map[string]string{"full_name": prefix + " Анна"}, map[string]string{"full_name": prefix + " Борис"}}}, admin.AccessToken)
	if status != 201 {
		t.Fatalf("bulk add: %d %s", status, body)
	}
	var result struct{ Created, Enrolled int }
	decodeJSON(t, body, &result)
	if result.Created != 2 || result.Enrolled != 2 {
		t.Fatalf("bulk count: %s", body)
	}
	var count int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM enrollments WHERE class_id=$1 AND from_date='2026-09-01'`, schoolClass.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("enrollment defaults: %d %v", count, err)
	}
	// The first row must roll back when a later row conflicts.
	status, body = callJSON(t, client, server.URL, http.MethodPost, path, map[string]any{"students": []any{map[string]string{"full_name": prefix + " Откат"}, map[string]string{"full_name": prefix + " Анна"}}}, admin.AccessToken)
	if status != 409 {
		t.Fatalf("duplicate card: %d %s", status, body)
	}
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM students WHERE school_id=$1 AND full_name=$2`, schoolID, prefix+" Откат").Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial write escaped rollback: %d %v", count, err)
	}
	status, body = callJSON(t, client, server.URL, http.MethodPost, "/admin/students", map[string]string{"full_name": prefix + " Вера"}, admin.AccessToken)
	if status != 201 {
		t.Fatalf("automatic student number: %d %s", status, body)
	}
	var existing struct {
		ID string `json:"id"`
	}
	decodeJSON(t, body, &existing)
	status, body = callJSON(t, client, server.URL, http.MethodPost, path, map[string]any{"students": []any{map[string]string{"student_id": existing.ID}}, "from_date": "2026-10-01"}, admin.AccessToken)
	if status != 201 {
		t.Fatalf("enroll existing: %d %s", status, body)
	}
	decodeJSON(t, body, &result)
	if result.Created != 0 || result.Enrolled != 1 {
		t.Fatalf("existing count: %s", body)
	}
	status, body = callJSON(t, client, server.URL, http.MethodPost, path, map[string]any{"students": []any{map[string]string{"student_id": existing.ID}}}, admin.AccessToken)
	if status != 409 {
		t.Fatalf("repeat enrollment: %d %s", status, body)
	}
	status, body = callJSON(t, client, server.URL, http.MethodPost, path, map[string]any{"students": []any{map[string]string{"full_name": prefix + " Дата"}}, "from_date": "2025-01-01"}, admin.AccessToken)
	if status != 400 {
		t.Fatalf("date outside year: %d %s", status, body)
	}

	// School boundaries must be checked before either creation or enrollment.
	var foreignSchool, foreignStudent, foreignYear, foreignClass string
	if err = db.QueryRowContext(ctx, `INSERT INTO schools(name) VALUES('QA foreign bulk school') RETURNING id::text`).Scan(&foreignSchool); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM students WHERE school_id=$1`, foreignSchool)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM classes WHERE school_id=$1`, foreignSchool)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM academic_years WHERE school_id=$1`, foreignSchool)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM schools WHERE id=$1`, foreignSchool)
	}()
	if err = db.QueryRowContext(ctx, `INSERT INTO students(school_id,external_id,full_name) VALUES($1,'foreign','QA foreign student') RETURNING id::text`, foreignSchool).Scan(&foreignStudent); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `INSERT INTO academic_years(school_id,name,starts_on,ends_on) VALUES($1,'QA','2026-09-01','2027-05-31') RETURNING id::text`, foreignSchool).Scan(&foreignYear); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `INSERT INTO classes(school_id,academic_year_id,name) VALUES($1,$2,'QA') RETURNING id::text`, foreignSchool, foreignYear).Scan(&foreignClass); err != nil {
		t.Fatal(err)
	}
	status, body = callJSON(t, client, server.URL, http.MethodPost, path, map[string]any{"students": []any{map[string]string{"student_id": foreignStudent}}}, admin.AccessToken)
	if status != 404 {
		t.Fatalf("foreign student must be hidden: %d %s", status, body)
	}
	status, body = callJSON(t, client, server.URL, http.MethodPost, "/admin/classes/"+foreignClass+"/students", map[string]any{"students": []any{map[string]string{"full_name": prefix + " Чужой класс"}}}, admin.AccessToken)
	if status != 404 {
		t.Fatalf("foreign class must be hidden: %d %s", status, body)
	}
	// A class teacher with both roles may read other subjects of their own class.
	classTeacher := loginDemoUser(t, client, server.URL, "class.teacher@demo-school.local", "Teacher2026!")
	classID := findResourceID(t, client, server.URL, admin.AccessToken, "/admin/classes", "7А")
	var termID string
	if err = db.QueryRowContext(ctx, `SELECT id::text FROM terms WHERE academic_year_id=$1 AND name='1 четверть'`, yearID).Scan(&termID); err != nil {
		t.Fatal(err)
	}
	if termID == "" {
		t.Fatal("no term")
	}
	status, body = callJSON(t, client, server.URL, http.MethodGet, "/grades?class_id="+classID+"&term_id="+termID, nil, classTeacher.AccessToken)
	if status != 200 {
		t.Fatalf("homeroom journal: %d %s", status, body)
	}
	var grades []struct {
		SubjectID string `json:"subject_id"`
	}
	decodeJSON(t, body, &grades)
	subjects := map[string]bool{}
	for _, grade := range grades {
		subjects[grade.SubjectID] = true
	}
	if len(subjects) < 2 {
		t.Fatalf("homeroom journal should include other subjects: %s", body)
	}
}
