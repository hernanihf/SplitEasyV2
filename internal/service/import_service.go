package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"

	"spliteasy/internal/domain"
	"spliteasy/internal/repository"
)

// Sentinel errors that handlers map to HTTP status codes via errors.Is.
var (
	ErrGroupNotEmpty = errors.New("group already has expenses or settlements — import is only for a brand-new group")
	ErrInvalidCSV    = errors.New("could not read this as a Splitwise-style expense CSV")
)

// ImportResult reports what happened after committing a parsed import —
// row-level failures (an unmapped column, a malformed amount) don't abort
// the rest of the batch, so this is a count rather than an all-or-nothing
// success/failure.
type ImportResult struct {
	Imported int
	Failed   int
}

type ImportService interface {
	// ParsePreview parses a Splitwise-style CSV export into a preview —
	// purely in-memory, no DB writes. Rows in a currency other than the
	// group's, or that otherwise can't be parsed, are counted in
	// SkippedRows rather than failing the whole upload.
	ParsePreview(ctx context.Context, groupID uint, file io.Reader) (*domain.ImportPreview, error)
	// Import creates one expense per row, resolving payer and splits from
	// each row's per-member net cents via memberMapping (CSV column name →
	// group member's user ID). Only allowed when the group has no expenses
	// or settlements yet (see ErrGroupNotEmpty) — this is a one-time
	// "migrate my history" action, not an ongoing sync.
	Import(ctx context.Context, groupID, callerID uint, rows []domain.ImportRow, memberMapping map[string]uint) (ImportResult, error)
	// ExportGroupCSV builds a CSV of every expense and settlement in the
	// group in the same [Date, Description, Category, Cost, Currency,
	// ...one column per member...] shape ParsePreview reads — so it opens
	// directly in Splitwise, and a group exported here round-trips back in
	// through this app's own importer. filename is derived from the
	// group's name for the download's Content-Disposition header.
	ExportGroupCSV(ctx context.Context, groupID uint) (data []byte, filename string, err error)
	// ExportSpendingXLSX builds a two-sheet workbook for the group's
	// non-deleted expenses within [from, to] (either bound may be nil,
	// meaning unbounded): a "Spending" sheet with one row per category
	// (total + share of the filtered total) and a native pie chart built
	// from it, and a "Details" sheet listing every expense in that
	// category order. filename is derived from the group's name and the
	// filter's bounds.
	ExportSpendingXLSX(ctx context.Context, groupID uint, from, to *time.Time) (data []byte, filename string, err error)
}

type importService struct {
	groupRepo      repository.GroupRepository
	expenseRepo    repository.ExpenseRepository
	settlementRepo repository.SettlementRepository
}

func NewImportService(groupRepo repository.GroupRepository, expenseRepo repository.ExpenseRepository, settlementRepo repository.SettlementRepository) ImportService {
	return &importService{groupRepo, expenseRepo, settlementRepo}
}

// csvDateLayout matches Splitwise's export format (e.g. "2023-09-24"),
// locale-independent since it's numeric.
const csvDateLayout = "2006-01-02"

func (s *importService) ParsePreview(ctx context.Context, groupID uint, file io.Reader) (*domain.ImportPreview, error) {
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, ErrGroupNotFound
	}

	reader := csv.NewReader(file)
	// Splitwise's own export has a trailing blank line and a short
	// "total balance" summary row with empty category/cost fields — tolerate
	// ragged rows here and filter them out below, instead of failing the
	// whole read on the first short row.
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, ErrInvalidCSV
	}
	if len(records) < 2 || len(records[0]) < 6 {
		return nil, ErrInvalidCSV
	}

	// Columns are always [Date, Description, Category, Cost, Currency, ...one
	// per group member...] in that fixed order — true regardless of which
	// language Splitwise exported the header row in, so this matches on
	// position rather than trying to recognize header text in every locale.
	memberColumns := make([]string, len(records[0])-5)
	for i, name := range records[0][5:] {
		memberColumns[i] = fixMojibake(strings.TrimSpace(name))
	}

	preview := &domain.ImportPreview{MemberColumns: memberColumns}
	var mismatchedCurrency string
	for _, rec := range records[1:] {
		if len(rec) < 6 {
			preview.SkippedRows++
			continue
		}
		date, err := time.Parse(csvDateLayout, strings.TrimSpace(rec[0]))
		if err != nil {
			preview.SkippedRows++
			continue
		}
		if rowCurrency := strings.TrimSpace(rec[4]); !strings.EqualFold(rowCurrency, group.Currency) {
			preview.SkippedRows++
			if mismatchedCurrency == "" {
				mismatchedCurrency = strings.ToUpper(rowCurrency)
			}
			continue
		}
		amountCents, err := parseCents(rec[3])
		if err != nil || amountCents == 0 {
			preview.SkippedRows++
			continue
		}

		nets := make(map[string]int64, len(memberColumns))
		for i, col := range memberColumns {
			idx := 5 + i
			if idx >= len(rec) {
				continue
			}
			cents, err := parseCents(rec[idx])
			if err != nil {
				continue
			}
			nets[col] = cents
		}

		preview.Rows = append(preview.Rows, domain.ImportRow{
			Date:        date,
			Description: fixMojibake(strings.TrimSpace(rec[1])),
			Category:    mapSplitwiseCategory(rec[2]),
			AmountCents: amountCents,
			MemberNets:  nets,
		})
	}

	if len(preview.Rows) == 0 && mismatchedCurrency != "" {
		preview.CurrencyMismatch = mismatchedCurrency
	}

	return preview, nil
}

