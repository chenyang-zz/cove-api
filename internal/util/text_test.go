package util_test

import (
	"math"
	"testing"

	"github.com/boxify/api-go/internal/util"
)

func TestTextSimNormalizesCaseAndWhitespace(t *testing.T) {
	if got := util.TextSim("  Boxify  ", "boxify"); got != 1.0 {
		t.Fatalf("TextSim = %v, want 1", got)
	}
}

func TestTextSimReturnsZeroForEmptyInput(t *testing.T) {
	if got := util.TextSim("", "boxify"); got != 0 {
		t.Fatalf("TextSim = %v, want 0", got)
	}
}

func TestTextSimUsesSequenceMatcherStyleRatio(t *testing.T) {
	if got := util.TextSim("abcd", "abxd"); math.Abs(got-0.75) > 1e-9 {
		t.Fatalf("TextSim = %v, want 0.75", got)
	}
}

func TestCosine(t *testing.T) {
	tests := []struct {
		name string
		a    []float64
		b    []float64
		want float64
	}{
		{name: "same direction", a: []float64{1, 2}, b: []float64{2, 4}, want: 1},
		{name: "orthogonal", a: []float64{1, 0}, b: []float64{0, 1}, want: 0},
		{name: "different lengths", a: []float64{1}, b: []float64{1, 2}, want: 0},
		{name: "zero vector", a: []float64{0, 0}, b: []float64{1, 2}, want: 0},
		{name: "nil vector", a: nil, b: []float64{1, 2}, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := util.Cosine(tt.a, tt.b); math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("Cosine = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMeanVector(t *testing.T) {
	tests := []struct {
		name    string
		vectors [][]float64
		want    []float64
	}{
		{name: "averages vectors by dimension", vectors: [][]float64{{1, 2, 3}, {3, 4, 5}}, want: []float64{2, 3, 4}},
		{name: "single vector", vectors: [][]float64{{1.5, -2}}, want: []float64{1.5, -2}},
		{name: "nil input", vectors: nil, want: []float64{}},
		{name: "empty input", vectors: [][]float64{}, want: []float64{}},
		{name: "contains empty vector", vectors: [][]float64{{1, 2}, {}}, want: []float64{}},
		{name: "different dimensions", vectors: [][]float64{{1, 2}, {3}}, want: []float64{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := util.MeanVector(tt.vectors)
			if len(got) != len(tt.want) {
				t.Fatalf("MeanVector len = %d, want %d; got=%v", len(got), len(tt.want), got)
			}
			for i := range got {
				if math.Abs(got[i]-tt.want[i]) > 1e-9 {
					t.Fatalf("MeanVector[%d] = %v, want %v; got=%v", i, got[i], tt.want[i], got)
				}
			}
		})
	}
}

func TestContainsNormalizesCaseAndWhitespace(t *testing.T) {
	if !util.Contains(" Box ", "my boxify agent") {
		t.Fatal("Contains = false, want true")
	}
	if !util.Contains("my boxify agent", "BOXIFY") {
		t.Fatal("Contains reverse = false, want true")
	}
}

func TestContainsReturnsFalseForEmptyInput(t *testing.T) {
	if util.Contains("", "boxify") {
		t.Fatal("Contains = true, want false")
	}
}

func TestNormalizeRequired(t *testing.T) {
	if got := util.NormalizeRequired("  Alice@Example.COM  "); got != "alice@example.com" {
		t.Fatalf("NormalizeRequired = %q, want alice@example.com", got)
	}
}

func TestNormalizeOptional(t *testing.T) {
	if got := util.NormalizeOptional(nil, true); got != nil {
		t.Fatalf("NormalizeOptional nil = %v, want nil", got)
	}
	if got := util.NormalizeOptional(ptr("   "), true); got != nil {
		t.Fatalf("NormalizeOptional blank = %v, want nil", got)
	}
	if got := util.NormalizeOptional(ptr("  ALICE@example.COM  "), true); got == nil || *got != "alice@example.com" {
		t.Fatalf("NormalizeOptional lower = %v, want alice@example.com", got)
	}
	if got := util.NormalizeOptional(ptr("  Alice  "), false); got == nil || *got != "Alice" {
		t.Fatalf("NormalizeOptional keep case = %v, want Alice", got)
	}
}

func ptr(value string) *string {
	return &value
}
