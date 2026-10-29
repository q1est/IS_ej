package admin

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
	"school-journal/internal/audit"
	"school-journal/internal/identity"
	"school-journal/internal/platform/httpx"
)

const (
	workbookMaxBytes = 20 << 20
	workbookMaxRows  = 1000
)

type workbookSheet struct {
	name    string
	headers []string
	rows    [][]string
	widths  []float64
	query   string
	args    []any
}

// exportExcel returns a school-scoped workbook. Authentication data and secrets
// are deliberately excluded; the workbook contains only school records.
func (h *Handler) exportExcel(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok || !identity.IsAdmin(p) {
		httpx.Error(w, http.StatusForbidden, "forbidden", "Administrator access required")
		return
	}

	sheets, err := h.exportSheets(r.Context(), p.SchoolID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "export_error", "Could not read school data for export")
		return
	}

	f := excelize.NewFile()
	defer f.Close()
	first := true
	for _, sheet := range sheets {
		var index int
		if first {
			index, err = f.GetSheetIndex("Sheet1")
			if err == nil {
				err = f.SetSheetName("Sheet1", sheet.name)
			}
			first = false
		} else {
			index, err = f.NewSheet(sheet.name)
		}
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "export_error", "Could not create workbook sheets")
			return
		}
		_ = index
		style, styleErr := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"365F91"}, Pattern: 1}})
		if styleErr != nil {
			httpx.Error(w, http.StatusInternalServerError, "export_error", "Could not format workbook")
			return
		}
		for col, header := range sheet.headers {
			cell, _ := excelize.CoordinatesToCellName(col+1, 1)
			if err := f.SetCellValue(sheet.name, cell, header); err != nil {
				httpx.Error(w, http.StatusInternalServerError, "export_error", "Could not write workbook headers")
				return
			}
		}
		last, _ := excelize.CoordinatesToCellName(len(sheet.headers), 1)
		if len(sheet.headers) > 0 {
			if err := f.SetCellStyle(sheet.name, "A1", last, style); err != nil {
				httpx.Error(w, http.StatusInternalServerError, "export_error", "Could not format workbook headers")
				return
			}
		}
		for rowIndex, row := range sheet.rows {
			for col, value := range row {
				cell, _ := excelize.CoordinatesToCellName(col+1, rowIndex+2)
				if err := f.SetCellValue(sheet.name, cell, value); err != nil {
					httpx.Error(w, http.StatusInternalServerError, "export_error", "Could not write workbook data")
					return
				}
			}
		}
		for col, width := range sheet.widths {
			name, _ := excelize.ColumnNumberToName(col + 1)
			if err := f.SetColWidth(sheet.name, name, name, width); err != nil {
				httpx.Error(w, http.StatusInternalServerError, "export_error", "Could not format workbook columns")
				return
			}
		}
		_ = f.SetPanes(sheet.name, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	}

	var output bytes.Buffer
	if err := f.Write(&output); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "export_error", "Could not encode workbook")
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="school-data.xlsx"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(output.Bytes())
}

