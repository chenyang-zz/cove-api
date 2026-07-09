package main

import (
	"fmt"
	"sort"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/Masterminds/sprig/v3"
)

type promptFieldKind uint8

const (
	promptFieldUnknown promptFieldKind = iota
	promptFieldScalar
	promptFieldObject
	promptFieldSliceScalar
	promptFieldSliceObject
)

type promptField struct {
	Name     string
	Kind     promptFieldKind
	Children []*promptField
	childMap map[string]*promptField
}

type promptInferer struct {
	name string
	root *promptField
}

func inferPromptFields(name string, file string, text string) ([]*promptField, error) {
	tpl, err := template.New(file).Funcs(sprig.TxtFuncMap()).Parse(text)
	if err != nil {
		return nil, fmt.Errorf("parse prompt %s template %s: %w", name, file, err)
	}
	inferer := &promptInferer{
		name: name,
		root: &promptField{Kind: promptFieldObject, childMap: make(map[string]*promptField)},
	}
	if err := inferer.walkList(tpl.Tree.Root, nil); err != nil {
		return nil, fmt.Errorf("infer prompt %s: %w", name, err)
	}
	inferer.finalize(inferer.root)
	return inferer.root.Children, nil
}

func (i *promptInferer) walkList(list *parse.ListNode, rangePath []string) error {
	if list == nil {
		return nil
	}
	for _, node := range list.Nodes {
		switch value := node.(type) {
		case *parse.ActionNode:
			if err := i.walkPipe(value.Pipe, rangePath, false); err != nil {
				return err
			}
		case *parse.IfNode:
			if err := i.walkPipe(value.Pipe, rangePath, false); err != nil {
				return err
			}
			if err := i.walkList(value.List, rangePath); err != nil {
				return err
			}
			if err := i.walkList(value.ElseList, rangePath); err != nil {
				return err
			}
		case *parse.WithNode:
			if err := i.walkPipe(value.Pipe, rangePath, false); err != nil {
				return err
			}
			if err := i.walkList(value.List, rangePath); err != nil {
				return err
			}
			if err := i.walkList(value.ElseList, rangePath); err != nil {
				return err
			}
		case *parse.RangeNode:
			path, err := fieldPathFromPipe(value.Pipe)
			if err != nil {
				return err
			}
			if len(path) == 0 {
				return fmt.Errorf("range expression must reference a template field")
			}
			if err := i.observe(path, promptFieldSliceScalar); err != nil {
				return err
			}
			if err := i.walkList(value.List, path); err != nil {
				return err
			}
			if err := i.walkList(value.ElseList, rangePath); err != nil {
				return err
			}
		case *parse.TemplateNode:
			return fmt.Errorf("nested template invocation %q is not supported", value.Name)
		}
	}
	return nil
}

func (i *promptInferer) walkPipe(pipe *parse.PipeNode, rangePath []string, rangeSelector bool) error {
	if pipe == nil {
		return nil
	}
	for _, command := range pipe.Cmds {
		if len(command.Args) == 0 {
			continue
		}
		if identifier, ok := command.Args[0].(*parse.IdentifierNode); ok && identifier.Ident == "index" {
			return fmt.Errorf("dynamic index expressions are not supported")
		}
		for argIndex, argument := range command.Args {
			switch value := argument.(type) {
			case *parse.FieldNode:
				path := append([]string(nil), value.Ident...)
				if len(rangePath) > 0 {
					path = append(append([]string(nil), rangePath...), path...)
					if err := i.observe(rangePath, promptFieldSliceObject); err != nil {
						return err
					}
				}
				kind := promptFieldScalar
				if argIndex > 0 && isJoinCommand(command) {
					kind = promptFieldSliceScalar
				}
				if err := i.observe(path, kind); err != nil {
					return err
				}
			case *parse.ChainNode:
				field, ok := value.Node.(*parse.FieldNode)
				if !ok {
					return fmt.Errorf("chain expression %s cannot be inferred", value.String())
				}
				path := append(append([]string(nil), field.Ident...), value.Field...)
				if err := i.observe(path, promptFieldScalar); err != nil {
					return err
				}
			case *parse.DotNode:
				if len(rangePath) > 0 {
					if err := i.observe(rangePath, promptFieldSliceScalar); err != nil {
						return err
					}
				}
			case *parse.VariableNode:
				// range 中的 $index/$value 只引用集合元素，不产生新的根参数。
				continue
			}
		}
	}
	return nil
}