func (s *importService) Import(ctx context.Context, groupID, callerID uint, rows []domain.ImportRow, memberMapping map[string]uint) (ImportResult, error) {
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return ImportResult{}, ErrGroupNotFound
	}
	if !isMember(group, callerID) {
		return ImportResult{}, ErrNotGroupMember
	}

	// Every mapped user must actually belong to the group — otherwise a
	// crafted mapping could attribute historical expenses (and therefore
	// balances) to an outsider.
	memberSet := make(map[uint]bool, len(group.Members))
	for _, m := range group.Members {
		memberSet[m.ID] = true
	}
	for _, uid := range memberMapping {
		if !memberSet[uid] {
			return ImportResult{}, errors.New("mapped user is not a member of this group")
		}
	}

	existingExpenses, err := s.expenseRepo.GetByGroupID(ctx, groupID)
	if err != nil {
		return ImportResult{}, err
	}
	existingSettlements, err := s.settlementRepo.GetByGroupID(ctx, groupID)
	if err != nil {
		return ImportResult{}, err
	}
	if len(existingExpenses) > 0 || len(existingSettlements) > 0 {
		return ImportResult{}, ErrGroupNotEmpty
	}

	var result ImportResult
	for _, row := range rows {
		splits, payerID, ok := resolveSplits(row, memberMapping)
		if !ok {
			result.Failed++
			continue
		}

		expense := &domain.Expense{
			GroupID:     groupID,
			PaidByID:    payerID,
			Description: row.Description,
			Category:    row.Category,
			Amount:      row.AmountCents,
			CreatedAt:   row.Date,
		}
		if err := s.expenseRepo.CreateWithSplits(ctx, expense, splits, nil); err != nil {
			result.Failed++
			continue
		}
		result.Imported++
	}

	return result, nil
}

