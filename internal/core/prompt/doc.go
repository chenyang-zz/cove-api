// Package prompt 提供提示词模板的注册、读取和渲染能力。
//
// 这个包把模板来源和模板解析分离：Manager 适合长期持有多组模板来源，
// Renderer 适合绑定单一文件系统来源，包级函数适合一次性读取或渲染。
//
// NewManager + RegisterFS + Render 示例：
//
//	manager := prompt.NewManager("internal/prompts")
//	err := manager.RegisterFS("rag", ragprompt.Templates)
//	out, err := manager.Render("rag/content_classifier.tmpl", ragprompt.ContentClassifierData{
//		Existing: "技术、学习",
//		Content:  "需要分类的正文",
//	})
//
// RegisterText + Render 示例：
//
//	type ClassifyPromptData struct {
//		Content string
//		Tags    []string
//	}
//
//	manager := prompt.NewManager("")
//	err := manager.RegisterText("db/classify", "内容：{{ .Content }}")
//	out, err := manager.Render("db/classify", ClassifyPromptData{Content: "用户输入"})
//
// RenderText 示例：
//
//	out, err := prompt.RenderText("你好 {{ .Name }}", map[string]any{"Name": "Boxify"})
package prompt
