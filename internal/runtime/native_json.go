package runtime

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// 类型常量别名，方便使用
const (
	ValNull       = bytecode.ValNull
	ValBool       = bytecode.ValBool
	ValInt        = bytecode.ValInt
	ValFloat      = bytecode.ValFloat
	ValString     = bytecode.ValString
	ValArray      = bytecode.ValArray
	ValSuperArray = bytecode.ValSuperArray
	ValObject     = bytecode.ValObject
)

// ============================================================================
// JSON Native Functions
// ============================================================================

// nativeJsonEncode 将Sola值编码为JSON字符串
// native_json_encode(value, pretty bool, indent string) -> string
func nativeJsonEncode(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewString("")
	}

	value := args[0]
	pretty := false
	indent := "  "

	if len(args) >= 2 {
		if b, ok := args[1].Data.(bool); ok {
			pretty = b
		}
	}
	if len(args) >= 3 {
		if s, ok := args[2].Data.(string); ok {
			indent = s
		}
	}

	// 转换为Go值
	goValue := solaValueToGo(value)

	// 编码为JSON
	var result []byte
	var err error
	if pretty {
		result, err = json.MarshalIndent(goValue, "", indent)
	} else {
		result, err = json.Marshal(goValue)
	}

	if err != nil {
		return bytecode.NewString("")
	}

	return bytecode.NewString(string(result))
}

// nativeJsonDecode 将JSON字符串解码为Sola值
// native_json_decode(json string) -> dynamic
func nativeJsonDecode(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.Value{Type: ValNull}
	}

	jsonStr, ok := args[0].Data.(string)
	if !ok {
		return bytecode.Value{Type: ValNull}
	}

	// 解码JSON
	var goValue interface{}
	err := json.Unmarshal([]byte(jsonStr), &goValue)
	if err != nil {
		return bytecode.Value{Type: ValNull}
	}

	// 转换为Sola值
	return goValueToSola(goValue)
}

// nativeJsonIsValid 检查字符串是否为有效JSON
// native_json_is_valid(json string) -> bool
func nativeJsonIsValid(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewBool(false)
	}

	jsonStr, ok := args[0].Data.(string)
	if !ok {
		return bytecode.NewBool(false)
	}

	return bytecode.NewBool(json.Valid([]byte(jsonStr)))
}

// nativeJsonEncodeObject 将Sola对象编码为JSON（支持注解）
// native_json_encode_object(obj Object, options map) -> string
func nativeJsonEncodeObject(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewString("{}")
	}

	obj, ok := args[0].Data.(*bytecode.Object)
	if !ok {
		// 如果不是对象，回退到普通编码
		return nativeJsonEncode(args)
	}

	pretty := false
	indent := "  "
	namingStrategy := 0 // 0=none, 1=snake, 2=camel, 3=pascal, 4=kebab

	// 解析选项
	if len(args) >= 2 {
		if opts, ok := args[1].Data.(*bytecode.SuperArray); ok {
			if v, exists := opts.Get(bytecode.NewString("pretty")); exists {
				if b, ok := v.Data.(bool); ok {
					pretty = b
				}
			}
			if v, exists := opts.Get(bytecode.NewString("indent")); exists {
				if s, ok := v.Data.(string); ok {
					indent = s
				}
			}
			if v, exists := opts.Get(bytecode.NewString("naming")); exists {
				if n, ok := v.Data.(int64); ok {
					namingStrategy = int(n)
				}
			}
		}
	}

	// 编码对象
	result := encodeObjectToJson(obj, namingStrategy)

	// 格式化
	var jsonBytes []byte
	var err error
	if pretty {
		jsonBytes, err = json.MarshalIndent(result, "", indent)
	} else {
		jsonBytes, err = json.Marshal(result)
	}

	if err != nil {
		return bytecode.NewString("{}")
	}

	return bytecode.NewString(string(jsonBytes))
}

// ============================================================================
// Helper Functions: Sola -> Go
// ============================================================================