// ExportGroupCSV lays out each expense/settlement's per-member effect on
// their own column — the same net-cents shape resolveSplits reconstructs a
// payer and splits from — so the two are inverses of each other.
func (s *importService) ExportGroupCSV(ctx context.Context, groupID uint) ([]byte, string, error) {
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, "", ErrGroupNotFound
	}

	expenses, err := s.expenseRepo.GetByGroupID(ctx, groupID)
	if err != nil {
		return nil, "", err
	}
	settlements, err := s.settlementRepo.GetByGroupID(ctx, groupID)
	if err != nil {
		return nil, "", err
	}

	memberNames := make([]string, len(group.Members))
	memberIndex := make(map[uint]int, len(group.Members))
	for i, m := range group.Members {
		memberNames[i] = m.Name
		memberIndex[m.ID] = i
	}

	type exportRow struct {
		date        time.Time
		description string
		category    string
		amountCents int64
		nets        []int64
	}

	rows := make([]exportRow, 0, len(expenses)+len(settlements))
	totals := make([]int64, len(memberNames))

	for _, e := range expenses {
		nets := make([]int64, len(memberNames))
		if idx, ok := memberIndex[e.PaidByID]; ok {
			nets[idx] += e.Amount
		}
		for _, sp := range e.Splits {
			if idx, ok := memberIndex[sp.UserID]; ok {
				nets[idx] -= sp.Amount
			}
		}
		for i, n := range nets {
			totals[i] += n
		}
		rows = append(rows, exportRow{
			date:        e.CreatedAt,
			description: e.Description,
			category:    splitwiseCategoryName(e.Category),
			amountCents: e.Amount,
			nets:        nets,
		})
	}

	for _, st := range settlements {
		nets := make([]int64, len(memberNames))
		if idx, ok := memberIndex[st.FromUserID]; ok {
			nets[idx] += st.Amount
		}
		if idx, ok := memberIndex[st.ToUserID]; ok {
			nets[idx] -= st.Amount
		}
		for i, n := range nets {
			totals[i] += n
		}
		rows = append(rows, exportRow{
			date:        st.CreatedAt,
			description: "Payment",
			category:    "Payment",
			amountCents: st.Amount,
			nets:        nets,
		})
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].date.Before(rows[j].date) })

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	header := append([]string{"Date", "Description", "Category", "Cost", "Currency"}, memberNames...)
	if err := w.Write(header); err != nil {
		return nil, "", err
	}

	// Splitwise's own export leads with this summary row — Date/Category/
	// Cost/Currency blank, just each member's running balance — which is
	// also exactly why ParsePreview above tolerates ragged/short rows.
	totalRow := append([]string{"", "Total balance", "", "", ""}, formatCentsList(totals)...)
	if err := w.Write(totalRow); err != nil {
		return nil, "", err
	}

	for _, r := range rows {
		rec := append([]string{
			r.date.Format(csvDateLayout),
			r.description,
			r.category,
			formatCents(r.amountCents),
			group.Currency,
		}, formatCentsList(r.nets)...)
		if err := w.Write(rec); err != nil {
			return nil, "", err
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, "", err
	}

	return buf.Bytes(), exportFilename(group.Name), nil
}

func (s *importService) ExportSpendingXLSX(ctx context.Context, groupID uint, from, to *time.Time) ([]byte, string, error) {
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, "", ErrGroupNotFound
	}

	expenses, err := s.expenseRepo.GetByGroupID(ctx, groupID)
	if err != nil {
		return nil, "", err
	}

	type spendingRow struct {
		category    string
		description string
		date        time.Time
		amountCents int64
	}
	rows := make([]spendingRow, 0, len(expenses))
	for _, e := range expenses {
		if from != nil && e.CreatedAt.Before(*from) {
			continue
		}
		if to != nil && e.CreatedAt.After(*to) {
			continue
		}
		category := e.Category
		if category == "" {
			category = "other"
		}
		rows = append(rows, spendingRow{category, e.Description, e.CreatedAt, e.Amount})
	}

	// Category totals, largest first — same ordering the app's own pie
	// chart uses, so the workbook's chart legend matches what the user
	// already saw on screen.
	totals := map[string]int64{}
	var order []string
	for _, r := range rows {
		if _, seen := totals[r.category]; !seen {
			order = append(order, r.category)
		}
		totals[r.category] += r.amountCents
	}
	sort.Slice(order, func(i, j int) bool { return totals[order[i]] > totals[order[j]] })
	categoryRank := make(map[string]int, len(order))
	for i, c := range order {
		categoryRank[c] = i
	}
	var grandTotal int64
	for _, t := range totals {
		grandTotal += t
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	// Renames excelize's default sheet rather than adding a new one, so the
	// workbook doesn't open with a stray empty "Sheet1" tab.
	const summarySheet = "Spending"
	if err := f.SetSheetName("Sheet1", summarySheet); err != nil {
		return nil, "", err
	}
	if err := f.SetSheetRow(summarySheet, "A1", &[]interface{}{"Category", "Total", "Percent"}); err != nil {
		return nil, "", err
	}
	for i, cat := range order {
		pct := 0.0
		if grandTotal > 0 {
			// Rounded to 1 decimal — the raw division is an ugly repeating
			// decimal (e.g. 71.42857142857143) that Excel's General format
			// would otherwise show in full.
			pct = math.Round(float64(totals[cat])/float64(grandTotal)*1000) / 10
		}
		cell := fmt.Sprintf("A%d", i+2)
		if err := f.SetSheetRow(summarySheet, cell, &[]interface{}{
			categoryDisplayName(cat),
			float64(totals[cat]) / 100,
			pct,
		}); err != nil {
			return nil, "", err
		}
	}

	if len(order) > 0 {
		lastRow := len(order) + 1
		if err := f.AddChart(summarySheet, "E2", &excelize.Chart{
			Type: excelize.Pie,
			Series: []excelize.ChartSeries{{
				Name:       summarySheet + "!$B$1",
				Categories: fmt.Sprintf("%s!$A$2:$A$%d", summarySheet, lastRow),
				Values:     fmt.Sprintf("%s!$B$2:$B$%d", summarySheet, lastRow),
			}},
			Title: excelize.ChartTitle{
				Paragraph: []excelize.RichTextRun{{Text: "Spending by category"}},
			},
			Legend:   excelize.ChartLegend{Position: "right"},
			PlotArea: excelize.ChartPlotArea{ShowPercent: true},
		}); err != nil {
			return nil, "", err
		}
	}

	const detailSheet = "Details"
	if _, err := f.NewSheet(detailSheet); err != nil {
		return nil, "", err
	}
	if err := f.SetSheetRow(detailSheet, "A1", &[]interface{}{"Date", "Category", "Description", "Amount"}); err != nil {
		return nil, "", err
	}
	sort.Slice(rows, func(i, j int) bool {
		if categoryRank[rows[i].category] != categoryRank[rows[j].category] {
			return categoryRank[rows[i].category] < categoryRank[rows[j].category]
		}
		return rows[i].date.After(rows[j].date)
	})
	for i, r := range rows {
		cell := fmt.Sprintf("A%d", i+2)
		if err := f.SetSheetRow(detailSheet, cell, &[]interface{}{
			r.date.Format(csvDateLayout),
			categoryDisplayName(r.category),
			r.description,
			float64(r.amountCents) / 100,
		}); err != nil {
			return nil, "", err
		}
	}

	// Spending kept sheet index 0 (renamed rather than replaced above), so
	// it's already what the workbook opens to — no explicit activation needed.

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), exportSpendingFilename(group.Name, from, to), nil
}

