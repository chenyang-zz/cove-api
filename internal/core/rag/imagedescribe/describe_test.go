package imagedescribe

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/boxify/api-go/internal/core/rag/imagecompress"
)

func TestDescriberCompressesImageAndCallsVisionClient(t *testing.T) {
	// 验证点：Describe 会先压缩图片，再把 base64、MIME、prompt 和 maxTokens 传给视觉模型。
	ctx := context.Background()
	compressor := &fakeCompressor{
		out: &imagecompress.Output{Data: []byte("compressed-image"), MIME: "image/jpeg"},
	}
	vision := &fakeVisionClient{
		answer: `{"description":"一张图片","ocr_text":"","objects":["图片"],"scene":"测试"}`,
	}
	parser := &fakeJSONParser{}

	got, err := NewDescriber(
		vision,
		WithCompressor(compressor),
		WithParser(parser),
		WithPrompt("自定义提示词"),
		WithMaxTokens(256),
	).Describe(ctx, Input{Data: []byte("raw-image"), FileExt: ".png"})

	if err != nil {
		t.Fatalf("Describe error = %v", err)
	}
	if compressor.got.Data == nil || string(compressor.got.Data) != "raw-image" || compressor.got.FileExt != ".png" {
		t.Fatalf("compressor input = %#v, want raw png input", compressor.got)
	}
	if vision.gotPrompt != "自定义提示词" || vision.gotMIME != "image/jpeg" || vision.gotMaxTokens != 256 {
		t.Fatalf("vision args = prompt:%q mime:%q tokens:%d", vision.gotPrompt, vision.gotMIME, vision.gotMaxTokens)
	}
	if vision.gotImageBase64 != base64.StdEncoding.EncodeToString([]byte("compressed-image")) {
		t.Fatalf("vision base64 = %q, want compressed-image base64", vision.gotImageBase64)
	}
	if got.Description != "一张图片" || got.Scene != "测试" || len(got.Objects) != 1 || got.Objects[0] != "图片" {
		t.Fatalf("description = %#v", got)
	}
}

func TestDescriberParsesRepairedJSONAnswer(t *testing.T) {
	// 验证点：模型返回的 markdown/json-repair 风格文本由注入 parser 负责修复并映射。
	vision := &fakeVisionClient{
		answer: "```json\n{'description':'文档截图','ocr_text':'标题','objects':['文字','表格'],'scene':'文档截图',}\n```",
	}
	parser := &fakeRepairParser{}

	got, err := NewDescriber(vision, WithParser(parser), WithCompressor(staticCompressor())).Describe(context.Background(), Input{
		Data:    []byte("image"),
		FileExt: ".jpg",
	})

	if err != nil {
		t.Fatalf("Describe error = %v", err)
	}
	if parser.gotInput != vision.answer {
		t.Fatalf("parser input = %q, want raw answer", parser.gotInput)
	}
	if got.Description != "文档截图" || got.OCRText != "标题" || got.Scene != "文档截图" {
		t.Fatalf("description = %#v", got)
	}
	if len(got.Objects) != 2 || got.Objects[0] != "文字" || got.Objects[1] != "表格" {
		t.Fatalf("objects = %#v", got.Objects)
	}
}

func TestDescriberFallsBackToRawDescriptionWhenParseFails(t *testing.T) {
	// 验证点：JSON 解析失败时不返回错误，截断原文作为 description，并清空结构化字段。
	rawAnswer := strings.Repeat("描述", 1200)
	vision := &fakeVisionClient{answer: rawAnswer}
	parser := &fakeJSONParser{err: errors.New("bad json")}

	got, err := NewDescriber(vision, WithParser(parser), WithCompressor(staticCompressor())).Describe(context.Background(), Input{
		Data:    []byte("image"),
		FileExt: ".jpg",
	})

	if err != nil {
		t.Fatalf("Describe error = %v", err)
	}
	if len([]rune(got.Description)) != 2000 {
		t.Fatalf("description rune len = %d, want 2000", len([]rune(got.Description)))
	}
	if got.OCRText != "" || got.Scene != "" || len(got.Objects) != 0 {
		t.Fatalf("fallback fields = %#v, want empty structured fields", got)
	}
}

func TestDescriberFiltersNonStringObjects(t *testing.T) {
	// 验证点：objects 只保留字符串项，避免业务层收到模型输出里的混合类型。
	vision := &fakeVisionClient{answer: `{"description":"桌面","ocr_text":"","objects":["电脑",42,true,"鼠标"],"scene":"办公室"}`}

	got, err := NewDescriber(vision, WithParser(&fakeJSONParser{}), WithCompressor(staticCompressor())).Describe(context.Background(), Input{
		Data:    []byte("image"),
		FileExt: ".jpg",
	})

	if err != nil {
		t.Fatalf("Describe error = %v", err)
	}
	if len(got.Objects) != 2 || got.Objects[0] != "电脑" || got.Objects[1] != "鼠标" {
		t.Fatalf("objects = %#v, want only string items", got.Objects)
	}
}

