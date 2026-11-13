//go:build miniapps

package miniapps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

var templatePattern = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

// ResolveValue resolves a typed reference or a string template against the
// current run references. The package-private form is used by runner.go.
func ResolveValue(value any, refs map[string]any) (any, error) {
	return resolveValue(value, refs)
}

func resolveValue(value any, refs map[string]any) (any, error) {
	switch typed := value.(type) {
	case Ref:
		return resolvePath(refs, typed.Ref)
	case *Ref:
		if typed == nil {
			return nil, errors.New("nil reference")
		}
		return resolvePath(refs, typed.Ref)
	case map[string]any:
		if raw, ok := typed["$ref"]; ok {
			path, ok := raw.(string)
			if !ok || len(typed) != 1 {
				return nil, errors.New("$ref must be the only string field in a reference object")
			}
			return resolvePath(refs, path)
		}
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			resolved, err := resolveValue(item, refs)
			if err != nil {
				return nil, err
			}
			out[key] = resolved
		}
		return out, nil
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			resolved, err := resolveValue(item, refs)
			if err != nil {
				return nil, err
			}
			out[index] = resolved
		}
		return out, nil
	case string:
		return interpolate(typed, refs)
	default:
		return value, nil
	}
}

func resolvePath(refs map[string]any, path string) (any, error) {
	path = strings.TrimSpace(path)
	if path == "" || strings.HasPrefix(path, ".") || strings.HasSuffix(path, ".") || strings.Contains(path, "..") {
		return nil, fmt.Errorf("invalid reference path %q", path)
	}
	parts := strings.Split(path, ".")
	var current any = refs
	for _, part := range parts {
		if part == "" || part == "$ref" {
			return nil, fmt.Errorf("invalid reference path %q", path)
		}
		var ok bool
		switch object := current.(type) {
		case map[string]any:
			current, ok = object[part]
		case map[string]string:
			var stringValue string
			stringValue, ok = object[part]
			current = stringValue
		default:
			value := reflect.ValueOf(current)
			if value.IsValid() && value.Kind() == reflect.Map && value.Type().Key().Kind() == reflect.String {
				item := value.MapIndex(reflect.ValueOf(part).Convert(value.Type().Key()))
				if item.IsValid() {
					ok = true
					current = item.Interface()
				}
			}
		}
		if !ok {
			return nil, fmt.Errorf("reference %q is not available", path)
		}
	}
	return current, nil
}

func interpolate(template string, refs map[string]any) (string, error) {
	var firstErr error
	result := templatePattern.ReplaceAllStringFunc(template, func(match string) string {
		if firstErr != nil {
			return ""
		}
		parts := templatePattern.FindStringSubmatch(match)
		value, err := resolvePath(refs, strings.TrimSpace(parts[1]))
		if err != nil {
			firstErr = err
			return ""
		}
		return fmt.Sprint(value)
	})
	if firstErr != nil {
		return "", firstErr
	}
	return result, nil
}

// EvaluateCondition evaluates the restricted v1 condition language.
func EvaluateCondition(condition Condition, refs map[string]any) (bool, error) {
	return evaluateCondition(condition, refs)
}

// ExecuteCondition is a context-aware adapter for callers that already carry
// a run context. Conditions are pure, but cancellation should still be
// observed before expensive nested evaluation.
func ExecuteCondition(ctx context.Context, condition Condition, refs map[string]any) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return evaluateCondition(condition, refs)
}

func evaluateCondition(condition Condition, refs map[string]any) (bool, error) {
	switch condition.Op {
	case "and", "or":
		if len(condition.Args) == 0 {
			return false, fmt.Errorf("%s requires at least one argument", condition.Op)
		}
		result := condition.Op == "and"
		for _, child := range condition.Args {
			value, err := evaluateCondition(child, refs)
			if err != nil {
				return false, err
			}
			if condition.Op == "and" {
				result = result && value
			} else {
				result = result || value
			}
		}
		return result, nil
	case "not":
		if len(condition.Args) != 1 {
			return false, errors.New("not requires one argument")
		}
		value, err := evaluateCondition(condition.Args[0], refs)
		return !value, err
	}

	leftRaw := condition.Left
	if condition.Value != nil {
		leftRaw = condition.Value
	}
	left, err := resolveValue(leftRaw, refs)
	if err != nil {
		if condition.Op == "exists" {
			return false, nil
		}
		return false, err
	}
	if condition.Op == "exists" {
		return left != nil, nil
	}
	if condition.Op == "empty" {
		return !truthy(left), nil
	}
	right, err := resolveValue(condition.Right, refs)
	if err != nil {
		return false, err
	}
	switch condition.Op {
	case "eq":
		return equalJSON(left, right), nil
	case "ne":
		return !equalJSON(left, right), nil
	case "gt", "gte", "lt", "lte":
		value, err := binaryOp(condition.Op, left, right)
		if err != nil {
			return false, err
		}
		return value.(bool), nil
	case "contains":
		if stringValue, ok := left.(string); ok {
			return strings.Contains(stringValue, fmt.Sprint(right)), nil
		}
		value := reflect.ValueOf(left)
		if value.IsValid() && (value.Kind() == reflect.Array || value.Kind() == reflect.Slice) {
			for index := 0; index < value.Len(); index++ {
				if equalJSON(value.Index(index).Interface(), right) {
					return true, nil
				}
			}
		}
		return false, nil
	case "matches":
		pattern := fmt.Sprint(right)
		if len(pattern) > 4096 {
			return false, errors.New("condition pattern exceeds 4096 bytes")
		}
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return false, err
		}
		return compiled.MatchString(fmt.Sprint(left)), nil
	default:
		return false, fmt.Errorf("unsupported condition %q", condition.Op)
	}
}

