package uploadfile

import (
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"

	"github.com/boxify/api-go/internal/xerr"
)

type UploadedFileInfo struct {
	FileName string
	Ext      string
	Size     int64
	Content  []byte
}

func Read(file *multipart.FileHeader, maxSize int64, sizeMessage string, readMessage string) (*UploadedFileInfo, error) {
	if file == nil {
		return nil, xerr.BadRequest("上传文件不能为空")
	}
	if maxSize > 0 && file.Size > maxSize {
		return nil, xerr.BadRequest(sizeMessage)
	}

	opened, err := file.Open()
	if err != nil {
		return nil, xerr.Wrap(err, readMessage)
	}
	defer opened.Close()

	content, err := io.ReadAll(opened)
	if err != nil {
		return nil, xerr.Wrap(err, readMessage)
	}
	if maxSize > 0 && int64(len(content)) > maxSize {
		return nil, xerr.BadRequest(sizeMessage)
	}

	fileName := strings.TrimSpace(filepath.Base(file.Filename))
	return &UploadedFileInfo{
		FileName: fileName,
		Ext:      strings.ToLower(filepath.Ext(fileName)),
		Size:     int64(len(content)),
		Content:  content,
	}, nil
}
