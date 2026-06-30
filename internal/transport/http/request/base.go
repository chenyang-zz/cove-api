/**
 * @Time   : 2026/6/30 21:48
 * @Author : chenyangzhao542@gmail.com
 * @File   : base.go
 **/

package request

type PageRequest struct {
	Page     int64 `json:"page" binding:"required,ge=1"`
	PageSize int64 `json:"page_size" binding:"required,ge=1,le=100"`
}
