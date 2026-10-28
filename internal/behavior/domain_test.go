package behavior

import "testing"

import "strconv"

func TestVerdictBoundaries(t *testing.T) {
	cases := []struct {
		total int
		want  string
	}{
		{total: 0, want: "недопустимое"},
		{total: 9, want: "недопустимое"},
		{total: 10, want: "допустимое"},
		{total: 23, want: "допустимое"},
		{total: 24, want: "образцовое"},
		{total: 30, want: "образцовое"},
	}
	for _, test := range cases {
		t.Run(strconv.Itoa(test.total), func(t *testing.T) {
			if got := Verdict(test.total); got != test.want {
				t.Fatalf("Verdict(%d) = %q, want %q", test.total, got, test.want)
			}
		})
	}
}

func TestDefaultCriteriaCountAndOrder(t *testing.T) {
	criteria := DefaultCriteria()
	if len(criteria) != 10 {
		t.Fatalf("DefaultCriteria() returned %d criteria, want 10", len(criteria))
	}
	seen := map[string]bool{}
	for index, criterion := range criteria {
		if criterion.Code == "" || criterion.Title == "" || criterion.SortOrder != index+1 {
			t.Fatalf("criterion %d is incomplete or out of order: %+v", index, criterion)
		}
		if seen[criterion.Code] {
			t.Fatalf("duplicate criterion code %q", criterion.Code)
		}
		seen[criterion.Code] = true
	}
}