// categoryDisplayName title-cases a category slug (e.g. "food" -> "Food")
// for a workbook that has no access to the app's own localized category
// names, which only live in the frontend's translations.
func categoryDisplayName(slug string) string {
	if slug == "" {
		slug = "other"
	}
	return strings.ToUpper(slug[:1]) + slug[1:]
}

// formatCents renders integer cents as a fixed 2-decimal string (e.g. -1050
// -> "-10.50"), the inverse of parseCents.
func formatCents(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	s := fmt.Sprintf("%d.%02d", cents/100, cents%100)
	if neg {
		s = "-" + s
	}
	return s
}

func formatCentsList(values []int64) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = formatCents(v)
	}
	return out
}

// asciiSlug lowercases a name down to just [a-z0-9-], collapsing spaces/
// underscores into hyphens — shared by every export filename below so a
// Content-Disposition header never has to worry about unicode or
// punctuation. Falls back to "group" if nothing alphanumeric survives.
func asciiSlug(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('-')
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "group"
	}
	return slug
}

// exportFilename turns a group name into a safe ASCII CSV filename for the
// download's Content-Disposition header, falling back to a generic name if
// the group name has nothing but punctuation/accents to offer.
func exportFilename(groupName string) string {
	return asciiSlug(groupName) + "-splitwise.csv"
}

// exportSpendingFilename names the spending workbook after the group and,
// when the filter is bounded, the date range it covers.
func exportSpendingFilename(groupName string, from, to *time.Time) string {
	name := asciiSlug(groupName) + "-spending"
	if from != nil || to != nil {
		fromStr, toStr := "start", "end"
		if from != nil {
			fromStr = from.Format(csvDateLayout)
		}
		if to != nil {
			toStr = to.Format(csvDateLayout)
		}
		name += "-" + fromStr + "-to-" + toStr
	}
	return name + ".xlsx"
}

// splitwiseCategoryName reverse-maps one of our category slugs back to a
// Splitwise category name — chosen so re-running it through
// mapSplitwiseCategory recovers the same slug, except "coffee" (a slug of
// ours Splitwise has no equivalent for) which falls back to "food".
func splitwiseCategoryName(slug string) string {
	if name, ok := splitwiseCategoryNameBySlug[slug]; ok {
		return name
	}
	return "General"
}

