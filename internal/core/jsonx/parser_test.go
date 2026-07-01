package jsonx_test

import (
	"encoding/json"
	"testing"

	"github.com/boxify/api-go/internal/core/jsonx"
)

type fakeParser struct {
	called bool
}

func (p *fakeParser) Repair(input string) (string, error) {
	return input, nil
}

func (p *fakeParser) Unmarshal(input string, out any) error {
	p.called = true
	data := []byte(`{"score":0.95}`)
	return json.Unmarshal(data, out)
}

func TestGenericDecodeUsesParserContract(t *testing.T) {
	// 验证点：Parse 使用传入 parser 的 Unmarshal 合约，不直接绑定具体 JSON 实现。
	parser := fakeParser{}

	var _ jsonx.Parser = &parser

	got, err := jsonx.Parse[struct {
		Score json.Number `json:"score"`
	}](&parser, `ignored`)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	if got.Score.String() != "0.95" {
		t.Fatalf("score = %s", got.Score.String())
	}
	if !parser.called {
		t.Fatal("parser was not used")
	}
}

func TestDefaultParserUnmarshalsJSON(t *testing.T) {
	// 验证点：默认 Parser 使用标准 JSON 解析，供 core 包在不注入修复器时安全使用。
	parser := jsonx.NewParser()

	var got struct {
		Name string `json:"name"`
	}
	if err := parser.Unmarshal(`{"name":"boxify"}`, &got); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if got.Name != "boxify" {
		t.Fatalf("name = %q, want boxify", got.Name)
	}
}
