package imagecompress

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestCompressorReturnsOriginalForSmallImage(t *testing.T) {
	// 验证点：小于目标字节数的图片不做重编码，直接返回原始内容和推断 MIME。
	raw := encodeJPEG(t, solidImage(12, 8, color.RGBA{R: 20, G: 40, B: 60, A: 255}), 90)

	out, err := NewCompressor(WithTargetBytes(len(raw) + 1)).Compress(Input{
		Data:    raw,
		FileExt: ".jpg",
	})

	if err != nil {
		t.Fatalf("Compress error = %v", err)
	}
	if !bytes.Equal(out.Data, raw) || out.MIME != "image/jpeg" || out.Compressed {
		t.Fatalf("output = %#v, data equal = %v, want original jpeg without compression", out, bytes.Equal(out.Data, raw))
	}
	if out.OriginalBytes != len(raw) || out.OutputBytes != len(raw) {
		t.Fatalf("bytes = %d/%d, want %d/%d", out.OriginalBytes, out.OutputBytes, len(raw), len(raw))
	}
}

func TestCompressorShrinksLargeJPEG(t *testing.T) {
	// 验证点：超过目标字节数的 JPEG 会按最长边缩放并重编码为更小的 JPEG。
	raw := encodeJPEG(t, gradientImage(640, 420), 95)

	out, err := NewCompressor(
		WithMaxEdge(128),
		WithTargetBytes(12*1024),
		WithQualities([]int{80, 60, 40}),
	).Compress(Input{
		Data:    raw,
		FileExt: "jpeg",
	})

	if err != nil {
		t.Fatalf("Compress error = %v", err)
	}
	if out.MIME != "image/jpeg" || !out.Compressed {
		t.Fatalf("mime/compressed = %s/%v, want jpeg compressed", out.MIME, out.Compressed)
	}
	if len(out.Data) >= len(raw) {
		t.Fatalf("compressed size = %d, original = %d, want smaller", len(out.Data), len(raw))
	}
	img, _, err := image.Decode(bytes.NewReader(out.Data))
	if err != nil {
		t.Fatalf("decode compressed image error = %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() > 128 || bounds.Dy() > 128 {
		t.Fatalf("compressed bounds = %dx%d, want longest edge <= 128", bounds.Dx(), bounds.Dy())
	}
}

func TestCompressorFlattensTransparentPNG(t *testing.T) {
	// 验证点：带透明通道的 PNG 压缩时会贴白底并转成 JPEG，避免模型侧收到大体积透明 PNG。
	raw := encodePNG(t, transparentImage(160, 120))

	out, err := NewCompressor(
		WithMaxEdge(80),
		WithTargetBytes(1),
		WithQualities([]int{75}),
	).Compress(Input{
		Data:    raw,
		FileExt: ".png",
	})

	if err != nil {
		t.Fatalf("Compress error = %v", err)
	}
	if out.MIME != "image/jpeg" || !out.Compressed {
		t.Fatalf("mime/compressed = %s/%v, want jpeg compressed", out.MIME, out.Compressed)
	}
	if _, _, err := image.Decode(bytes.NewReader(out.Data)); err != nil {
		t.Fatalf("decode flattened image error = %v", err)
	}
}

func TestCompressorUsesJPEGForUnknownExtension(t *testing.T) {
	// 验证点：未知扩展名按 JPEG 处理，保持多模态接口默认可接受格式。
	raw := []byte("not an image")

	out, err := NewCompressor(WithTargetBytes(len(raw) + 1)).Compress(Input{
		Data:    raw,
		FileExt: ".bin",
	})

	if err != nil {
		t.Fatalf("Compress error = %v", err)
	}
	if out.MIME != "image/jpeg" || !bytes.Equal(out.Data, raw) {
		t.Fatalf("output = %#v, want original data with jpeg mime", out)
	}
}

func TestCompressorReturnsOriginalWhenDecodeFails(t *testing.T) {
	// 验证点：大图解码失败时不升级为业务错误，返回原图供调用方决定后续处理。
	raw := bytes.Repeat([]byte("bad-image"), 128)

	out, err := NewCompressor(WithTargetBytes(1)).Compress(Input{
		Data:    raw,
		FileExt: ".webp",
	})

	if err != nil {
		t.Fatalf("Compress error = %v", err)
	}
	if out.MIME != "image/webp" || out.Compressed || !bytes.Equal(out.Data, raw) {
		t.Fatalf("output = %#v, data equal = %v, want original webp fallback", out, bytes.Equal(out.Data, raw))
	}
}

func TestCompressorAppliesOptions(t *testing.T) {
	// 验证点：构造器会先初始化默认值，再应用 With 选项覆盖压缩参数。
	compressor := NewCompressor(
		WithMaxEdge(256),
		WithTargetBytes(2048),
		WithQualities([]int{90, 50}),
	)

	if compressor.MaxEdge != 256 || compressor.TargetBytes != 2048 {
		t.Fatalf("options = edge:%d target:%d, want 256/2048", compressor.MaxEdge, compressor.TargetBytes)
	}
	if len(compressor.Qualities) != 2 || compressor.Qualities[0] != 90 || compressor.Qualities[1] != 50 {
		t.Fatalf("qualities = %#v, want [90 50]", compressor.Qualities)
	}
}

func solidImage(width int, height int, c color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: c}, image.Point{}, draw.Src)
	return img
}

func gradientImage(width int, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8((x * 255) / width),
				G: uint8((y * 255) / height),
				B: uint8((x + y) % 255),
				A: 255,
			})
		}
	}
	return img
}

func transparentImage(width int, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 240, G: 20, B: 120, A: uint8((x + y) % 255)})
		}
	}
	return img
}

func encodeJPEG(t *testing.T, img image.Image, quality int) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		t.Fatalf("encode jpeg error = %v", err)
	}
	return buf.Bytes()
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png error = %v", err)
	}
	return buf.Bytes()
}
