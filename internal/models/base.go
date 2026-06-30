/**
 * @Time   : 2026/6/30 22:36
 * @Author : chenyangzhao542@gmail.com
 * @File   : base.go
 **/

package models

import (
	"database/sql/driver"
	"encoding/json"
)

type JSONMap map[string]any

func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

func (m *JSONMap) Scan(value any) error {
	if value == nil {
		*m = JSONMap{}
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return nil
	}
	return json.Unmarshal(data, m)
}

type JSONMaps []map[string]any

func (m JSONMaps) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

func (m *JSONMaps) Scan(value any) error {
	if value == nil {
		*m = JSONMaps{}
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return nil
	}
	return json.Unmarshal(data, m)

}
