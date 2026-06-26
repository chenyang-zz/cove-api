package util

import "math"

// Round 将浮点数四舍五入到指定小数位；places 小于 0 时按 0 处理。
func Round(value float64, places int) float64 {
	if places < 0 {
		places = 0
	}
	scale := math.Pow10(places)
	return math.Round(value*scale) / scale
}
