package thermal

import "fmt"

// FormatCelsius writes a temperature the way every page shows it, e.g. "42.5°C".
func FormatCelsius(celsius float64, decimals int) string {
	return fmt.Sprintf("%.*f°C", decimals, celsius)
}

// FormatRPM writes a fan speed, e.g. "2317 RPM".
func FormatRPM(rpm float64) string { return fmt.Sprintf("%.0f RPM", rpm) }
