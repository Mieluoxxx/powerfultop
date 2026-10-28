// Package lang picks the English or Simplified Chinese text for the language the app shows. Text
// is said in place, as the Swift app's Localizable.strings pairs did; the catalogs in locales/ only
// declare the app's languages, so MyGo's own controls follow them. It imports nothing of the app,
// so every package can use it.
package lang

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/i18n"
)

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
