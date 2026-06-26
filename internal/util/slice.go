package util

// Head 返回切片前 n 个元素；n 小于等于 0 时返回空切片。
func Head[T any](s []T, n int) []T {
	if n <= 0 {
		return s[:0]
	}
	if len(s) <= n {
		return s
	}
	return s[:n]
}
