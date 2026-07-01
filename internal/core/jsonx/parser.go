package jsonx

import (
	"encoding/json"

	"github.com/boxify/api-go/internal/xerr"
)

type Parser interface {
	Repair(input string) (string, error)
	Unmarshal(input string, out any) error
}

type standardParser struct{}

func NewParser() Parser {
	return &standardParser{}
}

func (p *standardParser) Repair(input string) (string, error) {
	return input, nil
}

func (p *standardParser) Unmarshal(input string, out any) error {
	return json.Unmarshal([]byte(input), out)
}

func Parse[T any](parser Parser, input string) (T, error) {
	var out T
	if err := parser.Unmarshal(input, &out); err != nil {
		return out, xerr.Wrapf(err, "parse json failed: %v", err)
	}
	return out, nil
}
