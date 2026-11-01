package attendance

import (
	"errors"
	"fmt"
	"testing"
)

type postgresError struct{ state string }

func (e postgresError) Error() string    { return "database error" }
func (e postgresError) SQLState() string { return e.state }

func TestIsMissingAttendanceTable(t *testing.T) {
	if !isMissingAttendanceTable(postgresError{state: "42P01"}) {
		t.Fatal("undefined attendance table should be detected")
	}
	if !isMissingAttendanceTable(fmt.Errorf("query attendance: %w", postgresError{state: "42P01"})) {
		t.Fatal("wrapped undefined-table error should be detected")
	}
	if isMissingAttendanceTable(postgresError{state: "42501"}) {
		t.Fatal("permission error must not be reported as a missing table")
	}
	if isMissingAttendanceTable(errors.New("connection refused")) {
		t.Fatal("connection error must not be reported as a missing table")
	}
}

func TestValidStatus(t *testing.T) {
	for _, status := range []string{"present", "absent", "late", "excused"} {
		if !validStatus(status) {
			t.Errorf("validStatus(%q) = false, want true", status)
		}
	}
	for _, status := range []string{"", "unknown", "PRESENT", "unmarked"} {
		if validStatus(status) {
			t.Errorf("validStatus(%q) = true, want false", status)
		}
	}
}
