package util_test

import (
	"reflect"
	"testing"

	"github.com/boxify/api-go/internal/util"
)

func TestHead(t *testing.T) {
	tests := []struct {
		name string
		in   []int
		n    int
		want []int
	}{
		{name: "returns first n items", in: []int{1, 2, 3}, n: 2, want: []int{1, 2}},
		{name: "returns original slice when shorter than n", in: []int{1, 2}, n: 3, want: []int{1, 2}},
		{name: "returns original slice when equal to n", in: []int{1, 2}, n: 2, want: []int{1, 2}},
		{name: "zero n", in: []int{1, 2}, n: 0, want: []int{}},
		{name: "negative n", in: []int{1, 2}, n: -1, want: []int{}},
		{name: "nil slice", in: nil, n: 2, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := util.Head(tt.in, tt.n); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Head = %#v, want %#v", got, tt.want)
			}
		})
	}
}
