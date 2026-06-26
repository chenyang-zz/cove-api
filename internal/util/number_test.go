package util_test

import (
	"testing"

	"github.com/boxify/api-go/internal/util"
)

func TestRoundKeepsRequestedDecimalPlaces(t *testing.T) {
	tests := []struct {
		name   string
		value  float64
		places int
		want   float64
	}{
		{name: "two decimal places", value: 3.14159, places: 2, want: 3.14},
		{name: "rounds half away from zero", value: 3.145, places: 2, want: 3.15},
		{name: "negative value", value: -3.145, places: 2, want: -3.15},
		{name: "zero places", value: 12.6, places: 0, want: 13},
		{name: "negative places treated as zero", value: 12.4, places: -1, want: 12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := util.Round(tt.value, tt.places); got != tt.want {
				t.Fatalf("Round(%v, %d) = %v, want %v", tt.value, tt.places, got, tt.want)
			}
		})
	}
}
