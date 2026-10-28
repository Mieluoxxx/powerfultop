package view

import "powerfultop/internal/lang"

// L and Lf keep lang's functions short in the pages, which call them on nearly every line.
func L(en, zh string) string { return lang.L(en, zh) }

func Lf(en, zh string, args ...any) string { return lang.Lf(en, zh, args...) }
