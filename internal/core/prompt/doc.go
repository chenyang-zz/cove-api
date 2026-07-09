// Package prompt 提供提示词模板的注册、读取和渲染能力。
//
// 这个包把模板来源和模板解析分离：Manager 适合长期持有多组模板来源，
// Renderer 适合绑定单一文件系统来源，包级函数适合一次性读取或渲染。
//
// 外部业务提示词注册与渲染示例：
//
//	manager := prompt.NewManager()
//	err := appprompts.Register(manager)
//	client := promptsgen.NewClient(manager)
//	out, err := client.AgentOptimizePrompt(&promptsgen.AgentOptimizePromptParams{
//		RawPrompt: "你是一个助手",
//	})
//
// RegisterFS + Render 示例：
//
//	manager := prompt.NewManager()
//	err := manager.RegisterFS("custom", os.DirFS("/data/prompts"))
//	out, err := manager.Render("custom/classify.tmpl", data)
//
// RegisterText + Render 示例：
//
//	type ClassifyPromptData struct {
//		Content string
//		Tags    []string
//	}
//
//	manager := prompt.NewManager()
//	err := manager.RegisterText("db/classify", "内容：{{ .Content }}")
//	out, err := manager.Render("db/classify", ClassifyPromptData{Content: "用户输入"})
//
// RenderText 示例：
//
//	out, err := prompt.RenderText("你好 {{ .Name }}", map[string]any{"Name": "Boxify"})
package prompt
