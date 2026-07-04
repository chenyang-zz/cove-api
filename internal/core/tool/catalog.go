package tool

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type catalogEntry struct {
	descriptor SetDescriptor
	set        ToolSet
}

// Catalog 管理多个工具集，并按选择条件展开成 Registry。
//
// Catalog 可以并发注册和读取。它不执行工具，只负责工具集的组织、筛选和扁平化。
type Catalog struct {
	mu      sync.RWMutex
	entries map[string]catalogEntry
}

// NewCatalog 创建空工具集目录。
func NewCatalog() *Catalog {
	return &Catalog{
		entries: make(map[string]catalogEntry),
	}
}

// RegisterSet 注册一个工具集。
//
// RegisterSet 会调用 set.Describe(ctx) 获取名称并校验唯一性。nil 工具集、空名称、
// 描述失败和重复名称都会返回错误。
func (c *Catalog) RegisterSet(ctx context.Context, set ToolSet) error {
	if c == nil {
		return errors.New("catalog is nil")
	}
	if set == nil {
		return errors.New("tool set is nil")
	}

	descriptor, err := set.Describe(ctx)
	if err != nil {
		return fmt.Errorf("describe tool set: %w", err)
	}
	descriptor.Name = strings.TrimSpace(descriptor.Name)
	if descriptor.Name == "" {
		return errors.New("tool set name is empty")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[descriptor.Name]; ok {
		return fmt.Errorf("tool set %q already registered", descriptor.Name)
	}
	c.entries[descriptor.Name] = catalogEntry{
		descriptor: cloneSetDescriptor(descriptor),
		set:        set,
	}
	return nil
}

// ListSets 返回工具集描述清单。
//
// 返回结果按工具集名称升序排列，便于 UI 展示、缓存和测试保持稳定。
func (c *Catalog) ListSets(ctx context.Context) ([]SetDescriptor, error) {
	if c == nil {
		return nil, errors.New("catalog is nil")
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	names := c.sortedNamesLocked(nil)
	descriptors := make([]SetDescriptor, 0, len(names))
	for _, name := range names {
		descriptors = append(descriptors, cloneSetDescriptor(c.entries[name].descriptor))
	}
	return descriptors, nil
}

// BuildRegistry 按 selection 展开工具集，并返回扁平 Registry。
//
// selection 的空字段表示不限制对应维度。多个工具集展开后出现同名工具时，
// BuildRegistry 会返回错误，不会自动重命名。
func (c *Catalog) BuildRegistry(ctx context.Context, selection Selection) (*Registry, error) {
	if c == nil {
		return nil, errors.New("catalog is nil")
	}

	setFilter := namesSet(selection.SetNames)
	toolFilter := namesSet(selection.ToolNames)

	entries, err := c.selectedEntries(setFilter)
	if err != nil {
		return nil, err
	}
	registry := NewRegistry()
	seenTools := make(map[string]struct{}, len(toolFilter))
	for _, entry := range entries {
		tools, err := entry.set.Tools(ctx)
		if err != nil {
			return nil, fmt.Errorf("list tools from set %q: %w", entry.descriptor.Name, err)
		}
		for _, item := range tools {
			descriptor, err := item.Describe(ctx)
			if err != nil {
				return nil, fmt.Errorf("describe tool from set %q: %w", entry.descriptor.Name, err)
			}
			descriptor.Name = strings.TrimSpace(descriptor.Name)
			if len(toolFilter) > 0 {
				if _, ok := toolFilter[descriptor.Name]; !ok {
					continue
				}
				seenTools[descriptor.Name] = struct{}{}
			}
			if err := registry.Register(ctx, item); err != nil {
				return nil, fmt.Errorf("register tool %q from set %q: %w", descriptor.Name, entry.descriptor.Name, err)
			}
		}
	}
	if err := ensureAllSelected("tool", toolFilter, seenTools); err != nil {
		return nil, err
	}
	return registry, nil
}

func (c *Catalog) selectedEntries(setFilter map[string]struct{}) ([]catalogEntry, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	names := c.sortedNamesLocked(setFilter)
	seenSets := make(map[string]struct{}, len(setFilter))
	entries := make([]catalogEntry, 0, len(names))
	for _, name := range names {
		seenSets[name] = struct{}{}
		entries = append(entries, c.entries[name])
	}
	if err := ensureAllSelected("tool set", setFilter, seenSets); err != nil {
		return nil, err
	}
	return entries, nil
}

// sortedNamesLocked 按名称升序排序工具集名称。
func (c *Catalog) sortedNamesLocked(filter map[string]struct{}) []string {
	// 先排序工具集名称，再展开工具，避免 map 迭代顺序影响模型可见的工具顺序。
	names := make([]string, 0, len(c.entries))
	for name := range c.entries {
		if len(filter) > 0 {
			if _, ok := filter[name]; !ok {
				continue
			}
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// namesSet 转换为 map[string]struct{} 以便快速查找。
func namesSet(names []string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out[name] = struct{}{}
	}
	return out
}

// ensureAllSelected 确保所有选择的工具或工具集都已展开。
func ensureAllSelected(kind string, selected map[string]struct{}, seen map[string]struct{}) error {
	if len(selected) == 0 {
		return nil
	}
	missing := make([]string, 0)
	for name := range selected {
		if _, ok := seen[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("%s %q not found", kind, missing[0])
}