func isJoinCommand(command *parse.CommandNode) bool {
	if len(command.Args) == 0 {
		return false
	}
	identifier, ok := command.Args[0].(*parse.IdentifierNode)
	return ok && identifier.Ident == "join"
}

func fieldPathFromPipe(pipe *parse.PipeNode) ([]string, error) {
	if pipe == nil || len(pipe.Cmds) != 1 || len(pipe.Cmds[0].Args) != 1 {
		return nil, fmt.Errorf("range expression is too dynamic to infer")
	}
	field, ok := pipe.Cmds[0].Args[0].(*parse.FieldNode)
	if !ok {
		return nil, fmt.Errorf("range expression %s is too dynamic to infer", pipe.String())
	}
	return append([]string(nil), field.Ident...), nil
}

func (i *promptInferer) observe(path []string, kind promptFieldKind) error {
	if len(path) == 0 {
		return nil
	}
	current := i.root
	for index, name := range path {
		child := current.child(name)
		last := index == len(path)-1
		if last {
			if err := mergePromptFieldKind(child, kind); err != nil {
				return fmt.Errorf("field %s conflict: %w", strings.Join(path, "."), err)
			}
			return nil
		}
		switch child.Kind {
		case promptFieldUnknown, promptFieldObject:
			child.Kind = promptFieldObject
		case promptFieldSliceScalar:
			child.Kind = promptFieldSliceObject
		case promptFieldSliceObject:
			// 集合元素已经被识别为结构体，继续向其字段树下钻。
		default:
			return fmt.Errorf(
				"field %s conflict: %s cannot also be object",
				strings.Join(path[:index+1], "."),
				promptFieldKindName(child.Kind),
			)
		}
		current = child
	}
	return nil
}

func (f *promptField) child(name string) *promptField {
	if f.childMap == nil {
		f.childMap = make(map[string]*promptField)
	}
	if child, ok := f.childMap[name]; ok {
		return child
	}
	child := &promptField{Name: name, childMap: make(map[string]*promptField)}
	f.childMap[name] = child
	f.Children = append(f.Children, child)
	return child
}

func mergePromptFieldKind(field *promptField, next promptFieldKind) error {
	if field.Kind == promptFieldUnknown || field.Kind == next {
		field.Kind = next
		return nil
	}
	if field.Kind == promptFieldSliceScalar && next == promptFieldSliceObject {
		field.Kind = promptFieldSliceObject
		return nil
	}
	if field.Kind == promptFieldSliceObject && next == promptFieldSliceScalar {
		return nil
	}
	return fmt.Errorf("%s cannot also be %s", promptFieldKindName(field.Kind), promptFieldKindName(next))
}

func promptFieldKindName(kind promptFieldKind) string {
	switch kind {
	case promptFieldScalar:
		return "scalar"
	case promptFieldObject:
		return "object"
	case promptFieldSliceScalar:
		return "string slice"
	case promptFieldSliceObject:
		return "object slice"
	default:
		return "unknown"
	}
}

func (i *promptInferer) finalize(field *promptField) {
	if field.Kind == promptFieldUnknown {
		field.Kind = promptFieldScalar
	}
	sort.Slice(field.Children, func(a, b int) bool {
		return field.Children[a].Name < field.Children[b].Name
	})
	for _, child := range field.Children {
		i.finalize(child)
	}
}
