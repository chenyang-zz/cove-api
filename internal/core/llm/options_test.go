package llm

import (
	"testing"

	coretool "github.com/boxify/api-go/internal/core/tool"
)

// 验证 NewChatOptions 会提供核心层默认温度，并允许调用方显式覆盖。
func TestNewChatOptionsDefaultsTemperatureAndAllowsOverride(t *testing.T) {
	defaults := NewChatOptions()
	if defaults.Temperature == nil {
		t.Fatal("NewChatOptions().Temperature = nil, want default value")
	}
	if *defaults.Temperature != DefaultTemperature {
		t.Fatalf("NewChatOptions().Temperature = %v, want %v", *defaults.Temperature, DefaultTemperature)
	}
	if defaults.MaxTokens != nil {
		t.Fatalf("NewChatOptions().MaxTokens = %v, want nil", *defaults.MaxTokens)
	}

	overridden := NewChatOptions(WithTemperature(0.2))
	if overridden.Temperature == nil || *overridden.Temperature != 0.2 {
		t.Fatalf("NewChatOptions(WithTemperature).Temperature = %v, want 0.2", overridden.Temperature)
	}
}

// 验证 NewChatOptions 会接受合法 TopP，并忽略非法 TopP。
func TestNewChatOptionsWithTopP(t *testing.T) {
	valid := NewChatOptions(WithTopP(0.8))
	if valid.TopP == nil || *valid.TopP != 0.8 {
		t.Fatalf("NewChatOptions(WithTopP).TopP = %v, want 0.8", valid.TopP)
	}

	invalid := NewChatOptions(WithTopP(0), WithTopP(-1), WithTopP(1.2))
	if invalid.TopP != nil {
		t.Fatalf("NewChatOptions(invalid TopP).TopP = %v, want nil", *invalid.TopP)
	}
}

// 验证 WithTools 会复制工具描述，避免调用方后续修改污染模型调用参数。
func TestNewChatOptionsWithToolsClonesDescriptors(t *testing.T) {
	tools := []coretool.Descriptor{{
		Name:        "search",
		Description: "search docs",
		Schema: coretool.Schema{
			Parameters: coretool.ParametersSchema{
				Type: "object",
				Properties: map[string]coretool.PropertySchema{
					"query": {"type": "string"},
				},
				Required: []string{"query"},
			},
		},
		Annotations: map[string]any{"source": "test"},
	}}

	opts := NewChatOptions(WithTools(tools...))
	tools[0].Name = "changed"
	tools[0].Schema.Parameters.Properties["query"]["type"] = "number"
	tools[0].Schema.Parameters.Required[0] = "changed"
	tools[0].Annotations["source"] = "changed"

	if got := opts.Tools[0].Name; got != "search" {
		t.Fatalf("WithTools cloned name = %q, want search", got)
	}
	if got := opts.Tools[0].Schema.Parameters.Properties["query"]["type"]; got != "string" {
		t.Fatalf("WithTools cloned property type = %#v, want string", got)
	}
	if got := opts.Tools[0].Schema.Parameters.Required[0]; got != "query" {
		t.Fatalf("WithTools cloned required = %q, want query", got)
	}
	if got := opts.Tools[0].Annotations["source"]; got != "test" {
		t.Fatalf("WithTools cloned annotation = %#v, want test", got)
	}
}

// 验证 NewVisionOptions 复用聊天默认温度，并在未设置 MaxTokens 时回落看图默认值。
func TestNewVisionOptionsReusesChatDefaultsAndMaxTokensFallback(t *testing.T) {
	defaults := NewVisionOptions()
	if defaults.Temperature == nil || *defaults.Temperature != DefaultTemperature {
		t.Fatalf("NewVisionOptions().Temperature = %v, want %v", defaults.Temperature, DefaultTemperature)
	}
	if defaults.MaxTokens == nil || *defaults.MaxTokens != DefaultVisionMaxTokens {
		t.Fatalf("NewVisionOptions().MaxTokens = %v, want %d", defaults.MaxTokens, DefaultVisionMaxTokens)
	}

	overridden := NewVisionOptions(WithTemperature(0.2), WithTopP(0.9), WithMaxTokens(256))
	if overridden.Temperature == nil || *overridden.Temperature != 0.2 {
		t.Fatalf("NewVisionOptions().Temperature = %v, want 0.2", overridden.Temperature)
	}
	if overridden.TopP == nil || *overridden.TopP != 0.9 {
		t.Fatalf("NewVisionOptions().TopP = %v, want 0.9", overridden.TopP)
	}
	if overridden.MaxTokens == nil || *overridden.MaxTokens != 256 {
		t.Fatalf("NewVisionOptions().MaxTokens = %v, want 256", overridden.MaxTokens)
	}
}

// 验证工具选择 option 会输出预期策略，并忽略空工具名。
func TestNewChatOptionsWithToolChoice(t *testing.T) {
	cases := []struct {
		name string
		opt  ModelCallOption
		mode ToolChoiceMode
		tool string
	}{
		{name: "auto", opt: WithToolChoiceAuto(), mode: ToolChoiceAuto},
		{name: "none", opt: WithToolChoiceNone(), mode: ToolChoiceNone},
		{name: "required", opt: WithToolChoiceRequired(), mode: ToolChoiceRequired},
		{name: "tool", opt: WithRequiredTool("search"), mode: ToolChoiceTool, tool: "search"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := NewChatOptions(tc.opt)
			if opts.ToolChoice == nil {
				t.Fatalf("NewChatOptions(%s).ToolChoice = nil, want value", tc.name)
			}
			if opts.ToolChoice.Mode != tc.mode || opts.ToolChoice.Name != tc.tool {
				t.Fatalf("NewChatOptions(%s).ToolChoice = %#v, want mode %q name %q", tc.name, opts.ToolChoice, tc.mode, tc.tool)
			}
		})
	}

	invalid := NewChatOptions(WithRequiredTool(" "))
	if invalid.ToolChoice != nil {
		t.Fatalf("NewChatOptions(empty required tool).ToolChoice = %#v, want nil", invalid.ToolChoice)
	}
}