var splitwiseCategoryNameBySlug = map[string]string{
	"food":          "Dining out",
	"groceries":     "Groceries",
	"coffee":        "Food and drink - other",
	"drinks":        "Liquor",
	"transport":     "Other transportation",
	"fuel":          "Gas/fuel",
	"travel":        "Plane",
	"accommodation": "Hotel",
	"housing":       "Rent",
	"utilities":     "Other utilities",
	"internet":      "TV/Phone/Internet",
	"entertainment": "Other entertainment",
	"sports":        "Sports",
	"shopping":      "Clothing",
	"health":        "Medical expenses",
	"education":     "Education",
	"gifts":         "Gifts",
	"pets":          "Pets",
	"household":     "Household supplies",
	"other":         "General",
}

// resolveSplits turns one row's per-column net cents into a payer and a
// fixed-amount split list. Whoever has the largest net paid the expense (the
// overwhelmingly common single-payer case, and the only one Splitwise's own
// export — which only records each person's net, not who actually paid —
// lets us reconstruct); everyone else's share is simply the negation of
// their own net. ok is false if the payer's column isn't in memberMapping,
// since an expense with no resolvable payer can't be created.
func resolveSplits(row domain.ImportRow, memberMapping map[string]uint) (splits []domain.ExpenseSplit, payerID uint, ok bool) {
	payerCol := ""
	var payerNet int64 = -1 << 62
	for col, net := range row.MemberNets {
		if net > payerNet {
			payerCol, payerNet = col, net
		}
	}
	payerID, ok = memberMapping[payerCol]
	if !ok {
		return nil, 0, false
	}

	var shareSum int64
	for col, net := range row.MemberNets {
		uid, mapped := memberMapping[col]
		if !mapped {
			continue
		}
		var share int64
		if col == payerCol {
			share = row.AmountCents - net
		} else {
			share = -net
		}
		shareSum += share
		if share > 0 {
			splits = append(splits, domain.ExpenseSplit{UserID: uid, Amount: share})
		}
	}

	// Reconcile any 1-cent rounding drift (from Splitwise's own historical
	// rounding, not ours) onto the payer's share, so splits always sum to
	// exactly the total the way every other split method in this app already
	// guarantees.
	if diff := row.AmountCents - shareSum; diff != 0 {
		reconciled := false
		for i := range splits {
			if splits[i].UserID == payerID {
				splits[i].Amount += diff
				reconciled = true
				break
			}
		}
		if !reconciled && diff > 0 {
			splits = append(splits, domain.ExpenseSplit{UserID: payerID, Amount: diff})
		}
	}

	return splits, payerID, true
}

// fixMojibake repairs the specific "UTF-8 bytes misread as Latin-1/
// Windows-1252, then re-encoded to UTF-8" corruption (e.g. "Hernán" arriving
// as "HernÃ¡n") that some client/OS text-handling paths introduce on file
// upload — encoding/csv itself is charset-agnostic and never does this. If
// every rune fits in a byte (Latin-1's range), taking each rune's low byte
// and re-decoding that as UTF-8 recovers the original text; if the result
// isn't valid UTF-8, or any rune falls outside that range, it was never
// mis-decoded in the first place and s is returned unchanged.
func fixMojibake(s string) string {
	bs := make([]byte, 0, len(s))
	for _, r := range s {
		if r > 0xFF {
			return s
		}
		bs = append(bs, byte(r)) // #nosec G115 -- r is checked <= 0xFF above and range never yields negative runes
	}
	if utf8.Valid(bs) {
		return string(bs)
	}
	return s
}

// parseCents parses a decimal amount string like "4000.00" or "-2000.00"
// into integer cents, without float64's rounding risk. Splitwise always
// exports exactly 2 decimal places, but this tolerates 0-2.
func parseCents(raw string) (int64, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, errors.New("empty amount")
	}
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	} else if strings.HasPrefix(s, "+") {
		s = s[1:]
	}

	intPart, fracPart, hasFrac := strings.Cut(s, ".")
	if intPart == "" {
		intPart = "0"
	}
	whole, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, err
	}

	var frac int64
	if hasFrac {
		switch len(fracPart) {
		case 0:
			frac = 0
		case 1:
			f, err := strconv.ParseInt(fracPart, 10, 64)
			if err != nil {
				return 0, err
			}
			frac = f * 10
		default:
			f, err := strconv.ParseInt(fracPart[:2], 10, 64)
			if err != nil {
				return 0, err
			}
			frac = f
		}
	}

	cents := whole*100 + frac
	if neg {
		cents = -cents
	}
	return cents, nil
}

