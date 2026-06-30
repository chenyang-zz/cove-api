/**
 * @Time   : 2026/6/27 16:20
 * @Author : chenyangzhao542@gmail.com
 * @File   : base.go.go
 **/

package response

type ListResponse[T any] struct {
	List []T `json:"list"`
}

type PageListResponse[T any] struct {
	Total    int64 `json:"total"`
	Page     int64 `json:"page"`
	PageSize int64 `json:"page_size"`
	List     []T   `json:"list"`
}
