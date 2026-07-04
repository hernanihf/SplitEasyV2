package domain

// CurrencyCodes is the fixed set of currencies a group can be created in,
// in the display order the frontend shows them. The backend stores and
// validates the ISO 4217 code only; symbols and localized names live in the
// frontend.
var CurrencyCodes = []string{
	"USD",
	"ARS",
	"BRL",
	"MXN",
	"EUR",
}

// DefaultCurrency is used when a request doesn't specify a currency, and as
// the fallback in currencyFromLocale (auth_service.go) when a Google
// account's locale doesn't map to a known currency — SplitEasy's actual user
// base is Argentine, so that's a much more useful guess than USD.
const DefaultCurrency = "ARS"

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