// solaValueToGo 将Sola值转换为Go值（用于JSON编码）
func solaValueToGo(v bytecode.Value) interface{} {
	switch v.Type {
	case ValNull:
		return nil
	case ValBool:
		return v.Data.(bool)
	case ValInt:
		return v.Data.(int64)
	case ValFloat:
		return v.Data.(float64)
	case ValString:
		return v.Data.(string)
	case ValArray, ValSuperArray:
		return superArrayToGo(v.Data.(*bytecode.SuperArray))
	case ValObject:
		return objectToGo(v.Data.(*bytecode.Object))
	default:
		return nil
	}
}

// superArrayToGo 将SuperArray转换为Go的slice或map
func superArrayToGo(arr *bytecode.SuperArray) interface{} {
	if arr == nil || arr.Len() == 0 {
		// 空数组返回空slice
		return []interface{}{}
	}

	// 检查是否是纯整数键且连续（从0开始）
	isSequential := true
	maxIndex := int64(-1)

	for _, entry := range arr.Entries {
		if entry.Key.Type != ValInt {
			isSequential = false
			break
		}
		idx := entry.Key.Data.(int64)
		if idx > maxIndex {
			maxIndex = idx
		}
	}

	// 检查是否从0开始连续
	if isSequential && maxIndex >= 0 {
		expectedLen := maxIndex + 1
		if int64(arr.Len()) != expectedLen {
			isSequential = false
		} else {
			// 检查是否有所有索引0到maxIndex
			for i := int64(0); i <= maxIndex; i++ {
				found := false
			for _, entry := range arr.Entries {
				if entry.Key.Type == ValInt && entry.Key.Data.(int64) == i {
					found = true
					break
				}
			}
				if !found {
					isSequential = false
					break
				}
			}
		}
	}

	if isSequential && maxIndex >= 0 {
		// 转换为slice
		result := make([]interface{}, maxIndex+1)
		for _, entry := range arr.Entries {
			idx := entry.Key.Data.(int64)
			result[idx] = solaValueToGo(entry.Value)
		}
		return result
	}

	// 转换为map
	result := make(map[string]interface{})
	for _, entry := range arr.Entries {
		key := valueToString(entry.Key)
		result[key] = solaValueToGo(entry.Value)
	}
	return result
}

// objectToGo 将Sola对象转换为Go的map（只包含公开属性）
func objectToGo(obj *bytecode.Object) map[string]interface{} {
	result := make(map[string]interface{})
	if obj == nil {
		return result
	}

	for name, value := range obj.Fields {
		// 跳过私有属性（以_开头的视为私有，或检查类定义）
		// 这里简单处理：所有字段都序列化
		result[name] = solaValueToGo(value)
	}

	return result
}

// encodeObjectToJson 编码对象为JSON（支持注解和命名策略）
func encodeObjectToJson(obj *bytecode.Object, namingStrategy int) map[string]interface{} {
	result := make(map[string]interface{})
	if obj == nil {
		return result
	}

	// 获取类定义以读取注解
	class := obj.Class

	for fieldName, value := range obj.Fields {
		jsonName := fieldName
		omitEmpty := false
		ignore := false
		asString := false

		// 检查属性注解
		if class != nil && class.PropAnnotations != nil {
			if annotations, ok := class.PropAnnotations[fieldName]; ok {
				for _, ann := range annotations {
					switch ann.Name {
					case "JsonIgnore":
						ignore = true
					case "JsonProperty":
						// 支持位置参数（key="0"）或命名参数（key="name"）
						if arg, ok := ann.Args["0"]; ok {
							if s, ok := arg.Data.(string); ok {
								jsonName = s
							}
						} else if arg, ok := ann.Args["name"]; ok {
							if s, ok := arg.Data.(string); ok {
								jsonName = s
							}
						}
					case "JsonOmitEmpty":
						omitEmpty = true
					case "JsonString":
						asString = true
					}
				}
			}
		}

		// 忽略字段
		if ignore {
			continue
		}

		// 检查空值
		if omitEmpty && isEmptyValue(value) {
			continue
		}

		// 应用命名策略（如果没有显式指定JsonProperty）
		if jsonName == fieldName && namingStrategy > 0 {
			jsonName = applyNamingStrategy(fieldName, namingStrategy)
		}

		// 转换值
		goValue := solaValueToGo(value)

		// 数字转字符串
		if asString {
			switch v := goValue.(type) {
			case int64:
				goValue = fmt.Sprintf("%d", v)
			case float64:
				goValue = fmt.Sprintf("%v", v)
			}
		}

		result[jsonName] = goValue
	}

	return result
}

