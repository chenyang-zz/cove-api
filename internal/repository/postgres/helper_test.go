package postgres

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsUniqueViolation(t *testing.T) {
	uniqueErr := &pgconn.PgError{Code: "23505"}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "direct unique violation", err: uniqueErr, want: true},
		{name: "wrapped unique violation", err: fmt.Errorf("insert user: %w", uniqueErr), want: true},
		{name: "other postgres error", err: &pgconn.PgError{Code: "23503"}, want: false},
		{name: "ordinary error", err: errors.New("boom"), want: false},
		{name: "nil", err: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUniqueViolation(tt.err); got != tt.want {
				t.Fatalf("isUniqueViolation() = %v, want %v", got, tt.want)
			}
		})
	}
}
