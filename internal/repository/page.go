package repository

type PageQuery struct {
	Page     int64
	PageSize int64
}

func (q PageQuery) LimitOffset(defaultPageSize int64) (limit int, offset int) {
	page := q.Page
	if page < 1 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize < 1 {
		pageSize = 20
	}
	return int(pageSize), int((page - 1) * pageSize)
}
