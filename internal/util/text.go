package util

import (
	"math"
	"strings"
)

// TextSim 返回两个字符串的 SequenceMatcher 风格相似度。
// 入参会先去除首尾空白并转小写；任一侧为空时返回 0。
func TextSim(a string, b string) float64 {
	ar, br := normalizeRunes(a), normalizeRunes(b)
	if len(ar) == 0 || len(br) == 0 {
		return 0
	}
	matches := matchingRunes(ar, br)
	return float64(2*matches) / float64(len(ar)+len(br))
}

// Cosine 返回两个向量的余弦相似度。
// 任一向量为空、维度不一致或任一向量范数为 0 时返回 0。
func Cosine(a []float64, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na <= 0 || nb <= 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// MeanVector 返回多个同维向量的平均向量。
// 输入为空、存在空向量或维度不一致时返回空切片。
func MeanVector(vectors [][]float64) []float64 {
	if len(vectors) == 0 || len(vectors[0]) == 0 {
		return []float64{}
	}
	dim := len(vectors[0])
	mean := make([]float64, dim)
	for _, vector := range vectors {
		if len(vector) != dim {
			return []float64{}
		}
		for i, value := range vector {
			mean[i] += value
		}
	}
	for i := range mean {
		mean[i] /= float64(len(vectors))
	}
	return mean
}

// Contains 判断两个标准化后的字符串是否存在双向包含关系。
// 入参会先去除首尾空白并转小写；空字符串不会匹配。
func Contains(a string, b string) bool {
	a = normalizeText(a)
	b = normalizeText(b)
	return a != "" && b != "" && (strings.Contains(a, b) || strings.Contains(b, a))
}

func NormalizeRequired(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func NormalizeOptional(value *string, lower bool) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil
	}
	if lower {
		normalized = strings.ToLower(normalized)
	}
	return &normalized
}

func normalizeText(value string) string {
	return NormalizeRequired(value)
}

func normalizeRunes(value string) []rune {
	return []rune(normalizeText(value))
}

func matchingRunes(a []rune, b []rune) int {
	aStart, bStart, size := longestCommonBlock(a, b)
	if size == 0 {
		return 0
	}
	return matchingRunes(a[:aStart], b[:bStart]) +
		size +
		matchingRunes(a[aStart+size:], b[bStart+size:])
}

func longestCommonBlock(a []rune, b []rune) (int, int, int) {
	bestA, bestB, bestSize := 0, 0, 0
	for i := range a {
		for j := range b {
			size := 0
			for i+size < len(a) && j+size < len(b) && a[i+size] == b[j+size] {
				size++
			}
			if size > bestSize {
				bestA, bestB, bestSize = i, j, size
			}
		}
	}
	return bestA, bestB, bestSize
}
