package router

import (
	"mime/multipart"

	"github.com/meetmonth/dance-elite/internal/service"
)

// formValue 区分"字段未提交"（nil，不修改）与"提交空串"（清空）。
func formValue(form *multipart.Form, name string) *string {
	values, ok := form.Value[name]
	if !ok {
		return nil
	}
	v := ""
	if len(values) > 0 {
		v = values[0]
	}
	return &v
}

// openFormFile 打开 multipart 中的首个文件；字段未提交时返回 (nil, nil, nil)。
func openFormFile(form *multipart.Form, field string) (*service.MaterialFile, func(), error) {
	files := form.File[field]
	if len(files) == 0 {
		return nil, nil, nil
	}
	fh := files[0]
	f, err := fh.Open()
	if err != nil {
		return nil, nil, err
	}
	return &service.MaterialFile{Filename: fh.Filename, Size: fh.Size, Content: f},
		func() { _ = f.Close() }, nil
}