// mapSplitwiseCategory maps a Splitwise category name (Spanish or English —
// Splitwise exports in whatever locale the account is set to) to one of our
// slugs, falling back to "other" for anything unrecognized rather than
// failing the row.
func mapSplitwiseCategory(raw string) string {
	key := strings.ToLower(strings.TrimSpace(raw))
	if slug, ok := splitwiseCategoryMap[key]; ok {
		return slug
	}
	return "other"
}

var splitwiseCategoryMap = map[string]string{
	// Comida y bebida / Food and drink
	"alimentos":                "groceries",
	"comestibles":              "groceries",
	"groceries":                "groceries",
	"restaurantes":             "food",
	"dining out":               "food",
	"comidas y bebidas - otro": "food",
	"food and drink - other":   "food",
	"food and drink":           "food",
	"licor":                    "drinks",
	"liquor":                   "drinks",
	// Casa / Home
	"electrodomesticos":         "shopping",
	"electrónica":               "shopping",
	"electronics":               "shopping",
	"muebles":                   "household",
	"furniture":                 "household",
	"suministros para el hogar": "household",
	"household supplies":        "household",
	"mantenimiento":             "household",
	"maintenance":               "household",
	"hipoteca":                  "housing",
	"mortgage":                  "housing",
	"mascotas":                  "pets",
	"pets":                      "pets",
	"alquiler":                  "housing",
	"rent":                      "housing",
	"servicios":                 "utilities",
	"services":                  "utilities",
	"casa - otro":               "household",
	"other home":                "household",
	"casa":                      "household",
	"home":                      "household",
	// Vida / Life
	"cuidado de niños": "other",
	"childcare":        "other",
	"ropa":             "shopping",
	"clothing":         "shopping",
	"educacion":        "education",
	"educación":        "education",
	"education":        "education",
	"regalos":          "gifts",
	"gifts":            "gifts",
	"seguro":           "other",
	"insurance":        "other",
	"gastos médicos":   "health",
	"gastos medicos":   "health",
	"medical expenses": "health",
	"impuestos":        "other",
	"taxes":            "other",
	"vida - otro":      "other",
	"other life":       "other",
	"vida":             "other",
	"life":             "other",
	// Transporte / Transportation
	"bicicleta":            "transport",
	"bicycle":              "transport",
	"autobús/tren":         "transport",
	"autobus/tren":         "transport",
	"bus/train":            "transport",
	"coche":                "transport",
	"car":                  "transport",
	"gasolina":             "fuel",
	"gas/fuel":             "fuel",
	"hotel":                "accommodation",
	"estacionamiento":      "transport",
	"parking":              "transport",
	"avión":                "travel",
	"avion":                "travel",
	"plane":                "travel",
	"taxi":                 "transport",
	"transporte - otro":    "transport",
	"other transportation": "transport",
	"transporte":           "transport",
	"transportation":       "transport",
	// Servicios públicos / Utilities
	"limpieza":                  "household",
	"cleaning":                  "household",
	"electricidad":              "utilities",
	"electricity":               "utilities",
	"calefaccion/gas":           "utilities",
	"calefacción/gas":           "utilities",
	"heat/gas":                  "utilities",
	"basura":                    "utilities",
	"trash":                     "utilities",
	"tv/teléfono/internet":      "internet",
	"tv/telefono/internet":      "internet",
	"tv/phone/internet":         "internet",
	"agua":                      "utilities",
	"water":                     "utilities",
	"servicios públicos - otro": "utilities",
	"other utilities":           "utilities",
	// Entretenimiento / Entertainment
	"juegos":                 "entertainment",
	"games":                  "entertainment",
	"películas":              "entertainment",
	"peliculas":              "entertainment",
	"movies":                 "entertainment",
	"música":                 "entertainment",
	"musica":                 "entertainment",
	"music":                  "entertainment",
	"deportes":               "sports",
	"sports":                 "sports",
	"entretenimiento - otro": "entertainment",
	"other entertainment":    "entertainment",
	"entretenimiento":        "entertainment",
	"entertainment":          "entertainment",
	// Sin categorizar / catch-all
	"general":       "other",
	"uncategorized": "other",
}
