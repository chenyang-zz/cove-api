package webcrawl

import (
	"bytes"
	"context"
	"errors"
	"strings"

	"github.com/boxify/api-go/internal/core/valuex"
	"golang.org/x/net/html"
)

const defaultTitleMaxRunes = 200

type HTMLExtractor struct {
	TitleMaxRunes int
}

type ExtractorOption func(*HTMLExtractor)

func NewHTMLExtractor(opts ...ExtractorOption) *HTMLExtractor {
	extractor := &HTMLExtractor{
		TitleMaxRunes: defaultTitleMaxRunes,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(extractor)
		}
	}
	return extractor
}

func WithTitleMaxRunes(maxRunes int) ExtractorOption {
	return func(extractor *HTMLExtractor) {
		if maxRunes > 0 {
			extractor.TitleMaxRunes = maxRunes
		}
	}
}

// Extract 提取网页标题和内容
func (e *HTMLExtractor) Extract(ctx context.Context, page Page) (*Output, error) {
	doc, err := html.Parse(bytes.NewReader(page.HTML))
	if err != nil {
		return nil, err
	}
	title := firstTitle(doc)
	content := readableText(doc)
	if content == "" {
		return nil, errors.New("web page content is empty")
	}
	if title == "" {
		title = page.URL
	}
	return &Output{Title: valuex.TruncateRunesWithSuffix(title, e.TitleMaxRunes, "..."), Content: content, URL: page.URL}, nil
}

// firstTitle 提取网页标题
func firstTitle(root *html.Node) string {
	var title string
	var walk func(*html.Node, bool)
	walk = func(n *html.Node, inTitle bool) {
		if n == nil || title != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "title" {
			inTitle = true
		}
		if inTitle && n.Type == html.TextNode {
			title = strings.TrimSpace(n.Data)
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child, inTitle)
		}
	}
	walk(root, false)
	return normalizeSpace(title)
}

// readableText 提取网页可读文本
func readableText(root *html.Node) string {
	var parts []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "noscript" || n.Data == "title") {
			return
		}
		if n.Type == html.TextNode {
			if text := strings.TrimSpace(n.Data); text != "" {
				parts = append(parts, text)
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return normalizeSpace(strings.Join(parts, " "))
}

// normalizeSpace 规范化空格
func normalizeSpace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
