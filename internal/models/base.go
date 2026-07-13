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

// JSONStrings 表示 jsonb 字符串数组，用于图片 objects 等字段。
type JSONStrings []string

func (s JSONStrings) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

func (s *JSONStrings) Scan(value any) error {
	if value == nil {
		*s = JSONStrings{}
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
	// 优先按字符串数组解析。
	var strings []string
	if err := json.Unmarshal(data, &strings); err == nil {
		*s = strings
		return nil
	}
	// 兼容历史 map 数组形态：[{"name":"猫"}, ...]
	var maps []map[string]any
	if err := json.Unmarshal(data, &maps); err != nil {
		return err
	}
	out := make(JSONStrings, 0, len(maps))
	for _, item := range maps {
		if item == nil {
			continue
		}
		if name, ok := item["name"].(string); ok && name != "" {
			out = append(out, name)
		}
	}
	*s = out
	return nil
}
