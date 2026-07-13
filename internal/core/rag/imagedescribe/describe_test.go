package imagedescribe

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	corellm "github.com/boxify/api-go/internal/core/llm"
	"github.com/boxify/api-go/internal/core/rag/imagecompress"
)

// 验证点：Describe 会先压缩图片，再把 base64、MIME、prompt 和 maxTokens 传给 corellm.VisionClient。
func TestDescriberCompressesImageAndCallsVisionClient(t *testing.T) {
	ctx := context.Background()
	compressor := &fakeCompressor{
		out: &imagecompress.Output{Data: []byte("compressed-image"), MIME: "image/jpeg"},
	}
	vision := &fakeVisionClient{
		result: &corellm.VisionResult{
			Description: corellm.VisionDescription{
				Description: "一张图片",
				Objects:     []string{"图片"},
				Scene:       "测试",
			},
			Text: `{"description":"一张图片","objects":["图片"],"scene":"测试"}`,
		},
	}

	got, err := NewDescriber(
		vision,
		WithCompressor(compressor),
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

// 验证点：压缩器失败时直接返回错误，不调用视觉模型。
func TestDescriberReturnsCompressorError(t *testing.T) {
	wantErr := errors.New("compress failed")
	vision := &fakeVisionClient{}

	got, err := NewDescriber(vision, WithCompressor(&fakeCompressor{err: wantErr})).Describe(context.Background(), Input{
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

// 验证点：视觉模型调用失败时返回该错误。
func TestDescriberReturnsVisionError(t *testing.T) {
	wantErr := errors.New("vision failed")
	vision := &fakeVisionClient{err: wantErr}

	got, err := NewDescriber(vision, WithCompressor(staticCompressor())).Describe(context.Background(), Input{
		Data:    []byte("image"),
		FileExt: ".jpg",
	})

	if !errors.Is(err, wantErr) || got != nil {
		t.Fatalf("Describe = %#v, %v, want vision error", got, err)
	}
}

// 验证点：构造器先设置默认值，再应用 prompt、maxTokens 和 compressor 选项。
func TestDescriberAppliesOptions(t *testing.T) {
	compressor := staticCompressor()
	vision := &fakeVisionClient{}

	describer := NewDescriber(vision,
		WithPrompt("prompt"),
		WithMaxTokens(512),
		WithCompressor(compressor),
	)

	if describer.Prompt != "prompt" || describer.MaxTokens != 512 || describer.Compressor != compressor || describer.vision != vision {
		t.Fatalf("options = prompt:%q tokens:%d compressor:%T vision:%T", describer.Prompt, describer.MaxTokens, describer.Compressor, describer.vision)
	}
}

// 验证点：WithVisionClient 可覆盖构造时传入的客户端。
func TestDescriberWithVisionClientOverridesConstructor(t *testing.T) {
	primary := &fakeVisionClient{}
	override := &fakeVisionClient{
		result: &corellm.VisionResult{Description: corellm.VisionDescription{Description: "override"}},
	}
	describer := NewDescriber(primary, WithVisionClient(override), WithCompressor(staticCompressor()))
	got, err := describer.Describe(context.Background(), Input{Data: []byte("x"), FileExt: ".png"})
	if err != nil {
		t.Fatalf("Describe error = %v", err)
	}
	if primary.called {
		t.Fatal("primary vision was called, want override client")
	}
	if !override.called || got.Description != "override" {
		t.Fatalf("override result = %#v called=%v", got, override.called)
	}
}

// 验证点：Vision 返回的结构化 Description 会被原样透出。
func TestDescriberReturnsStructuredVisionDescription(t *testing.T) {
	vision := &fakeVisionClient{
		result: &corellm.VisionResult{
			Description: corellm.VisionDescription{
				Description: "默认解析",
				OCRText:     "文字",
				Objects:     []string{"屏幕"},
				Scene:       "截图",
			},
		},
	}
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
	result         *corellm.VisionResult
	err            error
}

func (f *fakeVisionClient) Vision(ctx context.Context, prompt string, imageBase64 string, mime string, opts ...corellm.ModelCallOption) (*corellm.VisionResult, error) {
	f.called = true
	f.gotPrompt = prompt
	f.gotImageBase64 = imageBase64
	f.gotMIME = mime
	visionOpts := corellm.NewVisionOptions(opts...)
	if visionOpts.MaxTokens != nil {
		f.gotMaxTokens = *visionOpts.MaxTokens
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func staticCompressor() *fakeCompressor {
	return &fakeCompressor{
		out: &imagecompress.Output{Data: []byte("compressed"), MIME: "image/jpeg"},
	}
}
