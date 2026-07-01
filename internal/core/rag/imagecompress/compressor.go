package imagecompress

import (
	"bytes"
	"image"
	"image/color"
	stddraw "image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"strings"

	xdraw "golang.org/x/image/draw"
)

type Compressor struct {
	Options
}

func NewCompressor(opts ...Option) *Compressor {
	compressor := &Compressor{
		Options: Options{
			MaxEdge:     defaultMaxEdge,
			TargetBytes: defaultTargetBytes,
			Qualities:   append([]int(nil), defaultQualities...),
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&compressor.Options)
		}
	}
	return compressor
}

// Compress 压缩图片
func (c *Compressor) Compress(input Input) (*Output, error) {
	mime := GuessMIME(input.FileExt)
	out := &Output{
		Data:          input.Data,
		MIME:          mime,
		OriginalBytes: len(input.Data),
		OutputBytes:   len(input.Data),
	}
	if len(input.Data) <= c.TargetBytes {
		return out, nil
	}

	img, _, err := image.Decode(bytes.NewReader(input.Data))
	if err != nil {
		return out, nil
	}

	rgb := flattenToRGB(img)
	resized := resizeToMaxEdge(rgb, c.MaxEdge)
	data, err := encodeJPEGByQuality(resized, c.Qualities, c.TargetBytes)
	if err != nil {
		return out, nil
	}

	out.Data = data
	out.MIME = "image/jpeg"
	out.Compressed = true
	out.OutputBytes = len(data)
	return out, nil
}

// GuessMIME 根据文件扩展名猜测 MIME 类型
func GuessMIME(fileExt string) string {
	switch strings.TrimPrefix(strings.ToLower(strings.TrimSpace(fileExt)), ".") {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "bmp":
		return "image/bmp"
	case "webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

// flattenToRGB 将图片转换为 RGB 格式，并统一贴白底。
func flattenToRGB(img image.Image) *image.RGBA {
	bounds := img.Bounds()
	rgb := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))

	// 透明图片统一贴白底后再编码 JPEG，避免透明通道在模型输入中造成体积和兼容性问题。
	stddraw.Draw(rgb, rgb.Bounds(), &image.Uniform{C: color.White}, image.Point{}, stddraw.Src)
	stddraw.Draw(rgb, rgb.Bounds(), img, bounds.Min, stddraw.Over)
	return rgb
}

// resizeToMaxEdge 约束最长边，保持原始宽高比，避免为了压缩破坏图片语义。
func resizeToMaxEdge(img *image.RGBA, maxEdge int) *image.RGBA {
	if maxEdge <= 0 {
		return img
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	longest := max(width, height)
	if longest <= maxEdge {
		return img
	}

	// 只约束最长边，保持原始宽高比，避免为了压缩破坏图片语义。
	scale := float64(maxEdge) / float64(longest)
	targetWidth := max(1, int(float64(width)*scale))
	targetHeight := max(1, int(float64(height)*scale))
	resized := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	xdraw.CatmullRom.Scale(resized, resized.Bounds(), img, bounds, xdraw.Over, nil)
	return resized
}

// encodeJPEGByQuality 按质量从高到低尝试，命中目标体积后立即返回，保留尽可能多的视觉信息。
func encodeJPEGByQuality(img image.Image, qualities []int, targetBytes int) ([]byte, error) {
	if len(qualities) == 0 {
		qualities = defaultQualities
	}

	var last []byte
	for _, quality := range qualities {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
			return nil, err
		}

		// 按质量从高到低尝试，命中目标体积后立即返回，保留尽可能多的视觉信息。
		last = buf.Bytes()
		if targetBytes <= 0 || len(last) <= targetBytes {
			return last, nil
		}
	}
	return last, nil
}
