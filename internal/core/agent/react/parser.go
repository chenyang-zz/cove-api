package react

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	coretool "github.com/boxify/api-go/internal/core/tool"
)

var (
	reactThoughtRE     = regexp.MustCompile(`(?m)^Thought\s*:\s*(.+)$`)
	reactActionRE      = regexp.MustCompile(`(?m)^Action\s*:\s*(.+)$`)
	reactActionInputRE = regexp.MustCompile(`(?s)Action\s*Input\s*:\s*(.*)$`)
	reactFinalRE       = regexp.MustCompile(`(?s)Final\s*Answer\s*:\s*(.*)$`)
)

const defaultTextActionInputKey = "query"

// Parser 解析模型输出为 Agent 可执行的决策。
type Parser interface {
	Parse(ctx context.Context, text string) (Decision, error)
}

// ParserOption 配置 ReAct parser 的文本兜底行为。
type ParserOption func(*ReActParser)

// ReActParser 解析基础 ReAct 文本协议。
type ReActParser struct {
	textActionInputKey string
}

// NewReActParser 创建默认 ReAct parser。
func NewReActParser(opts ...ParserOption) *ReActParser {
	parser := &ReActParser{textActionInputKey: defaultTextActionInputKey}
	for _, opt := range opts {
		if opt != nil {
			opt(parser)
		}
	}
	return parser
}

// WithTextActionInputKey 设置纯文本 Action Input 映射到的工具参数字段。
//
// key 为空时会被忽略，默认字段为 query。
func WithTextActionInputKey(key string) ParserOption {
	return func(parser *ReActParser) {
		key = strings.TrimSpace(key)
		if key != "" {
			parser.textActionInputKey = key
		}
	}
}

// Parse 解析 ReAct 文本输出。
func (p *ReActParser) Parse(ctx context.Context, text string) (Decision, error) {
	thought := firstMatch(reactThoughtRE, text)
	if final := firstMatch(reactFinalRE, text); final != "" {
		return Decision{
			Kind:        DecisionFinal,
			Thought:     thought,
			FinalAnswer: strings.TrimSpace(final),
		}, nil
	}

	action := firstMatch(reactActionRE, text)
	if action == "" {
		return Decision{}, fmt.Errorf("%w: missing action or final answer", ErrParseDecision)
	}
	input, err := p.parseActionInput(firstMatch(reactActionInputRE, text))
	if err != nil {
		return Decision{}, err
	}
	return Decision{
		Kind:        DecisionToolCall,
		Thought:     thought,
		Action:      strings.TrimSpace(action),
		ActionInput: input,
	}, nil
}

func (p *ReActParser) parseActionInput(raw string) (coretool.Input, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return coretool.Input{}, nil
	}
	if strings.HasPrefix(raw, "{") {
		var out map[string]any
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidActionInput, err)
		}
		if out == nil {
			return nil, errors.New("invalid action input")
		}
		return coretool.Input(out), nil
	}

	// JSON 非 object 值通常来自模型误输出，不能当作查询词静默传给工具。
	if json.Valid([]byte(raw)) {
		return nil, fmt.Errorf("%w: action input must be object or plain text", ErrInvalidActionInput)
	}
	return coretool.Input{p.textActionInputKey: raw}, nil
}

func firstMatch(re *regexp.Regexp, text string) string {
	match := re.FindStringSubmatch(text)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}
