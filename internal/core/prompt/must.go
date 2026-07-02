// Package prompt 提供启动期使用的 Must 系列包装。
//
// 本文件中的函数只适合默认提示词初始化等不可恢复路径；运行期处理用户输入或
// 外部模板时，应使用返回 error 的 TemplateText、Render 或 RenderText。
package prompt

// MustTemplateText 读取模板失败时 panic，适合包级默认提示词初始化。
func MustTemplateText(fsys TemplateFS, name string) string {
	return must(TemplateText(fsys, name))
}

// MustRender 读取或渲染模板失败时 panic，适合不可恢复的默认提示词初始化。
func MustRender(fsys TemplateFS, name string, data any) string {
	return must(Render(fsys, name, data))
}

// MustRenderText 渲染文本模板失败时 panic，适合启动期常量初始化。
func MustRenderText(text string, data any) string {
	return must(RenderText(text, data))
}

// must 把启动期必须成功的模板错误转换为 panic，运行期路径应使用返回 error 的 API。
func must(value string, err error) string {
	if err != nil {
		panic(err)
	}
	return value
}
