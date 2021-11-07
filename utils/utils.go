package utils

import "math"

// roundFloat rounds a float to the nearest n integer
func RoundFloat(f float64, n int) float64 {
	pow := math.Pow10(n)
	return math.Round(f*pow) / pow
}
