package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"school-journal/internal/identity"
)

func TestNormalizeClassStudents(t *testing.T) {
	valid := []classStudentInput{{FullName: "  Иванов   Иван  ", ExternalID: " 123 "}, {StudentID: "student-2"}}
	if err := normalizeClassStudents(valid); err != nil {
		t.Fatal(err)
	}
	if valid[0].FullName != "Иванов Иван" || valid[0].ExternalID != "123" {
		t.Fatalf("normalization: %+v", valid)
	}
	cases := map[string][]classStudentInput{
		"empty": {}, "empty name": {{FullName: "  "}},
		"duplicate name":     {{FullName: "Иванов Иван"}, {FullName: "иванов  Иван"}},
		"duplicate number":   {{FullName: "Анна", ExternalID: "1"}, {FullName: "Иван", ExternalID: "1"}},
		"duplicate existing": {{StudentID: "same"}, {StudentID: "same"}},
		"mixed identity":     {{StudentID: "existing", FullName: "Другой"}},
		"bad birth date":     {{FullName: "Иван", BirthDate: "2026-02-30"}},
		"too many":           make([]classStudentInput, 101),
	}
	for name, students := range cases {
		t.Run(name, func(t *testing.T) {
			if normalizeClassStudents(students) == nil {
				t.Fatal("expected invalid list")
			}
		})
	}
}

func TestStudentNumberGeneratedWhenOmitted(t *testing.T) {
	body := map[string]json.RawMessage{"full_name": json.RawMessage(`"Иванов Иван"`)}
	_, args, err := resourceInsert("students", body, identity.Principal{SchoolID: "school"})
	if err != nil {
		t.Fatal(err)
	}
	first := args[1].(string)
	if !strings.HasPrefix(first, "ST-") {
		t.Fatalf("unexpected number %q", first)
	}
	_, other, err := resourceInsert("students", body, identity.Principal{SchoolID: "school"})
	if err != nil || other[1] == first {
		t.Fatal("generated numbers must differ")
	}
}

func TestClassStudentAdditionRequiresAdmin(t *testing.T) {
	for _, roles := range [][]string{nil, {"teacher"}, {"class_teacher"}, {"student"}} {
		request := httptest.NewRequest(http.MethodPost, "/admin/classes/class/students", strings.NewReader(`{"students":[{"full_name":"Иван"}]}`))
		request = request.WithContext(identity.WithPrincipal(request.Context(), identity.Principal{Roles: roles}))
		response := httptest.NewRecorder()
		New(nil).addClassStudents(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("roles %v: status %d", roles, response.Code)
		}
	}
}
