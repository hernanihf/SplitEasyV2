package domain

// ExpenseCategorySlugs is the fixed set of categories an expense can have,
// in the display order the frontend shows them. The backend stores and
// validates the slug only; human-readable (localized) names and the emoji
// for each slug live in the frontend.
var ExpenseCategorySlugs = []string{
	"food",
	"groceries",
	"coffee",
	"drinks",
	"transport",
	"fuel",
	"travel",
	"accommodation",
	"housing",
	"utilities",
	"internet",
	"entertainment",
	"sports",
	"shopping",
	"health",
	"education",
	"gifts",
	"pets",
	"household",
	"other",
}

// DefaultExpenseCategory is used when a request doesn't specify a category
// (and is the migration default for expenses that predate categories).
const DefaultExpenseCategory = "other"

var expenseCategorySet = func() map[string]bool {
	set := make(map[string]bool, len(ExpenseCategorySlugs))
	for _, slug := range ExpenseCategorySlugs {
		set[slug] = true
	}
	return set
}()

func IsValidExpenseCategory(slug string) bool {
	return expenseCategorySet[slug]
}
