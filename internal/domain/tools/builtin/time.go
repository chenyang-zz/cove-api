package builtin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	coretool "github.com/boxify/api-go/internal/core/tool"
)

// ErrInvalidTimezone 表示调用方传入了无法加载的 IANA 时区名称。
var ErrInvalidTimezone = errors.New("invalid timezone")

// NewCurrentTimeTool 创建内置 current_time 工具。
func NewCurrentTimeTool(opts ...Option) coretool.Tool {
	cfg := applyOptions(opts...)
	return coretool.NewFuncTool(coretool.Descriptor{
		Name:        ToolCurrentTime,
		Description: "获取当前时间，可按指定 IANA 时区返回。",
		Schema: coretool.Schema{
			Parameters: coretool.ParametersSchema{
				Type: "object",
				Properties: map[string]coretool.PropertySchema{
					"timezone": {
						"type":        "string",
						"description": "可选的 IANA 时区名称，默认使用 UTC。",
					},
				},
				AdditionalProperties: false,
			},
		},
	}, func(_ context.Context, input coretool.Input) (coretool.Output, error) {
		location, err := loadLocation(input)
		if err != nil {
			return coretool.Output{}, err
		}
		now := cfg.clock().In(location)
		formatted := now.Format(time.RFC3339)
		return coretool.Output{
			Text: formatted,
			Metadata: map[string]any{
				"time":     formatted,
				"unix":     now.Unix(),
				"timezone": location.String(),
				"offset":   formatOffset(now),
			},
		}, nil
	})
}

func loadLocation(input coretool.Input) (*time.Location, error) {
	timezone, err := timezoneFromInput(input)
	if err != nil {
		return nil, err
	}
	if timezone == "" {
		return time.UTC, nil
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidTimezone, timezone)
	}
	return location, nil
}

func timezoneFromInput(input coretool.Input) (string, error) {
	if input == nil {
		return "", nil
	}
	value, ok := input["timezone"]
	if !ok || value == nil {
		return "", nil
	}
	timezone, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%w: timezone must be a string", ErrInvalidTimezone)
	}
	return strings.TrimSpace(timezone), nil
}

func formatOffset(t time.Time) string {
	_, seconds := t.Zone()
	sign := "+"
	if seconds < 0 {
		sign = "-"
		seconds = -seconds
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	return fmt.Sprintf("%s%02d:%02d", sign, hours, minutes)
}
