package identity

import "testing"

func TestRoleChecks(t *testing.T) {
	student := Principal{Roles: []string{"student"}}
	if !HasRole(student, "student") || HasRole(student, "admin") || IsAdmin(student) {
		t.Fatal("student role checks are incorrect")
	}
	admin := Principal{Roles: []string{"admin"}}
	if !IsAdmin(admin) || !HasRole(admin, "teacher") || !HasRole(admin, "class_teacher") {
		t.Fatal("administrator should be allowed through administrator role checks")
	}
}
