package uploadfile_test

import (
	"bytes"
	"mime/multipart"
	"testing"

	"github.com/boxify/api-go/internal/util/uploadfile"
)

func testUploadFileHeader(t *testing.T, name string, content []byte) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	reader := multipart.NewReader(&body, writer.Boundary())
	form, err := reader.ReadForm(int64(len(content)) + 1024)
	if err != nil {
		t.Fatalf("read form: %v", err)
	}
	return form.File["file"][0]
}

func TestReadNormalizesNameAndReturnsFileInfo(t *testing.T) {
	// 验证上传文件读取会规范化文件名、扩展名，并返回内容和真实大小。
	file := testUploadFileHeader(t, "  ../Report.MD  ", []byte("hello"))

	info, err := uploadfile.Read(file, 10, "too large", "read failed")
	if err != nil {
		t.Fatalf("Read error = %v", err)
	}
	if info.FileName != "Report.MD" || info.Ext != ".md" || info.Size != 5 || string(info.Content) != "hello" {
		t.Fatalf("info = %+v content=%q, want normalized metadata and content", info, string(info.Content))
	}
}

func TestReadRejectsMissingAndOversizedFiles(t *testing.T) {
	// 验证上传文件读取会拒绝空文件、header 超限和实际内容超限。
	if _, err := uploadfile.Read(nil, 10, "too large", "read failed"); err == nil {
		t.Fatal("Read nil error = nil, want error")
	}

	headerTooLarge := testUploadFileHeader(t, "a.txt", []byte("abc"))
	headerTooLarge.Size = 11
	if _, err := uploadfile.Read(headerTooLarge, 10, "too large", "read failed"); err == nil {
		t.Fatal("Read header too large error = nil, want error")
	}

	contentTooLarge := testUploadFileHeader(t, "a.txt", []byte("abcdef"))
	contentTooLarge.Size = 1
	if _, err := uploadfile.Read(contentTooLarge, 5, "too large", "read failed"); err == nil {
		t.Fatal("Read content too large error = nil, want error")
	}
}
