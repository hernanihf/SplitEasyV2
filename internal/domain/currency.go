package domain

// CurrencyCodes is the fixed set of currencies a group can be created in,
// in the display order the frontend shows them. The backend stores and
// validates the ISO 4217 code only; symbols and localized names live in the
// frontend.
var CurrencyCodes = []string{
	"USD",
	"ARS",
	"EUR",
	"BRL",
	"CLP",
	"UYU",
	"MXN",
	"COP",
}

// DefaultCurrency is used when a request doesn't specify a currency.
const DefaultCurrency = "USD"

var currencyCodeSet = func() map[string]bool {
	set := make(map[string]bool, len(CurrencyCodes))
	for _, code := range CurrencyCodes {
		set[code] = true
	}
	return set
}()

func IsValidCurrency(code string) bool {
	return currencyCodeSet[code]
}