func (h *Handler) exportSheets(ctx context.Context, schoolID string) ([]workbookSheet, error) {
	sheets := []workbookSheet{
		{name: "Учебные годы", headers: []string{"Название", "Начало", "Окончание", "Текущий"}, query: `SELECT name,starts_on::text,ends_on::text,is_current FROM academic_years WHERE school_id=$1 ORDER BY starts_on DESC`, args: []any{schoolID}, widths: []float64{24, 14, 14, 12}},
		{name: "Классы", headers: []string{"Класс", "Учебный год"}, query: `SELECT c.name,y.name FROM classes c JOIN academic_years y ON y.id=c.academic_year_id WHERE c.school_id=$1 ORDER BY y.starts_on DESC,c.name`, args: []any{schoolID}, widths: []float64{20, 24}},
		{name: "Предметы", headers: []string{"Предмет"}, query: `SELECT name FROM subjects WHERE school_id=$1 ORDER BY name`, args: []any{schoolID}, widths: []float64{32}},
		{name: "Периоды", headers: []string{"Период", "Учебный год", "Начало", "Окончание"}, query: `SELECT t.name,y.name,t.starts_on::text,t.ends_on::text FROM terms t JOIN academic_years y ON y.id=t.academic_year_id WHERE t.school_id=$1 ORDER BY t.starts_on`, args: []any{schoolID}, widths: []float64{22, 24, 14, 14}},
		{name: "Ученики", headers: []string{"Табельный номер", "ФИО", "Дата рождения", "Класс", "Учебный год", "Дата начала зачисления"}, query: `SELECT s.external_id,s.full_name,COALESCE(s.birth_date::text,''),COALESCE(c.name,''),COALESCE(y.name,''),COALESCE(e.from_date::text,'') FROM students s LEFT JOIN LATERAL (SELECT e.class_id,e.academic_year_id,e.from_date FROM enrollments e WHERE e.student_id=s.id AND e.status='active' ORDER BY e.created_at DESC LIMIT 1) e ON true LEFT JOIN classes c ON c.id=e.class_id LEFT JOIN academic_years y ON y.id=e.academic_year_id WHERE s.school_id=$1 ORDER BY s.full_name`, args: []any{schoolID}, widths: []float64{18, 34, 16, 16, 24, 24}},
		{name: "Зачисления", headers: []string{"Табельный номер", "Класс", "Учебный год", "Статус", "Дата начала", "Дата окончания"}, query: `SELECT s.external_id,c.name,y.name,e.status,COALESCE(e.from_date::text,''),COALESCE(e.to_date::text,'') FROM enrollments e JOIN students s ON s.id=e.student_id JOIN classes c ON c.id=e.class_id JOIN academic_years y ON y.id=e.academic_year_id WHERE e.school_id=$1 ORDER BY y.starts_on DESC,c.name,s.full_name`, args: []any{schoolID}, widths: []float64{18, 16, 24, 16, 16, 16}},
		{name: "Оценки", headers: []string{"Табельный номер", "Предмет", "Класс", "Учебный год", "Период", "Оценка", "Тип оценки", "Дата", "Комментарий"}, query: `SELECT s.external_id,sub.name,c.name,y.name,t.name,g.value,g.grade_type,g.work_date::text,COALESCE(g.comment,'') FROM grades g JOIN students s ON s.id=g.student_id JOIN subjects sub ON sub.id=g.subject_id JOIN classes c ON c.id=g.class_id JOIN academic_years y ON y.id=g.academic_year_id JOIN terms t ON t.id=g.term_id WHERE g.school_id=$1 AND g.deleted_at IS NULL ORDER BY g.work_date DESC,s.full_name`, args: []any{schoolID}, widths: []float64{18, 24, 14, 24, 18, 12, 16, 14, 36}},
		{name: "Проекты", headers: []string{"Проект"}, query: `SELECT name FROM projects WHERE school_id=$1 ORDER BY name`, args: []any{schoolID}, widths: []float64{32}},
		{name: "Проекты учеников", headers: []string{"Табельный номер", "Проект", "Учебный год"}, query: `SELECT s.external_id,p.name,y.name FROM student_projects sp JOIN students s ON s.id=sp.student_id JOIN projects p ON p.id=sp.project_id JOIN academic_years y ON y.id=sp.academic_year_id WHERE sp.school_id=$1 ORDER BY y.starts_on DESC,s.full_name,p.name`, args: []any{schoolID}, widths: []float64{18, 30, 24}},
		{name: "Назначения учителей", headers: []string{"Email учителя", "Класс", "Предмет", "Учебный год"}, query: `SELECT u.email::text,c.name,s.name,y.name FROM teacher_assignments a JOIN users u ON u.id=a.teacher_id JOIN classes c ON c.id=a.class_id JOIN subjects s ON s.id=a.subject_id JOIN academic_years y ON y.id=a.academic_year_id WHERE a.school_id=$1 ORDER BY y.starts_on DESC,c.name,s.name`, args: []any{schoolID}, widths: []float64{30, 14, 24, 24}},
		{name: "Классные руководители", headers: []string{"Email учителя", "Класс", "Учебный год"}, query: `SELECT u.email::text,c.name,y.name FROM class_teacher_assignments a JOIN users u ON u.id=a.teacher_id JOIN classes c ON c.id=a.class_id JOIN academic_years y ON y.id=a.academic_year_id WHERE a.school_id=$1 ORDER BY y.starts_on DESC,c.name`, args: []any{schoolID}, widths: []float64{30, 14, 24}},
		{name: "Права оценивать поведение", headers: []string{"Email учителя", "Класс", "Учебный год", "Разрешено"}, query: `SELECT u.email::text,c.name,y.name,p.can_score FROM behavior_permissions p JOIN users u ON u.id=p.teacher_id JOIN classes c ON c.id=p.class_id JOIN academic_years y ON y.id=p.academic_year_id WHERE p.school_id=$1 ORDER BY y.starts_on DESC,c.name`, args: []any{schoolID}, widths: []float64{30, 14, 24, 14}},
	}
	for i := range sheets {
		rows, err := h.queryWorkbookRows(ctx, sheets[i].query, sheets[i].args...)
		if err != nil {
			return nil, err
		}
		sheets[i].rows = rows
	}
	criteria, err := h.behaviorCriteria(ctx, schoolID)
	if err != nil {
		return nil, err
	}
	behaviorSheet := workbookSheet{name: "Ведомости поведения", headers: []string{"Табельный номер", "Класс", "Учебный год", "Период"}, query: ``, args: nil, widths: []float64{18, 16, 24, 18}}
	for _, criterion := range criteria {
		behaviorSheet.headers = append(behaviorSheet.headers, criterion.code)
		behaviorSheet.widths = append(behaviorSheet.widths, 20)
	}
	behaviorSheet.rows, err = h.exportBehaviorRows(ctx, schoolID, criteria)
	if err != nil {
		return nil, err
	}
	sheets = append(sheets, behaviorSheet)
	criteriaSheet := workbookSheet{name: "Критерии поведения", headers: []string{"Код", "Название", "Макс. балл", "Порядок"}, widths: []float64{24, 76, 15, 12}}
	criteriaSheet.rows, err = h.queryWorkbookRows(ctx, `SELECT code,title,max_score,sort_order FROM behavior_criteria WHERE school_id=$1 ORDER BY sort_order`, schoolID)
	if err != nil {
		return nil, err
	}
	sheets = append(sheets, criteriaSheet)
	return sheets, nil
}

type behaviorCriterion struct {
	id, code string
	maxScore int
}

