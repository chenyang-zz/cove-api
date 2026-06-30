/**
 * @Time   : 2026/6/30 21:48
 * @Author : chenyangzhao542@gmail.com
 * @File   : base.go
 **/

package request

type PageRequest struct {
	Page     int64 `json:"page" form:"page" binding:"required,gte=1"`
	PageSize int64 `json:"page_size" form:"page_size" binding:"required,gte=1,lte=100"`
}
