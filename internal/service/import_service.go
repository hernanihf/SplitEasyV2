package service

import (
	"context"
	"encoding/csv"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

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
		memberColumns[i] = strings.TrimSpace(name)
	}

	preview := &domain.ImportPreview{MemberColumns: memberColumns}
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
		if !strings.EqualFold(strings.TrimSpace(rec[4]), group.Currency) {
			preview.SkippedRows++
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
			Description: strings.TrimSpace(rec[1]),
			Category:    mapSplitwiseCategory(rec[2]),
			AmountCents: amountCents,
			MemberNets:  nets,
		})
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