func (h *Handler) behaviorCriteria(ctx context.Context, schoolID string) ([]behaviorCriterion, error) {
	rows, err := h.db.QueryContext(ctx, `SELECT id::text,code,max_score FROM behavior_criteria WHERE school_id=$1 ORDER BY sort_order`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	criteria := make([]behaviorCriterion, 0, 10)
	for rows.Next() {
		var item behaviorCriterion
		if err := rows.Scan(&item.id, &item.code, &item.maxScore); err != nil {
			return nil, err
		}
		criteria = append(criteria, item)
	}
	return criteria, rows.Err()
}

func (h *Handler) exportBehaviorRows(ctx context.Context, schoolID string, criteria []behaviorCriterion) ([][]string, error) {
	rows, err := h.db.QueryContext(ctx, `SELECT s.external_id,c.name,y.name,t.name,bs.id::text,bc.code,sc.score
		FROM behavior_sheets bs JOIN students s ON s.id=bs.student_id JOIN classes c ON c.id=bs.class_id
		JOIN academic_years y ON y.id=bs.academic_year_id JOIN terms t ON t.id=bs.term_id
		LEFT JOIN behavior_criteria bc ON bc.school_id=bs.school_id
		LEFT JOIN behavior_scores sc ON sc.behavior_sheet_id=bs.id AND sc.criterion_id=bc.id
		WHERE bs.school_id=$1 AND bs.deleted_at IS NULL ORDER BY y.starts_on DESC,c.name,s.full_name,t.starts_on,bc.sort_order`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([][]string, 0)
	var currentID string
	var currentRow []string
	scores := make(map[string]string, len(criteria))
	flush := func() {
		if currentID == "" {
			return
		}
		for _, criterion := range criteria {
			currentRow = append(currentRow, scores[criterion.code])
		}
		result = append(result, currentRow)
	}
	for rows.Next() {
		var externalID, className, yearName, termName, sheetID string
		var code sql.NullString
		var score sql.NullInt64
		if err := rows.Scan(&externalID, &className, &yearName, &termName, &sheetID, &code, &score); err != nil {
			return nil, err
		}
		if currentID != sheetID {
			flush()
			currentID = sheetID
			currentRow = []string{externalID, className, yearName, termName}
			scores = make(map[string]string, len(criteria))
		}
		if code.Valid && score.Valid {
			scores[code.String] = strconv.FormatInt(score.Int64, 10)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	flush()
	return result, nil
}

func (h *Handler) queryWorkbookRows(ctx context.Context, query string, args ...any) ([][]string, error) {
	rows, err := h.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result := make([][]string, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		dest := make([]any, len(columns))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		row := make([]string, len(values))
		for i, value := range values {
			row[i] = workbookString(value)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func workbookString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []byte:
		return string(v)
	case bool:
		return strconv.FormatBool(v)
	case time.Time:
		return v.Format(time.RFC3339)
	case int64:
		return strconv.FormatInt(v, 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int:
		return strconv.Itoa(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}

type workbookRecord struct {
	row    int
	values map[string]string
}

type workbookProblem struct {
	Sheet  string `json:"sheet"`
	Row    int    `json:"row"`
	Reason string `json:"reason"`
}

func (h *Handler) importExcel(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.FromContext(r.Context())
	if !ok || !identity.IsAdmin(p) {
		httpx.Error(w, http.StatusForbidden, "forbidden", "Administrator access required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, workbookMaxBytes+(1<<20))
	if err := r.ParseMultipartForm(workbookMaxBytes); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_workbook", "Upload a valid Excel workbook under 20 MB")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("file")
	if err != nil || header.Size > workbookMaxBytes {
		httpx.Error(w, http.StatusBadRequest, "invalid_workbook", "Choose an Excel .xlsx file under 20 MB")
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, workbookMaxBytes+1))
	if err != nil || len(content) > workbookMaxBytes {
		httpx.Error(w, http.StatusBadRequest, "invalid_workbook", "The Excel file exceeds the 20 MB limit")
		return
	}
	book, err := excelize.OpenReader(bytes.NewReader(content), excelize.Options{RawCellValue: true, UnzipSizeLimit: 40 << 20, UnzipXMLSizeLimit: 20 << 20})
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_workbook", "Could not read the .xlsx workbook")
		return
	}
	defer book.Close()

	imports := make(map[string][]workbookRecord)
	rowCount := 0
	for _, sheetName := range book.GetSheetList() {
		rows, err := book.GetRows(sheetName, excelize.Options{RawCellValue: true})
		if err != nil {
			httpx.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_workbook", "message": "Could not read worksheet", "details": []workbookProblem{{Sheet: sheetName, Row: 1, Reason: "Не удалось прочитать лист"}}})
			return
		}
		if len(rows) == 0 {
			continue
		}
		if len(rows)-1 > workbookMaxRows {
			httpx.Error(w, http.StatusBadRequest, "too_many_rows", "An import workbook can contain at most 1,000 data rows")
			return
		}
		if err := rejectFormulas(book, sheetName, len(rows)); err != nil {
			httpx.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_workbook", "message": "Formulas are not accepted as import values", "details": []workbookProblem{{Sheet: sheetName, Row: 1, Reason: "Перед загрузкой замените формулы готовыми значениями"}}})
			return
		}
		records, err := parseWorkbookSheet(sheetName, rows)
		if err != nil {
			httpx.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_workbook", "message": err.Error(), "details": []workbookProblem{{Sheet: sheetName, Row: 1, Reason: err.Error()}}})
			return
		}
		rowCount += len(records)
		if rowCount > workbookMaxRows {
			httpx.Error(w, http.StatusBadRequest, "too_many_rows", "An import workbook can contain at most 1,000 data rows")
			return
		}
		if len(records) > 0 {
			imports[sheetName] = records
		}
	}
	if rowCount == 0 {
		httpx.Error(w, http.StatusBadRequest, "empty_workbook", "No supported data rows were found")
		return
	}

	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not start import")
		return
	}
	defer tx.Rollback()
	counts := make(map[string]int)
	for _, name := range importSheetOrder {
		for _, record := range imports[name] {
			if err := importWorkbookRecord(r.Context(), tx, name, record.values, p); err != nil {
				reason := err.Error()
				httpx.JSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "import_row_failed", "message": "Импорт отменён; данные не изменены", "details": []workbookProblem{{Sheet: name, Row: record.row, Reason: reason}}})
				return
			}
			counts[name]++
		}
	}
	if err := audit.Write(r.Context(), tx, r, p, "import", "school_workbook", "school", nil, map[string]any{"rows": rowCount, "sheets": counts}); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "audit_error", "Could not record import")
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "import_error", "Could not commit imported data")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"imported": counts, "total": rowCount})
}

func rejectFormulas(book *excelize.File, sheet string, rowCount int) error {
	for row := 1; row <= rowCount; row++ {
		for col := 1; col <= 16; col++ {
			cell, _ := excelize.CoordinatesToCellName(col, row)
			formula, err := book.GetCellFormula(sheet, cell)
			if err != nil {
				return err
			}
			if formula != "" {
				return fmt.Errorf("formulas are not allowed")
			}
		}
	}
	return nil
}

var importSheetOrder = []string{"Учебные годы", "Классы", "Предметы", "Периоды", "Критерии поведения", "Проекты", "Ученики", "Зачисления", "Назначения учителей", "Классные руководители", "Права оценивать поведение", "Проекты учеников", "Оценки", "Ведомости поведения"}

func parseWorkbookSheet(name string, rows [][]string) ([]workbookRecord, error) {
	required := map[string][]string{
		"Учебные годы": {"Название", "Начало", "Окончание"}, "Классы": {"Класс", "Учебный год"}, "Предметы": {"Предмет"},
		"Периоды": {"Период", "Учебный год", "Начало", "Окончание"}, "Критерии поведения": {"Код", "Название", "Макс. балл", "Порядок"}, "Ученики": {"Табельный номер", "ФИО"},
		"Зачисления": {"Табельный номер", "Класс", "Учебный год"}, "Оценки": {"Табельный номер", "Предмет", "Класс", "Учебный год", "Период", "Оценка", "Дата"},
		"Проекты": {"Проект"}, "Проекты учеников": {"Табельный номер", "Проект", "Учебный год"},
		"Назначения учителей": {"Email учителя", "Класс", "Предмет", "Учебный год"}, "Классные руководители": {"Email учителя", "Класс", "Учебный год"},
		"Права оценивать поведение": {"Email учителя", "Класс", "Учебный год", "Разрешено"}, "Ведомости поведения": {"Табельный номер", "Класс", "Учебный год", "Период"},
	}
	headers, supported := required[name]
	if !supported {
		return nil, fmt.Errorf("лист %q не поддерживается; используйте листы из шаблона экспорта", name)
	}
	if len(rows[0]) > 16 {
		return nil, fmt.Errorf("лист %q содержит слишком много столбцов", name)
	}
	columnIndex := make(map[string]int)
	for i, header := range rows[0] {
		key := strings.TrimSpace(header)
		if key != "" {
			if _, exists := columnIndex[key]; exists {
				return nil, fmt.Errorf("на листе %q повторяется столбец «%s»", name, key)
			}
			columnIndex[key] = i
		}
	}
	for _, header := range headers {
		if _, ok := columnIndex[header]; !ok {
			return nil, fmt.Errorf("на листе %q отсутствует обязательный столбец «%s»", name, header)
		}
	}
	if name == "Ведомости поведения" && len(rows[0]) < 5 {
		return nil, fmt.Errorf("на листе «Ведомости поведения» добавьте столбцы критериев после первых четырёх колонок")
	}
	result := make([]workbookRecord, 0, len(rows)-1)
	for rowIndex, cells := range rows[1:] {
		values := make(map[string]string, len(rows[0]))
		nonEmpty := false
		for col, header := range rows[0] {
			key := strings.TrimSpace(header)
			if key == "" {
				continue
			}
			value := ""
			if col < len(cells) {
				value = strings.TrimSpace(cells[col])
			}
			if value != "" {
				nonEmpty = true
			}
			values[key] = value
		}
		if nonEmpty {
			if len(result) >= workbookMaxRows {
				return nil, fmt.Errorf("лист %q превышает лимит строк", name)
			}
			result = append(result, workbookRecord{row: rowIndex + 2, values: values})
		}
	}
	return result, nil
}

func requiredValue(values map[string]string, key string) (string, error) {
	value := strings.TrimSpace(values[key])
	if value == "" {
		return "", fmt.Errorf("столбец «%s» не может быть пустым", key)
	}
	return value, nil
}

func parseWorkbookDate(value string, required bool) (any, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if required {
			return nil, fmt.Errorf("дата обязательна")
		}
		return nil, nil
	}
	for _, layout := range []string{"2006-01-02", "02.01.2006", "2.1.2006", "01/02/2006", "2006-01-02T15:04:05Z07:00"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("2006-01-02"), nil
		}
	}
	if serial, err := strconv.ParseFloat(value, 64); err == nil && serial > 0 {
		parsed, err := excelize.ExcelDateToTime(serial, false)
		if err == nil {
			return parsed.Format("2006-01-02"), nil
		}
	}
	return nil, fmt.Errorf("неверный формат даты %q; используйте ГГГГ-ММ-ДД", value)
}

