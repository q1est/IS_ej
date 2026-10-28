package behavior

type Criterion struct {
	Code      string
	Title     string
	SortOrder int
}

func DefaultCriteria() []Criterion {
	return []Criterion{
		{Code: "schedule_rules", Title: "Расписание, правила и нормы поведения в учебной деятельности (в том числе внеурочной).", SortOrder: 1},
		{Code: "appearance", Title: "Одежда обучающегося и требования к внешнему виду.", SortOrder: 2},
		{Code: "civic_actions", Title: "Нормы поведения при гражданско-патриотических акциях.", SortOrder: 3},
		{Code: "safety", Title: "Правила пожарной безопасности, электробезопасности, поведения при угрозе ЧС.", SortOrder: 4},
		{Code: "peer_communication", Title: "Конструктивное взаимодействие со сверстниками, общепринятые нормы коммуникации.", SortOrder: 5},
		{Code: "conflict_prevention", Title: "Отсутствие умышленных конфликтов и агрессии (травли).", SortOrder: 6},
		{Code: "teacher_communication", Title: "Уважительное взаимодействие с педагогами, нормы коммуникации.", SortOrder: 7},
		{Code: "responsibility", Title: "Ответственное отношение к поручениям и рекомендациям педагогов.", SortOrder: 8},
		{Code: "lawful_behavior", Title: "Отсутствие фактов антиобщественных действий и противоправного поведения.", SortOrder: 9},
		{Code: "prevention_records", Title: "Отсутствие индивидуальной профилактической работы / учёта.", SortOrder: 10},
	}
}

func Verdict(total int) string {
	switch {
	case total >= 24:
		return "образцовое"
	case total >= 10:
		return "допустимое"
	default:
		return "недопустимое"
	}
}
