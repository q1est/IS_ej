package attendance

import "testing"

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