func execOne(ctx context.Context, tx *sql.Tx, query string, args ...any) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, query, args...).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("не удалось сохранить строку; проверьте связанные справочники, уникальность и даты")
	}
	return id, nil
}

func importWorkbookRecord(ctx context.Context, tx *sql.Tx, sheet string, v map[string]string, p identity.Principal) error {
	switch sheet {
	case "Учебные годы":
		name, err := requiredValue(v, "Название")
		if err != nil {
			return err
		}
		start, err := parseWorkbookDate(v["Начало"], true)
		if err != nil {
			return err
		}
		end, err := parseWorkbookDate(v["Окончание"], true)
		if err != nil {
			return err
		}
		current := strings.EqualFold(v["Текущий"], "true") || strings.EqualFold(v["Текущий"], "да") || v["Текущий"] == "1"
		if current {
			if _, err := tx.ExecContext(ctx, `UPDATE academic_years SET is_current=false WHERE school_id=$1 AND is_current AND name<>$2`, p.SchoolID, name); err != nil {
				return fmt.Errorf("не удалось переключить текущий учебный год")
			}
		}
		_, err = execOne(ctx, tx, `INSERT INTO academic_years(school_id,name,starts_on,ends_on,is_current) VALUES($1,$2,$3::date,$4::date,$5) ON CONFLICT(school_id,name) DO UPDATE SET starts_on=excluded.starts_on,ends_on=excluded.ends_on,is_current=excluded.is_current RETURNING id::text`, p.SchoolID, name, start, end, current)
		return err
	case "Классы":
		name, err := requiredValue(v, "Класс")
		if err != nil {
			return err
		}
		year, err := requiredValue(v, "Учебный год")
		if err != nil {
			return err
		}
		_, err = execOne(ctx, tx, `INSERT INTO classes(school_id,name,academic_year_id) SELECT $1,$2,y.id FROM academic_years y WHERE y.school_id=$1 AND y.name=$3 ON CONFLICT(academic_year_id,name) DO UPDATE SET name=excluded.name RETURNING id::text`, p.SchoolID, name, year)
		return err
	case "Предметы":
		name, err := requiredValue(v, "Предмет")
		if err != nil {
			return err
		}
		_, err = execOne(ctx, tx, `INSERT INTO subjects(school_id,name) VALUES($1,$2) ON CONFLICT(school_id,name) DO UPDATE SET name=excluded.name RETURNING id::text`, p.SchoolID, name)
		return err
	case "Периоды":
		name, err := requiredValue(v, "Период")
		if err != nil {
			return err
		}
		year, err := requiredValue(v, "Учебный год")
		if err != nil {
			return err
		}
		start, err := parseWorkbookDate(v["Начало"], true)
		if err != nil {
			return err
		}
		end, err := parseWorkbookDate(v["Окончание"], true)
		if err != nil {
			return err
		}
		_, err = execOne(ctx, tx, `INSERT INTO terms(school_id,academic_year_id,name,starts_on,ends_on) SELECT $1,y.id,$3,$4::date,$5::date FROM academic_years y WHERE y.school_id=$1 AND y.name=$2 ON CONFLICT(academic_year_id,name) DO UPDATE SET starts_on=excluded.starts_on,ends_on=excluded.ends_on RETURNING id::text`, p.SchoolID, year, name, start, end)
		return err
	case "Критерии поведения":
		return importBehaviorCriterion(ctx, tx, p, v)
	case "Проекты":
		name, err := requiredValue(v, "Проект")
		if err != nil {
			return err
		}
		_, err = execOne(ctx, tx, `INSERT INTO projects(school_id,name) VALUES($1,$2) ON CONFLICT(school_id,name) DO UPDATE SET name=excluded.name RETURNING id::text`, p.SchoolID, name)
		return err
	case "Ученики":
		externalID, err := requiredValue(v, "Табельный номер")
		if err != nil {
			return err
		}
		fullName, err := requiredValue(v, "ФИО")
		if err != nil {
			return err
		}
		birth, err := parseWorkbookDate(v["Дата рождения"], false)
		if err != nil {
			return err
		}
		studentID, err := execOne(ctx, tx, `INSERT INTO students(school_id,external_id,full_name,birth_date) VALUES($1,$2,$3,$4::date) ON CONFLICT(school_id,external_id) DO UPDATE SET full_name=excluded.full_name,birth_date=COALESCE(excluded.birth_date,students.birth_date) RETURNING id::text`, p.SchoolID, externalID, fullName, birth)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE users u SET full_name=$3 FROM students s WHERE s.id=$1 AND s.school_id=$2 AND s.user_id=u.id AND u.school_id=$2`, studentID, p.SchoolID, fullName); err != nil {
			return fmt.Errorf("не удалось обновить профиль ученика")
		}
		if strings.TrimSpace(v["Класс"]) != "" || strings.TrimSpace(v["Учебный год"]) != "" {
			if strings.TrimSpace(v["Класс"]) == "" || strings.TrimSpace(v["Учебный год"]) == "" {
				return fmt.Errorf("для зачисления укажите и класс, и учебный год")
			}
			return importEnrollment(ctx, tx, p, externalID, v["Класс"], v["Учебный год"], "active", v["Дата начала зачисления"], "")
		}
		_ = studentID
		return nil
	case "Зачисления":
		student, err := requiredValue(v, "Табельный номер")
		if err != nil {
			return err
		}
		class, err := requiredValue(v, "Класс")
		if err != nil {
			return err
		}
		year, err := requiredValue(v, "Учебный год")
		if err != nil {
			return err
		}
		status := strings.TrimSpace(v["Статус"])
		if status == "" {
			status = "active"
		}
		return importEnrollment(ctx, tx, p, student, class, year, status, v["Дата начала"], v["Дата окончания"])
	case "Оценки":
		return importGrade(ctx, tx, p, v)
	case "Проекты учеников":
		student, err := requiredValue(v, "Табельный номер")
		if err != nil {
			return err
		}
		project, err := requiredValue(v, "Проект")
		if err != nil {
			return err
		}
		year, err := requiredValue(v, "Учебный год")
		if err != nil {
			return err
		}
		_, err = execOne(ctx, tx, `INSERT INTO student_projects(student_id,project_id,academic_year_id,school_id) SELECT s.id,pj.id,y.id,s.school_id FROM students s JOIN projects pj ON pj.school_id=s.school_id AND pj.name=$3 JOIN academic_years y ON y.school_id=s.school_id AND y.name=$4 WHERE s.school_id=$1 AND s.external_id=$2 ON CONFLICT(student_id,project_id,academic_year_id) DO UPDATE SET school_id=excluded.school_id RETURNING student_id::text`, p.SchoolID, student, project, year)
		return err
	case "Назначения учителей":
		teacher, err := requiredValue(v, "Email учителя")
		if err != nil {
			return err
		}
		class, err := requiredValue(v, "Класс")
		if err != nil {
			return err
		}
		subject, err := requiredValue(v, "Предмет")
		if err != nil {
			return err
		}
		year, err := requiredValue(v, "Учебный год")
		if err != nil {
			return err
		}
		_, err = execOne(ctx, tx, `INSERT INTO teacher_assignments(teacher_id,class_id,subject_id,academic_year_id,school_id) SELECT u.id,c.id,s.id,y.id,u.school_id FROM users u JOIN user_roles ur ON ur.user_id=u.id AND ur.school_id=u.school_id JOIN roles r ON r.id=ur.role_id AND r.code='teacher' JOIN classes c ON c.school_id=u.school_id AND c.name=$3 JOIN subjects s ON s.school_id=u.school_id AND s.name=$4 JOIN academic_years y ON y.school_id=u.school_id AND y.name=$5 AND y.id=c.academic_year_id WHERE u.school_id=$1 AND u.email=$2 ON CONFLICT(teacher_id,class_id,subject_id,academic_year_id) DO UPDATE SET school_id=excluded.school_id RETURNING id::text`, p.SchoolID, teacher, class, subject, year)
		return err
	case "Классные руководители":
		teacher, err := requiredValue(v, "Email учителя")
		if err != nil {
			return err
		}
		class, err := requiredValue(v, "Класс")
		if err != nil {
			return err
		}
		year, err := requiredValue(v, "Учебный год")
		if err != nil {
			return err
		}
		_, err = execOne(ctx, tx, `INSERT INTO class_teacher_assignments(teacher_id,class_id,academic_year_id,school_id) SELECT u.id,c.id,y.id,u.school_id FROM users u JOIN user_roles ur ON ur.user_id=u.id AND ur.school_id=u.school_id JOIN roles r ON r.id=ur.role_id AND r.code='class_teacher' JOIN classes c ON c.school_id=u.school_id AND c.name=$3 JOIN academic_years y ON y.school_id=u.school_id AND y.name=$4 AND y.id=c.academic_year_id WHERE u.school_id=$1 AND u.email=$2 ON CONFLICT(class_id,academic_year_id) DO UPDATE SET teacher_id=excluded.teacher_id RETURNING id::text`, p.SchoolID, teacher, class, year)
		return err
	case "Права оценивать поведение":
		teacher, err := requiredValue(v, "Email учителя")
		if err != nil {
			return err
		}
		class, err := requiredValue(v, "Класс")
		if err != nil {
			return err
		}
		year, err := requiredValue(v, "Учебный год")
		if err != nil {
			return err
		}
		can := strings.EqualFold(v["Разрешено"], "true") || strings.EqualFold(v["Разрешено"], "да") || v["Разрешено"] == "1"
		_, err = execOne(ctx, tx, `INSERT INTO behavior_permissions(teacher_id,class_id,academic_year_id,can_score,school_id) SELECT u.id,c.id,y.id,$5,u.school_id FROM users u JOIN user_roles ur ON ur.user_id=u.id AND ur.school_id=u.school_id JOIN roles r ON r.id=ur.role_id AND r.code='teacher' JOIN classes c ON c.school_id=u.school_id AND c.name=$3 JOIN academic_years y ON y.school_id=u.school_id AND y.name=$4 AND y.id=c.academic_year_id WHERE u.school_id=$1 AND u.email=$2 ON CONFLICT(teacher_id,class_id,academic_year_id) DO UPDATE SET can_score=excluded.can_score RETURNING id::text`, p.SchoolID, teacher, class, year, can)
		return err
	case "Ведомости поведения":
		return importBehavior(ctx, tx, p, v)
	default:
		return fmt.Errorf("unsupported worksheet")
	}
}

func importEnrollment(ctx context.Context, tx *sql.Tx, p identity.Principal, externalID, class, year, status, fromDate, toDate string) error {
	if status != "active" && status != "transferred" && status != "completed" {
		return fmt.Errorf("статус должен быть active, transferred или completed")
	}
	from, err := parseWorkbookDate(fromDate, false)
	if err != nil {
		return err
	}
	to, err := parseWorkbookDate(toDate, false)
	if err != nil {
		return err
	}
	if fromText, fromOK := from.(string); fromOK {
		if toText, toOK := to.(string); toOK && toText < fromText {
			return fmt.Errorf("дата окончания не может быть раньше даты начала")
		}
	}
	if status == "active" {
		_, err = tx.ExecContext(ctx, `WITH destination AS (
			SELECT s.id AS student_id,c.id AS class_id,y.id AS year_id FROM students s
			JOIN classes c ON c.school_id=s.school_id AND c.name=$3
			JOIN academic_years y ON y.school_id=s.school_id AND y.name=$4 AND y.id=c.academic_year_id
			WHERE s.school_id=$1 AND s.external_id=$2
		) UPDATE enrollments e SET status='transferred',
			to_date=GREATEST(COALESCE($5::date,CURRENT_DATE),COALESCE(e.from_date,CURRENT_DATE))
		FROM destination d WHERE e.student_id=d.student_id AND e.academic_year_id=d.year_id
			AND e.status='active' AND e.class_id<>d.class_id`, p.SchoolID, externalID, class, year, from)
		if err != nil {
			return fmt.Errorf("не удалось обновить активное зачисление ученика")
		}
	}
	_, err = execOne(ctx, tx, `INSERT INTO enrollments(student_id,class_id,academic_year_id,status,from_date,to_date,school_id) SELECT s.id,c.id,y.id,$5,$6::date,$7::date,s.school_id FROM students s JOIN classes c ON c.school_id=s.school_id AND c.name=$3 JOIN academic_years y ON y.school_id=s.school_id AND y.name=$4 AND y.id=c.academic_year_id WHERE s.school_id=$1 AND s.external_id=$2 ON CONFLICT(class_id,student_id,academic_year_id) DO UPDATE SET status=excluded.status,from_date=COALESCE(excluded.from_date,enrollments.from_date),to_date=COALESCE(excluded.to_date,enrollments.to_date) RETURNING id::text`, p.SchoolID, externalID, class, year, status, from, to)
	return err
}

func importBehaviorCriterion(ctx context.Context, tx *sql.Tx, p identity.Principal, v map[string]string) error {
	code, err := requiredValue(v, "Код")
	if err != nil {
		return err
	}
	title, err := requiredValue(v, "Название")
	if err != nil {
		return err
	}
	maximum, err := strconv.Atoi(strings.TrimSpace(v["Макс. балл"]))
	if err != nil || maximum < 0 || maximum > 3 {
		return fmt.Errorf("максимальный балл должен быть целым числом от 0 до 3")
	}
	order, err := strconv.Atoi(strings.TrimSpace(v["Порядок"]))
	if err != nil || order < 1 || order > 10 {
		return fmt.Errorf("порядок критерия должен быть от 1 до 10")
	}
	var id string
	err = tx.QueryRowContext(ctx, `SELECT id::text FROM behavior_criteria WHERE school_id=$1 AND code=$2 FOR UPDATE`, p.SchoolID, code).Scan(&id)
	if err == sql.ErrNoRows {
		_, err = execOne(ctx, tx, `INSERT INTO behavior_criteria(school_id,code,title,max_score,sort_order) VALUES($1,$2,$3,$4,$5) RETURNING id::text`, p.SchoolID, code, title, maximum, order)
		return err
	}
	if err != nil {
		return fmt.Errorf("не удалось найти критерий %q", code)
	}
	var hasHigherScores bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM behavior_scores WHERE criterion_id=$1 AND school_id=$2 AND score>$3)`, id, p.SchoolID, maximum).Scan(&hasHigherScores)
	if err != nil || hasHigherScores {
		return fmt.Errorf("новый максимальный балл критерия %q ниже уже сохранённых баллов", code)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE behavior_criteria SET title=$3,max_score=$4,sort_order=$5 WHERE id=$1 AND school_id=$2`, id, p.SchoolID, title, maximum, order); err != nil {
		return fmt.Errorf("не удалось сохранить критерий %q; проверьте порядок", code)
	}
	return nil
}

func importGrade(ctx context.Context, tx *sql.Tx, p identity.Principal, v map[string]string) error {
	student, err := requiredValue(v, "Табельный номер")
	if err != nil {
		return err
	}
	subject, err := requiredValue(v, "Предмет")
	if err != nil {
		return err
	}
	class, err := requiredValue(v, "Класс")
	if err != nil {
		return err
	}
	year, err := requiredValue(v, "Учебный год")
	if err != nil {
		return err
	}
	term, err := requiredValue(v, "Период")
	if err != nil {
		return err
	}
	valueText, err := requiredValue(v, "Оценка")
	if err != nil {
		return err
	}
	value, err := strconv.Atoi(valueText)
	if err != nil || value < 1 || value > 5 {
		return fmt.Errorf("оценка должна быть целым числом от 1 до 5")
	}
	date, err := parseWorkbookDate(v["Дата"], true)
	if err != nil {
		return err
	}
	kind := strings.TrimSpace(v["Тип оценки"])
	if kind == "" {
		kind = "regular"
	}
	_, err = execOne(ctx, tx, `INSERT INTO grades(student_id,subject_id,class_id,term_id,value,grade_type,work_date,comment,created_by,school_id,academic_year_id)
		SELECT s.id,su.id,c.id,t.id,$7,$8,$9::date,NULLIF($10,''),$11,s.school_id,y.id FROM students s JOIN subjects su ON su.school_id=s.school_id AND su.name=$3 JOIN classes c ON c.school_id=s.school_id AND c.name=$4 JOIN academic_years y ON y.school_id=s.school_id AND y.name=$5 AND y.id=c.academic_year_id JOIN terms t ON t.school_id=s.school_id AND t.name=$6 AND t.academic_year_id=y.id WHERE s.school_id=$1 AND s.external_id=$2 AND EXISTS(SELECT 1 FROM enrollments e WHERE e.student_id=s.id AND e.class_id=c.id AND e.academic_year_id=y.id AND e.school_id=s.school_id)
		ON CONFLICT(student_id,subject_id,term_id,grade_type,work_date) WHERE deleted_at IS NULL DO UPDATE SET value=excluded.value,comment=COALESCE(excluded.comment,grades.comment),updated_at=now() RETURNING id::text`, p.SchoolID, student, subject, class, year, term, value, kind, date, strings.TrimSpace(v["Комментарий"]), p.UserID)
	return err
}

func importBehavior(ctx context.Context, tx *sql.Tx, p identity.Principal, v map[string]string) error {
	student, err := requiredValue(v, "Табельный номер")
	if err != nil {
		return err
	}
	class, err := requiredValue(v, "Класс")
	if err != nil {
		return err
	}
	year, err := requiredValue(v, "Учебный год")
	if err != nil {
		return err
	}
	term, err := requiredValue(v, "Период")
	if err != nil {
		return err
	}
	var sheetID string
	err = tx.QueryRowContext(ctx, `INSERT INTO behavior_sheets(class_id,student_id,academic_year_id,term_id,total_score,is_complete,verdict,updated_by,school_id) SELECT c.id,s.id,y.id,t.id,0,false,NULL,$6,s.school_id FROM students s JOIN classes c ON c.school_id=s.school_id AND c.name=$3 JOIN academic_years y ON y.school_id=s.school_id AND y.name=$4 AND y.id=c.academic_year_id JOIN terms t ON t.school_id=s.school_id AND t.name=$5 AND t.academic_year_id=y.id WHERE s.school_id=$1 AND s.external_id=$2 AND EXISTS(SELECT 1 FROM enrollments e WHERE e.student_id=s.id AND e.class_id=c.id AND e.academic_year_id=y.id AND e.school_id=s.school_id) ON CONFLICT(student_id,academic_year_id,term_id) DO UPDATE SET class_id=excluded.class_id,updated_by=excluded.updated_by,updated_at=now(),deleted_at=NULL RETURNING id::text`, p.SchoolID, student, class, year, term, p.UserID).Scan(&sheetID)
	if err != nil {
		return fmt.Errorf("не удалось найти ученика, класс, период или его зачисление")
	}
	criteriaRows, err := tx.QueryContext(ctx, `SELECT id::text,code,max_score FROM behavior_criteria WHERE school_id=$1 ORDER BY sort_order`, p.SchoolID)
	if err != nil {
		return err
	}
	type criterionRow struct {
		id, code string
		max      int
	}
	criteria := make([]criterionRow, 0, 10)
	for criteriaRows.Next() {
		var id, code string
		var max int
		if err := criteriaRows.Scan(&id, &code, &max); err != nil {
			criteriaRows.Close()
			return err
		}
		criteria = append(criteria, criterionRow{id: id, code: code, max: max})
	}
	if err := criteriaRows.Err(); err != nil {
		criteriaRows.Close()
		return err
	}
	if err := criteriaRows.Close(); err != nil {
		return err
	}
	if len(criteria) != 10 {
		return fmt.Errorf("для импорта ведомостей должны быть настроены 10 критериев поведения")
	}
	for _, criterion := range criteria {
		id, code, max := criterion.id, criterion.code, criterion.max
		raw, exists := v[code]
		if !exists {
			return fmt.Errorf("в ведомости отсутствует столбец критерия %q", code)
		}
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(raw), "УДАЛИТЬ") {
			if _, err := tx.ExecContext(ctx, `DELETE FROM behavior_scores WHERE behavior_sheet_id=$1 AND criterion_id=$2`, sheetID, id); err != nil {
				return err
			}
			continue
		}
		score, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || score < 0 || score > max {
			return fmt.Errorf("балл для критерия %q должен быть от 0 до %d", code, max)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO behavior_scores(behavior_sheet_id,criterion_id,score,school_id) VALUES($1,$2,$3,$4) ON CONFLICT(behavior_sheet_id,criterion_id) DO UPDATE SET score=excluded.score`, sheetID, id, score, p.SchoolID); err != nil {
			return fmt.Errorf("не удалось сохранить балл для критерия %q", code)
		}
	}
	_, err = tx.ExecContext(ctx, `WITH totals AS (SELECT bs.id,COALESCE(sum(sc.score),0)::smallint AS total,count(sc.id)=10 AND (SELECT count(*) FROM behavior_criteria WHERE school_id=$2)=10 AS complete FROM behavior_sheets bs LEFT JOIN behavior_scores sc ON sc.behavior_sheet_id=bs.id WHERE bs.id=$1 GROUP BY bs.id) UPDATE behavior_sheets bs SET total_score=totals.total,is_complete=totals.complete,verdict=CASE WHEN totals.complete THEN CASE WHEN totals.total>=24 THEN 'образцовое' WHEN totals.total>=10 THEN 'допустимое' ELSE 'недопустимое' END ELSE NULL END,updated_at=now() FROM totals WHERE bs.id=totals.id`, sheetID, p.SchoolID)
	return err
}
