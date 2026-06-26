package response

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/boxify/api-go/internal/xerr"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

var registerValidatorTagNamesOnce sync.Once

func RegisterValidatorTagNames() {
	registerValidatorTagNamesOnce.Do(func() {
		if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
			v.RegisterTagNameFunc(jsonTagName)
		}
	})
}

func validationFieldErrors(err error) []FieldError {
	appErr := xerr.From(err)
	if appErr == nil || appErr.Code != 40001 || appErr.Cause == nil {
		return nil
	}
	return fieldErrorsFromCause(appErr.Cause)
}

func fieldErrorsFromCause(err error) []FieldError {
	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) {
		out := make([]FieldError, 0, len(validationErrs))
		for _, item := range validationErrs {
			field := normalizeFieldName(item.Field())
			tag := item.Tag()
			param := item.Param()
			out = append(out, FieldError{
				Field:   field,
				Tag:     tag,
				Param:   param,
				Message: validationMessage(field, tag, param),
			})
		}
		return out
	}

	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return []FieldError{{
			Tag:     "json",
			Param:   strconv.FormatInt(syntaxErr.Offset, 10),
			Message: "请求体 JSON 格式错误",
		}}
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		field := normalizeFieldName(typeErr.Field)
		return []FieldError{{
			Field:   field,
			Tag:     "type",
			Message: validationMessage(field, "type", ""),
		}}
	}

	return []FieldError{{
		Tag:     "binding",
		Message: "请求参数错误",
	}}
}

func validationMessage(field, tag, param string) string {
	name := field
	if name == "" {
		name = "请求参数"
	}
	switch tag {
	case "required":
		return name + " 不能为空"
	case "min":
		return name + " 不能小于 " + param
	case "max":
		return name + " 不能大于 " + param
	case "oneof":
		return name + " 必须是以下值之一: " + param
	case "email":
		return name + " 必须是有效邮箱"
	case "type":
		return name + " 类型错误"
	case "json":
		return "请求体 JSON 格式错误"
	default:
		return name + " 参数错误"
	}
}

func jsonTagName(field reflect.StructField) string {
	for _, key := range []string{"json", "form", "query"} {
		if name := tagName(field.Tag.Get(key)); name != "" {
			return name
		}
	}
	return toSnakeCase(field.Name)
}

func tagName(tag string) string {
	if tag == "" {
		return ""
	}
	name := strings.Split(tag, ",")[0]
	if name == "-" {
		return ""
	}
	return name
}

func normalizeFieldName(field string) string {
	if field == "" {
		return ""
	}
	parts := strings.Split(field, ".")
	for i, part := range parts {
		parts[i] = toSnakeCase(part)
	}
	return strings.Join(parts, ".")
}

func toSnakeCase(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	var prevLowerOrDigit bool
	for i, r := range s {
		if r == '-' {
			b.WriteRune('_')
			prevLowerOrDigit = false
			continue
		}
		if unicode.IsUpper(r) {
			if i > 0 && prevLowerOrDigit {
				b.WriteRune('_')
			}
			b.WriteRune(unicode.ToLower(r))
			prevLowerOrDigit = false
			continue
		}
		b.WriteRune(r)
		prevLowerOrDigit = unicode.IsLower(r) || unicode.IsDigit(r)
	}
	return b.String()
}
