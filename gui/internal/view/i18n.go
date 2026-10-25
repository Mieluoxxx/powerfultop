package view

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/i18n"
)

// L picks the English or Simplified Chinese text for the language the app shows. Pages say their
// text in place, as the Swift app's Localizable.strings pairs did; the catalogs in locales/ only
// declare the app's languages, so MyGo's own controls follow them.
func L(en, zh string) string {
	if strings.HasPrefix(i18n.Locale(), "zh") {
		return zh
	}
	return en
}

// Lf formats the text L picks.
func Lf(en, zh string, args ...any) string {
	return fmt.Sprintf(L(en, zh), args...)
}
