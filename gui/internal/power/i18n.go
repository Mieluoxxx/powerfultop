package power

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/i18n"
)

// l mirrors view.L, which this package can't import: view's pages import power.
func l(en, zh string) string {
	if strings.HasPrefix(i18n.Locale(), "zh") {
		return zh
	}
	return en
}

func lf(en, zh string, args ...any) string {
	return fmt.Sprintf(l(en, zh), args...)
}
