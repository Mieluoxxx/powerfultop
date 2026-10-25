package thermal

import "fmt"

// AppleSmartBattery temperature fields use different units; these document them in one place.

// InstantCelsius converts the instant pack Temperature, in centidegrees Celsius.
func InstantCelsius(centidegrees int64) float64 { return float64(centidegrees) / 100 }

// LifetimeCelsius converts MaximumTemperature / MinimumTemperature, already whole degrees Celsius.
func LifetimeCelsius(wholeDegrees int64) int64 { return wholeDegrees }

// LifetimeAverageCelsius converts AverageTemperature, in decidegrees Celsius.
func LifetimeAverageCelsius(decidegrees int64) float64 { return float64(decidegrees) / 10 }

// FormatCelsius writes a temperature the way every page shows it, e.g. "42.5°C".
func FormatCelsius(celsius float64, decimals int) string {
	return fmt.Sprintf("%.*f°C", decimals, celsius)
}

// FormatRPM writes a fan speed, e.g. "2317 RPM".
func FormatRPM(rpm float64) string { return fmt.Sprintf("%.0f RPM", rpm) }