func equalJSON(left, right any) bool {
	leftRaw, leftErr := json.Marshal(left)
	rightRaw, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftRaw) == string(rightRaw)
}

func number(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, !math.IsNaN(typed) && !math.IsInf(typed, 0)
	case float32:
		return float64(typed), !math.IsNaN(float64(typed)) && !math.IsInf(float64(typed), 0)
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case json.Number:
		parsed, err := strconv.ParseFloat(string(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func binaryOp(op string, left, right any) (any, error) {
	leftNumber, leftOK := number(left)
	rightNumber, rightOK := number(right)
	if !leftOK || !rightOK {
		return nil, fmt.Errorf("%s requires numeric operands", op)
	}
	switch op {
	case "gt":
		return leftNumber > rightNumber, nil
	case "gte":
		return leftNumber >= rightNumber, nil
	case "lt":
		return leftNumber < rightNumber, nil
	case "lte":
		return leftNumber <= rightNumber, nil
	default:
		return nil, fmt.Errorf("unsupported binary operation %q", op)
	}
}

func truthy(value any) bool {
	if value == nil {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return typed != ""
	case float64:
		return typed != 0 && !math.IsNaN(typed)
	case float32:
		return typed != 0 && !math.IsNaN(float64(typed))
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return reflected.Len() > 0
	case reflect.Bool:
		return reflected.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return reflected.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return reflected.Uint() != 0
	}
	return true
}

func deepCopyValue[T any](value T) T {
	var out T
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return value
	}
	return out
}

// validateJSONType implements the small schema subset needed by declared
// output contracts. Unknown schema keywords are ignored by design; schema
// shape itself is checked by Validate before execution.
func validateJSONType(value any, schema map[string]any) error {
	if len(schema) == 0 {
		return nil
	}
	typeName, _ := schema["type"].(string)
	if typeName != "" && !jsonTypeMatches(value, typeName) {
		return fmt.Errorf("value does not match JSON type %q", typeName)
	}
	if required, ok := schema["required"].([]any); ok {
		object, objectOK := value.(map[string]any)
		if !objectOK {
			return errors.New("required fields require an object")
		}
		for _, item := range required {
			name, _ := item.(string)
			if name == "" {
				continue
			}
			if _, exists := object[name]; !exists {
				return fmt.Errorf("required field %q is missing", name)
			}
		}
	}
	if enum, ok := schema["enum"].([]any); ok && !containsJSON(enum, value) {
		return errors.New("value is not in the schema enum")
	}
	if numeric, ok := number(value); ok {
		if minimum, ok := number(schema["minimum"]); ok && numeric < minimum {
			return errors.New("value is below the schema minimum")
		}
		if maximum, ok := number(schema["maximum"]); ok && numeric > maximum {
			return errors.New("value exceeds the schema maximum")
		}
	}
	if text, ok := value.(string); ok {
		if minimum, ok := number(schema["minLength"]); ok && len([]rune(text)) < int(minimum) {
			return errors.New("string is shorter than schema minLength")
		}
		if maximum, ok := number(schema["maxLength"]); ok && len([]rune(text)) > int(maximum) {
			return errors.New("string is longer than schema maxLength")
		}
	}
	if object, ok := value.(map[string]any); ok {
		if properties, ok := schema["properties"].(map[string]any); ok {
			for name, rawChildSchema := range properties {
				child, exists := object[name]
				if !exists {
					continue
				}
				childSchema, ok := rawChildSchema.(map[string]any)
				if !ok {
					return fmt.Errorf("schema property %q must be an object", name)
				}
				if err := validateJSONType(child, childSchema); err != nil {
					return fmt.Errorf("property %q: %w", name, err)
				}
			}
		}
	}
	reflected := reflect.ValueOf(value)
	if reflected.IsValid() && (reflected.Kind() == reflect.Array || reflected.Kind() == reflect.Slice) {
		if rawItems, exists := schema["items"]; exists {
			itemSchema, ok := rawItems.(map[string]any)
			if !ok {
				return errors.New("schema items must be an object")
			}
			for index := 0; index < reflected.Len(); index++ {
				if err := validateJSONType(reflected.Index(index).Interface(), itemSchema); err != nil {
					return fmt.Errorf("item %d: %w", index, err)
				}
			}
		}
	}
	return nil
}

func jsonTypeMatches(value any, typeName string) bool {
	switch typeName {
	case "null":
		return value == nil
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := number(value)
		return ok
	case "integer":
		n, ok := number(value)
		return ok && n == math.Trunc(n)
	case "array":
		reflected := reflect.ValueOf(value)
		return reflected.IsValid() && (reflected.Kind() == reflect.Array || reflected.Kind() == reflect.Slice)
	case "object":
		reflected := reflect.ValueOf(value)
		return reflected.IsValid() && reflected.Kind() == reflect.Map
	default:
		return false
	}
}
