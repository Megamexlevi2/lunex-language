package runtime

import (
	"fmt"
	"lunex/internal/errfmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

func jsonStringify(val *Value, indent string, depth int) string {
	if val == nil {
		return "null"
	}
	switch val.Tag {
	case TypeNull, TypeUndefined:
		return "null"
	case TypeBool:
		if val.BoolVal {
			return "true"
		}
		return "false"
	case TypeNumber:
		if math.IsNaN(val.NumVal) || math.IsInf(val.NumVal, 0) {
			return "null"
		}
		if val.NumVal == math.Trunc(val.NumVal) {
			return fmt.Sprintf("%.0f", val.NumVal)
		}
		return strconv.FormatFloat(val.NumVal, 'f', -1, 64)
	case TypeString:
		return fmt.Sprintf("%q", val.StrVal)
	case TypeArray:
		if len(val.ArrVal) == 0 {
			return "[]"
		}
		var parts []string
		for _, el := range val.ArrVal {
			if el == nil {
				parts = append(parts, "null")
			} else {
				parts = append(parts, jsonStringify(el, indent, depth+1))
			}
		}
		if indent == "" {
			return "[" + strings.Join(parts, ",") + "]"
		}
		pad := strings.Repeat(indent, depth+1)
		return "[\n" + pad + strings.Join(parts, ",\n"+pad) + "\n" + strings.Repeat(indent, depth) + "]"
	case TypeObject:
		if len(val.ObjVal) == 0 {
			return "{}"
		}

		keys := make([]string, 0, len(val.ObjVal))
		for k, v := range val.ObjVal {
			if v == nil || v.Tag == TypeFunction {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var parts []string
		for _, k := range keys {
			v := val.ObjVal[k]
			key := fmt.Sprintf("%q", k)
			parts = append(parts, key+":"+jsonStringify(v, indent, depth+1))
		}
		if indent == "" {
			return "{" + strings.Join(parts, ",") + "}"
		}
		pad := strings.Repeat(indent, depth+1)
		return "{\n" + pad + strings.Join(parts, ",\n"+pad) + "\n" + strings.Repeat(indent, depth) + "}"
	case TypeInstance:
		obj := ObjectVal(nil)
		if val.InstVal != nil {
			obj.ObjVal = val.InstVal.OwnFields()
		}
		return jsonStringify(obj, indent, depth)
	default:
		return "null"
	}
}

func jsonParse(s string) (*Value, error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return Null, nil
	}
	switch s {
	case "null":
		return Null, nil
	case "true":
		return True, nil
	case "false":
		return False, nil
	}
	if s[0] == '"' {
		var str string
		if err := jsonUnquote(s, &str); err != nil {
			return nil, err
		}
		return StringVal(str), nil
	}
	if s[0] == '[' {
		return jsonParseArray(s)
	}
	if s[0] == '{' {
		return jsonParseObject(s)
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, &errfmt.LunexError{
			Message: fmt.Sprintf("invalid JSON value: %s", s),
			Kind:    errfmt.KindEncoding,
			Code:    "E0067",
		}
	}
	return NumberVal(f), nil
}

func jsonUnquote(s string, out *string) error {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return &errfmt.LunexError{
			Message: "invalid JSON string: value is not a quoted string",
			Kind:    errfmt.KindEncoding,
			Code:    "E0067",
		}
	}
	inner := s[1 : len(s)-1]

	if !strings.ContainsRune(inner, '\\') {
		*out = inner
		return nil
	}
	var buf strings.Builder
	buf.Grow(len(inner))
	for i := 0; i < len(inner); i++ {
		if inner[i] != '\\' || i+1 >= len(inner) {
			buf.WriteByte(inner[i])
			continue
		}
		i++
		switch inner[i] {
		case '"':
			buf.WriteByte('"')
		case '\\':
			buf.WriteByte('\\')
		case '/':
			buf.WriteByte('/')
		case 'n':
			buf.WriteByte('\n')
		case 'r':
			buf.WriteByte('\r')
		case 't':
			buf.WriteByte('\t')
		case 'b':
			buf.WriteByte('\b')
		case 'f':
			buf.WriteByte('\f')
		case 'u':
			if i+4 < len(inner) {
				r, err := strconv.ParseInt(inner[i+1:i+5], 16, 32)
				if err == nil {
					buf.WriteRune(rune(r))
					i += 4
					continue
				}
			}
			buf.WriteString(`\u`)
		default:
			buf.WriteByte('\\')
			buf.WriteByte(inner[i])
		}
	}
	*out = buf.String()
	return nil
}

func jsonParseArray(s string) (*Value, error) {
	if s == "[]" {
		return ArrayVal(nil), nil
	}
	inner := strings.TrimSpace(s[1 : len(s)-1])
	if inner == "" {
		return ArrayVal(nil), nil
	}
	parts := jsonSplit(inner)
	result := make([]*Value, len(parts))
	for i, p := range parts {
		v, err := jsonParse(strings.TrimSpace(p))
		if err != nil {
			return nil, err
		}
		result[i] = v
	}
	return ArrayVal(result), nil
}

func jsonParseObject(s string) (*Value, error) {
	if s == "{}" {
		return ObjectVal(nil), nil
	}
	inner := strings.TrimSpace(s[1 : len(s)-1])
	if inner == "" {
		return ObjectVal(nil), nil
	}
	obj := make(map[string]*Value)
	parts := jsonSplit(inner)
	for _, part := range parts {
		part = strings.TrimSpace(part)

		colonIdx := -1
		if len(part) > 0 && part[0] == '"' {
			for i := 1; i < len(part); i++ {
				if part[i] == '\\' {
					i++
				} else if part[i] == '"' {

					for j := i + 1; j < len(part); j++ {
						if part[j] == ':' {
							colonIdx = j
							break
						} else if part[j] != ' ' && part[j] != '\t' {
							break
						}
					}
					break
				}
			}
		}
		if colonIdx < 0 {
			colonIdx = strings.Index(part, ":")
		}
		if colonIdx < 0 {
			continue
		}
		key := strings.TrimSpace(part[:colonIdx])
		val := strings.TrimSpace(part[colonIdx+1:])
		var keyStr string
		if err := jsonUnquote(key, &keyStr); err != nil {
			keyStr = key
		}
		v, err := jsonParse(val)
		if err != nil {
			continue
		}
		obj[keyStr] = v
	}
	return ObjectVal(obj), nil
}

func jsonSplit(s string) []string {
	var parts []string
	depth := 0
	start := 0
	inStr := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if c == '\\' {
				i++
			} else if c == '"' {
				inStr = false
			}
		} else {
			switch c {
			case '"':
				inStr = true
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			case ',':
				if depth == 0 {
					parts = append(parts, s[start:i])
					start = i + 1
				}
			}
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}
