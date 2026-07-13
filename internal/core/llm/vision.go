package llm

import (
	"encoding/json"
	"strings"

	"github.com/boxify/api-go/internal/core/valuex"
)

const (
	maxVisionDescriptionRunes = 2000
	maxVisionOCRTextRunes     = 2000
	maxVisionSceneRunes       = 64
)

// VisionDescription 表示看图任务约定的结构化字段，与默认图片描述 prompt 的 JSON 契约对齐。
type VisionDescription struct {
	Description string   `json:"description"`
	OCRText     string   `json:"ocr_text"`
	Objects     []string `json:"objects"`
	Scene       string   `json:"scene"`
}

// VisionResult 表示一次 Vision 调用的结构化结果。
//
// Description 是规整后的看图字段；Text 保留模型原文，便于审计与兜底。
type VisionResult struct {
	Description VisionDescription
	Text        string
	Model       string
	Provider    string
	ID          string
	StopReason  string
	Usage       TokenUsage
	RawJSON     string
	Metadata    map[string]any
}

type rawVisionDescription struct {
	Description any `json:"description"`
	OCRText     any `json:"ocr_text"`
	Objects     any `json:"objects"`
	Scene       any `json:"scene"`
}

// ParseVisionDescription 将模型原文解析为 VisionDescription。
//
// 支持标准 JSON；会去掉常见 markdown 代码块包裹。解析失败时把原文截断写入 Description，
// Objects 为空切片，避免上层丢失可检索文本。
func ParseVisionDescription(answer string) VisionDescription {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return VisionDescription{Objects: []string{}}
	}

	payload := stripMarkdownCodeFence(answer)
	var raw rawVisionDescription
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return VisionDescription{
			Description: truncateRunes(answer, maxVisionDescriptionRunes),
			Objects:     []string{},
		}
	}
	return VisionDescription{
		Description: truncateRunes(valuex.String(raw.Description), maxVisionDescriptionRunes),
		OCRText:     truncateRunes(valuex.String(raw.OCRText), maxVisionOCRTextRunes),
		Objects:     valuex.StringList(raw.Objects),
		Scene:       truncateRunes(valuex.String(raw.Scene), maxVisionSceneRunes),
	}
}

func stripMarkdownCodeFence(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "```") {
		return text
	}
	lines := strings.Split(text, "\n")
	if len(lines) < 2 {
		return text
	}
	// 去掉首行 ``` 或 ```json
	body := lines[1:]
	if len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "```" {
		body = body[:len(body)-1]
	}
	return strings.TrimSpace(strings.Join(body, "\n"))
}

func truncateRunes(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes])
}
