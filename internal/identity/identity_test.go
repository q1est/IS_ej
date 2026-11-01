package identity

import "testing"

func TestRoleChecks(t *testing.T) {
	student := Principal{Roles: []string{"student"}}
	if !HasRole(student, "student") || !IsStudent(student) || HasRole(student, "admin") || IsAdmin(student) {
		t.Fatal("student role checks are incorrect")
	}
	admin := Principal{Roles: []string{"admin"}}
	if !IsAdmin(admin) || HasRole(admin, "teacher") || HasRole(admin, "class_teacher") || HasRole(admin, "student") || IsStudent(admin) {
		t.Fatal("administrator must retain only explicitly assigned roles")
	}
	mixed := Principal{Roles: []string{"admin", "student"}}
	if !IsAdmin(mixed) || !HasRole(mixed, "student") || IsStudent(mixed) {
		t.Fatal("explicitly assigned roles must be preserved for administrators")
	}
}