// isEmptyValue 检查值是否为空
func isEmptyValue(v bytecode.Value) bool {
	switch v.Type {
	case ValNull:
		return true
	case ValBool:
		return false // bool never considered empty
	case ValInt:
		return v.Data.(int64) == 0
	case ValFloat:
		return v.Data.(float64) == 0
	case ValString:
		return v.Data.(string) == ""
	case ValArray, ValSuperArray:
		if arr, ok := v.Data.(*bytecode.SuperArray); ok {
			return arr.Len() == 0
		}
		return true
	default:
		return false
	}
}

// applyNamingStrategy 应用命名策略
func applyNamingStrategy(name string, strategy int) string {
	switch strategy {
	case 1: // SNAKE_CASE
		return toSnakeCase(name)
	case 2: // CAMEL_CASE
		return toCamelCase(name)
	case 3: // PASCAL_CASE
		return toPascalCase(name)
	case 4: // KEBAB_CASE
		return toKebabCase(name)
	default:
		return name
	}
}

// toSnakeCase 转换为snake_case
func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				result.WriteByte('_')
			}
			result.WriteByte(byte(r + 32)) // 转小写
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// toCamelCase 转换为camelCase
func toCamelCase(s string) string {
	parts := strings.Split(s, "_")
	if len(parts) == 0 {
		return s
	}
	result := strings.ToLower(parts[0])
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			result += strings.ToUpper(parts[i][:1]) + strings.ToLower(parts[i][1:])
		}
	}
	return result
}

// toPascalCase 转换为PascalCase
func toPascalCase(s string) string {
	parts := strings.Split(s, "_")
	var result string
	for _, part := range parts {
		if len(part) > 0 {
			result += strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
		}
	}
	return result
}

// toKebabCase 转换为kebab-case
func toKebabCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				result.WriteByte('-')
			}
			result.WriteByte(byte(r + 32))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// valueToString 将值转换为字符串（用作map键）
func valueToString(v bytecode.Value) string {
	switch v.Type {
	case ValInt:
		return fmt.Sprintf("%d", v.Data.(int64))
	case ValString:
		return v.Data.(string)
	default:
		return fmt.Sprintf("%v", v.Data)
	}
}

// ============================================================================
// Helper Functions: Go -> Sola
// ============================================================================

// goValueToSola 将Go值转换为Sola值（用于JSON解码）
func goValueToSola(v interface{}) bytecode.Value {
	if v == nil {
		return bytecode.Value{Type: ValNull}
	}

	switch val := v.(type) {
	case bool:
		return bytecode.NewBool(val)
	case float64:
		// JSON数字默认解码为float64
		// 检查是否为整数
		if val == float64(int64(val)) {
			return bytecode.NewInt(int64(val))
		}
		return bytecode.NewFloat(val)
	case string:
		return bytecode.NewString(val)
	case []interface{}:
		return goSliceToSuperArray(val)
	case map[string]interface{}:
		return goMapToSuperArray(val)
	default:
		return bytecode.Value{Type: ValNull}
	}
}

// goSliceToSuperArray 将Go slice转换为SuperArray（整数键）
func goSliceToSuperArray(slice []interface{}) bytecode.Value {
	arr := bytecode.NewSuperArray()
	for i, v := range slice {
		arr.Set(bytecode.NewInt(int64(i)), goValueToSola(v))
	}
	return bytecode.Value{Type: ValSuperArray, Data: arr}
}

// goMapToSuperArray 将Go map转换为SuperArray（字符串键）
func goMapToSuperArray(m map[string]interface{}) bytecode.Value {
	arr := bytecode.NewSuperArray()
	
	// 为了保持一致的输出顺序，对键排序
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	
	for _, k := range keys {
		arr.Set(bytecode.NewString(k), goValueToSola(m[k]))
	}
	return bytecode.Value{Type: ValSuperArray, Data: arr}
}