func TestDescriberReturnsCompressorError(t *testing.T) {
	// 验证点：压缩器失败时直接返回错误，不调用视觉模型。
	wantErr := errors.New("compress failed")
	vision := &fakeVisionClient{}

	got, err := NewDescriber(vision, WithParser(&fakeJSONParser{}), WithCompressor(&fakeCompressor{err: wantErr})).Describe(context.Background(), Input{
		Data:    []byte("image"),
		FileExt: ".jpg",
	})

	if !errors.Is(err, wantErr) || got != nil {
		t.Fatalf("Describe = %#v, %v, want compressor error", got, err)
	}
	if vision.called {
		t.Fatal("vision client was called, want skipped after compressor error")
	}
}

func TestDescriberReturnsVisionError(t *testing.T) {
	// 验证点：视觉模型调用失败时返回该错误，不进入 JSON 解析。
	wantErr := errors.New("vision failed")
	vision := &fakeVisionClient{err: wantErr}
	parser := &fakeJSONParser{}

	got, err := NewDescriber(vision, WithParser(parser), WithCompressor(staticCompressor())).Describe(context.Background(), Input{
		Data:    []byte("image"),
		FileExt: ".jpg",
	})

	if !errors.Is(err, wantErr) || got != nil {
		t.Fatalf("Describe = %#v, %v, want vision error", got, err)
	}
	if parser.called {
		t.Fatal("parser was called, want skipped after vision error")
	}
}

func TestDescriberAppliesOptions(t *testing.T) {
	// 验证点：构造器先设置默认值，再应用 prompt、maxTokens 和 compressor 选项。
	compressor := staticCompressor()

	describer := NewDescriber(&fakeVisionClient{},
		WithPrompt("prompt"),
		WithMaxTokens(512),
		WithCompressor(compressor),
		WithParser(&fakeJSONParser{}),
	)

	if describer.Prompt != "prompt" || describer.MaxTokens != 512 || describer.Compressor != compressor || describer.Parser == nil {
		t.Fatalf("options = prompt:%q tokens:%d compressor:%T parser:%T", describer.Prompt, describer.MaxTokens, describer.Compressor, describer.Parser)
	}
}

func TestDescriberUsesDefaultJSONParser(t *testing.T) {
	// 验证点：未通过 WithParser 注入时，Describer 使用 jsonx 默认 parser 解析标准 JSON。
	vision := &fakeVisionClient{answer: `{"description":"默认解析","ocr_text":"文字","objects":["屏幕"],"scene":"截图"}`}

	got, err := NewDescriber(vision, WithCompressor(staticCompressor())).Describe(context.Background(), Input{
		Data:    []byte("image"),
		FileExt: ".jpg",
	})

	if err != nil {
		t.Fatalf("Describe error = %v", err)
	}
	if got.Description != "默认解析" || got.OCRText != "文字" || len(got.Objects) != 1 || got.Objects[0] != "屏幕" || got.Scene != "截图" {
		t.Fatalf("description = %#v", got)
	}
}

type fakeCompressor struct {
	got imagecompress.Input
	out *imagecompress.Output
	err error
}

func (f *fakeCompressor) Compress(input imagecompress.Input) (*imagecompress.Output, error) {
	f.got = input
	if f.err != nil {
		return nil, f.err
	}
	return f.out, nil
}

type fakeVisionClient struct {
	called         bool
	gotPrompt      string
	gotImageBase64 string
	gotMIME        string
	gotMaxTokens   int64
	answer         string
	err            error
}

func (f *fakeVisionClient) Describe(ctx context.Context, prompt string, imageBase64 string, mime string, maxTokens int64) (string, error) {
	f.called = true
	f.gotPrompt = prompt
	f.gotImageBase64 = imageBase64
	f.gotMIME = mime
	f.gotMaxTokens = maxTokens
	if f.err != nil {
		return "", f.err
	}
	return f.answer, nil
}

type fakeJSONParser struct {
	called bool
	err    error
}

func (f *fakeJSONParser) Repair(input string) (string, error) {
	return input, f.err
}

func (f *fakeJSONParser) Unmarshal(input string, out any) error {
	f.called = true
	if f.err != nil {
		return f.err
	}
	return json.Unmarshal([]byte(input), out)
}

type fakeRepairParser struct {
	gotInput string
}

func (f *fakeRepairParser) Repair(input string) (string, error) {
	return input, nil
}

func (f *fakeRepairParser) Unmarshal(input string, out any) error {
	f.gotInput = input
	repaired := `{"description":"文档截图","ocr_text":"标题","objects":["文字","表格"],"scene":"文档截图"}`
	return json.Unmarshal([]byte(repaired), out)
}

func staticCompressor() *fakeCompressor {
	return &fakeCompressor{
		out: &imagecompress.Output{Data: []byte("compressed"), MIME: "image/jpeg"},
	}
}
