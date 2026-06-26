package util_test

import (
	"testing"
	"time"

	"github.com/boxify/api-go/internal/util"
)

func TestISO8601OrNULLReturnsNULLForZeroTime(t *testing.T) {
	if got := util.ISO8601OrNULL(time.Time{}); got != "NULL" {
		t.Fatalf("ISO8601OrNULL = %q, want NULL", got)
	}
}

func TestISO8601OrNULLReturnsUTCISOTime(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	value := time.Date(2026, 6, 23, 10, 4, 5, 0, loc)

	if got := util.ISO8601OrNULL(value); got != "2026-06-23T02:04:05Z" {
		t.Fatalf("ISO8601OrNULL = %q", got)
	}
}

func TestParseISOTime(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  time.Time
	}{
		{
			name:  "rfc3339 with zone",
			value: "2026-06-23T10:04:05+08:00",
			want:  time.Date(2026, 6, 23, 10, 4, 5, 0, time.FixedZone("", 8*60*60)),
		},
		{
			name:  "rfc3339 nano",
			value: "2026-06-23T10:04:05.123456789Z",
			want:  time.Date(2026, 6, 23, 10, 4, 5, 123456789, time.UTC),
		},
		{
			name:  "python isoformat space separator",
			value: "2026-06-23 10:04:05+08:00",
			want:  time.Date(2026, 6, 23, 10, 4, 5, 0, time.FixedZone("", 8*60*60)),
		},
		{
			name:  "without timezone uses utc",
			value: "2026-06-23T10:04:05",
			want:  time.Date(2026, 6, 23, 10, 4, 5, 0, time.UTC),
		},
		{
			name:  "date only uses utc midnight",
			value: "2026-06-23",
			want:  time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := util.ParseISOTime(tt.value)
			if err != nil {
				t.Fatalf("ParseISOTime error = %v", err)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("ParseISOTime = %s, want %s", got.Format(time.RFC3339Nano), tt.want.Format(time.RFC3339Nano))
			}
		})
	}
}

func TestParseISOTimeReturnsErrorForEmptyOrInvalidInput(t *testing.T) {
	for _, value := range []string{"", "   ", "not-time"} {
		t.Run(value, func(t *testing.T) {
			if _, err := util.ParseISOTime(value); err == nil {
				t.Fatal("ParseISOTime error = nil, want error")
			}
		})
	}
}
