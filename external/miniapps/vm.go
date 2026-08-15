//go:build miniapps

package miniapps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	maxVMInstructions = 10_000_000
	maxVMStackDepth   = 4096
	maxVMCallDepth    = 256
	maxVMHeapBytes    = 256 << 20
)

var allowedOpcodes = map[string]bool{
	"const": true, "ref.get": true, "local.get": true, "local.set": true,
	"dup": true, "swap": true, "pop": true,
	"add": true, "sub": true, "mul": true, "div": true, "mod": true, "neg": true,
	"eq": true, "ne": true, "lt": true, "lte": true, "gt": true, "gte": true,
	"not": true, "and": true, "or": true,
	"string.concat": true, "string.len": true, "string.contains": true,
	"to.string": true, "to.number": true, "to.boolean": true,
	"array.new": true, "array.len": true, "array.get": true, "array.set": true, "array.push": true,
	"object.new": true, "object.len": true, "object.get": true, "object.set": true, "object.has": true,
	"label": true, "jump": true, "jump_if_true": true, "jump_if_false": true,
	"call": true, "return": true, "try": true, "end_try": true, "throw": true,
	"json.validate": true, "host.call": true,
}

// nameArg returns the string argument an opcode names something with.
func nameArg(function string, index int, instruction Instruction) (string, error) {
	name, ok := instruction.Arg.(string)
	if !ok || name == "" {
		return "", fmt.Errorf("function %s instruction %d: %s requires a name argument",
			function, index, instruction.Op)
	}
	return name, nil
}

type verifiedProgram struct {
	program Program
	labels  map[string]map[string]int
}

func verifyProgram(program Program) (*verifiedProgram, error) {
	if program.Language != VMVersion {
		return nil, fmt.Errorf("program.language must be %s", VMVersion)
	}
	if program.Entry == "" {
		return nil, errors.New("program.entry is required")
	}
	if _, ok := program.Functions[program.Entry]; !ok {
		return nil, fmt.Errorf("program entry function %q does not exist", program.Entry)
	}
	if program.Limits.Instructions <= 0 || program.Limits.Instructions > maxVMInstructions {
		return nil, fmt.Errorf("program instruction limit must be between 1 and %d", maxVMInstructions)
	}
	if program.Limits.StackDepth <= 0 || program.Limits.StackDepth > maxVMStackDepth {
		return nil, fmt.Errorf("program stack limit must be between 1 and %d", maxVMStackDepth)
	}
	if program.Limits.CallDepth <= 0 || program.Limits.CallDepth > maxVMCallDepth {
		return nil, fmt.Errorf("program call limit must be between 1 and %d", maxVMCallDepth)
	}
	if program.Limits.HeapBytes < 0 || program.Limits.HeapBytes > maxVMHeapBytes {
		return nil, fmt.Errorf("program heap limit exceeds engine policy")
	}

	for id := range program.Imports {
		if id == "" {
			return nil, errors.New("program contains an empty import name")
		}
	}
	labels := make(map[string]map[string]int, len(program.Functions))
	for name, code := range program.Functions {
		if name == "" {
			return nil, errors.New("program contains an empty function name")
		}
		labels[name] = map[string]int{}
		for i, instruction := range code {
			if !allowedOpcodes[instruction.Op] {
				return nil, fmt.Errorf("function %s instruction %d: unknown opcode %q", name, i, instruction.Op)
			}
			if instruction.Op == "label" {
				label, err := nameArg(name, i, instruction)
				if err != nil {
					return nil, err
				}
				if _, duplicate := labels[name][label]; duplicate {
					return nil, fmt.Errorf("function %s: duplicate label %q", name, label)
				}
				labels[name][label] = i
			}
		}
	}
	for name, code := range program.Functions {
		for i, instruction := range code {
			switch instruction.Op {
			case "jump", "jump_if_true", "jump_if_false", "try", "call", "host.call":
			default:
				continue
			}
			// The interpreter reads these arguments as strings without
			// re-checking, so reject anything else before the program runs.
			arg, err := nameArg(name, i, instruction)
			if err != nil {
				return nil, err
			}
			switch instruction.Op {
			case "jump", "jump_if_true", "jump_if_false", "try":
				if _, ok := labels[name][arg]; !ok {
					return nil, fmt.Errorf("function %s instruction %d: unknown label %q", name, i, arg)
				}
			case "call":
				if _, ok := program.Functions[arg]; !ok {
					return nil, fmt.Errorf("function %s instruction %d: unknown function %q", name, i, arg)
				}
			case "host.call":
				if _, ok := program.Imports[arg]; !ok {
					return nil, fmt.Errorf("function %s instruction %d: undeclared import %q", name, i, arg)
				}
			}
		}
	}
	return &verifiedProgram{program: program, labels: labels}, nil
}

type VMHost interface {
	Call(context.Context, string, any) (any, error)
}

type frame struct {
	function string
	code     []Instruction
	pc       int
	locals   map[string]any
	handlers []int
}

// ExecuteProgram validates and runs a pure JSON program.
func ExecuteProgram(ctx context.Context, program Program, refs map[string]any) (any, error) {
	return ExecuteProgramWithHost(ctx, program, refs, nil)
}

func ExecuteProgramWithHost(ctx context.Context, program Program, refs map[string]any, host VMHost) (any, error) {
	verified, err := verifyProgram(program)
	if err != nil {
		return nil, err
	}
	if program.Limits.WallTimeSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(program.Limits.WallTimeSeconds)*time.Second)
		defer cancel()
	}
	frames := []frame{{
		function: program.Entry,
		code:     program.Functions[program.Entry],
		locals:   map[string]any{},
	}}
	stack := make([]any, 0, 16)
	fuel := program.Limits.Instructions

	push := func(value any) error {
		if len(stack) >= program.Limits.StackDepth {
			return errors.New("program stack limit exceeded")
		}
		stack = append(stack, value)
		return nil
	}
	pop := func() (any, error) {
		if len(stack) == 0 {
			return nil, errors.New("program stack underflow")
		}
		value := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return value, nil
	}

	for len(frames) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("program cancelled: %w", err)
		}
		if fuel == 0 {
			return nil, errors.New("program instruction limit exceeded")
		}
		fuel--
		current := &frames[len(frames)-1]
		if current.pc >= len(current.code) {
			stack = append(stack, nil)
			frames = frames[:len(frames)-1]
			if len(frames) == 0 {
				return stack[len(stack)-1], nil
			}
			continue
		}
		instruction := current.code[current.pc]
		current.pc++
		fail := func(opErr error) (bool, error) {
			if len(current.handlers) == 0 {
				return false, fmt.Errorf("%s[%d] %s: %w", current.function, current.pc-1, instruction.Op, opErr)
			}
			handler := current.handlers[len(current.handlers)-1]
			current.handlers = current.handlers[:len(current.handlers)-1]
			_ = push(opErr.Error())
			current.pc = handler + 1
			return true, nil
		}

		switch instruction.Op {
		case "label":
			continue
		case "const":
			err = push(deepCopyValue(instruction.Arg))
		case "ref.get":
			path, _ := instruction.Arg.(string)
			var value any
			value, err = resolvePath(refs, path)
			if err == nil {
				err = push(deepCopyValue(value))
			}
		case "local.get":
			name, _ := instruction.Arg.(string)
			value, ok := current.locals[name]
			if !ok {
				err = fmt.Errorf("unknown local %q", name)
			} else {
				err = push(deepCopyValue(value))
			}
		case "local.set":
			name, _ := instruction.Arg.(string)
			var value any
			value, err = pop()
			if err == nil {
				current.locals[name] = deepCopyValue(value)
			}
		case "dup":
			if len(stack) == 0 {
				err = errors.New("stack underflow")
			} else {
				err = push(deepCopyValue(stack[len(stack)-1]))
			}
		case "swap":
			if len(stack) < 2 {
				err = errors.New("stack underflow")
			} else {
				stack[len(stack)-1], stack[len(stack)-2] = stack[len(stack)-2], stack[len(stack)-1]
			}
		case "pop":
			_, err = pop()
		case "add", "sub", "mul", "div", "mod", "eq", "ne", "lt", "lte", "gt", "gte",
			"and", "or", "string.concat", "string.contains":
			var right, left any
			right, err = pop()
			if err == nil {
				left, err = pop()
			}
			if err == nil {
				var value any
				value, err = binaryOp(instruction.Op, left, right)
				if err == nil {
					err = push(value)
				}
			}
		case "neg":
			var value any
			value, err = pop()
			if err == nil {
				if n, ok := number(value); ok {
					err = push(-n)
				} else {
					err = errors.New("neg expects a number")
				}
			}
		case "not":
			var value any
			value, err = pop()
			if err == nil {
				err = push(!truthy(value))
			}
		case "string.len":
			var value any
			value, err = pop()
			if err == nil {
				str, ok := value.(string)
				if !ok {
					err = errors.New("string.len expects a string")
				} else {
					err = push(float64(len([]rune(str))))
				}
			}
		case "to.string", "to.number", "to.boolean":
			var value any
			value, err = pop()
			if err == nil {
				switch instruction.Op {
				case "to.string":
					err = push(fmt.Sprint(value))
				case "to.boolean":
					err = push(truthy(value))
				case "to.number":
					if n, ok := number(value); ok {
						err = push(n)
					} else if str, ok := value.(string); ok {
						var n float64
						n, err = strconv.ParseFloat(str, 64)
						if err == nil {
							err = push(n)
						}
					} else {
						err = errors.New("cannot convert value to number")
					}
				}
			}
		case "array.new":
			err = push([]any{})
		case "array.len":
			var value any
			value, err = pop()
			if err == nil {
				items, ok := value.([]any)
				if !ok {
					err = errors.New("array.len expects an array")
				} else {
					err = push(float64(len(items)))
				}
			}
		case "array.get":
			var indexValue, arrayValue any
			indexValue, err = pop()
			if err == nil {
				arrayValue, err = pop()
			}
			if err == nil {
				items, ok := arrayValue.([]any)
				index, indexOK := integer(indexValue)
				if !ok || !indexOK || index < 0 || index >= len(items) {
					err = errors.New("invalid array index")
				} else {
					err = push(deepCopyValue(items[index]))
				}
			}
		case "array.set", "array.push":
			var value, indexValue, arrayValue any
			value, err = pop()
			if instruction.Op == "array.set" && err == nil {
				indexValue, err = pop()
			}
			if err == nil {
				arrayValue, err = pop()
			}
			if err == nil {
				items, ok := arrayValue.([]any)
				if !ok {
					err = errors.New("array operation expects an array")
				} else if instruction.Op == "array.push" {
					items = append(items, deepCopyValue(value))
					err = push(items)
				} else if index, ok := integer(indexValue); !ok || index < 0 || index >= len(items) {
					err = errors.New("invalid array index")
				} else {
					items[index] = deepCopyValue(value)
					err = push(items)
				}
			}
		case "object.new":
			err = push(map[string]any{})
		case "object.len":
			var value any
			value, err = pop()
			if err == nil {
				object, ok := value.(map[string]any)
				if !ok {
					err = errors.New("object.len expects an object")
				} else {
					err = push(float64(len(object)))
				}
			}
		case "object.get", "object.has":
			var keyValue, objectValue any
			keyValue, err = pop()
			if err == nil {
				objectValue, err = pop()
			}
			if err == nil {
				object, ok := objectValue.(map[string]any)
				key, keyOK := keyValue.(string)
				if !ok || !keyOK {
					err = errors.New("object operation expects an object and string key")
				} else if instruction.Op == "object.has" {
					_, exists := object[key]
					err = push(exists)
				} else if value, exists := object[key]; exists {
					err = push(deepCopyValue(value))
				} else {
					err = fmt.Errorf("object key %q does not exist", key)
				}
			}
		case "object.set":
			var value, keyValue, objectValue any
			value, err = pop()
			if err == nil {
				keyValue, err = pop()
			}
			if err == nil {
				objectValue, err = pop()
			}
			if err == nil {
				object, ok := objectValue.(map[string]any)
				key, keyOK := keyValue.(string)
				if !ok || !keyOK {
					err = errors.New("object.set expects an object and string key")
				} else {
					object[key] = deepCopyValue(value)
					err = push(object)
				}
			}
		case "jump", "jump_if_true", "jump_if_false":
			jump := instruction.Op == "jump"
			if instruction.Op != "jump" {
				var condition any
				condition, err = pop()
				if err == nil {
					jump = truthy(condition) == (instruction.Op == "jump_if_true")
				}
			}
			if err == nil && jump {
				current.pc = verified.labels[current.function][instruction.Arg.(string)] + 1
			}
		case "call":
			if len(frames) >= program.Limits.CallDepth {
				err = errors.New("program call-depth limit exceeded")
			} else {
				name := instruction.Arg.(string)
				frames = append(frames, frame{function: name, code: program.Functions[name], locals: map[string]any{}})
			}
		case "return":
			var value any
			value, err = pop()
			if err == nil {
				frames = frames[:len(frames)-1]
				if len(frames) == 0 {
					return value, nil
				}
				err = push(value)
			}
		case "try":
			current.handlers = append(current.handlers, verified.labels[current.function][instruction.Arg.(string)])
		case "end_try":
			if len(current.handlers) > 0 {
				current.handlers = current.handlers[:len(current.handlers)-1]
			}
		case "throw":
			var value any
			value, err = pop()
			if err == nil {
				err = fmt.Errorf("%v", value)
			}
		case "json.validate":
			var value any
			value, err = pop()
			if err == nil {
				err = validateJSONType(value, instruction.Arg)
				if err == nil {
					err = push(value)
				}
			}
		case "host.call":
			if host == nil {
				err = errors.New("host capabilities are unavailable")
			} else {
				var input any
				input, err = pop()
				if err == nil {
					var output any
					output, err = host.Call(ctx, instruction.Arg.(string), input)
					if err == nil {
						err = push(output)
					}
				}
			}
		}
		if err != nil {
			handled, handlerErr := fail(err)
			if handlerErr != nil {
				return nil, handlerErr
			}
			if handled {
				err = nil
			}
		}
		if program.Limits.HeapBytes > 0 {
			raw, _ := json.Marshal([]any{stack, frames[len(frames)-1].locals})
			if int64(len(raw)) > program.Limits.HeapBytes {
				return nil, errors.New("program heap limit exceeded")
			}
		}
	}
	return nil, errors.New("program ended without a result")
}

func binaryOp(op string, left, right any) (any, error) {
	switch op {
	case "string.concat":
		return fmt.Sprint(left) + fmt.Sprint(right), nil
	case "string.contains":
		return strings.Contains(fmt.Sprint(left), fmt.Sprint(right)), nil
	case "eq":
		return equalJSON(left, right), nil
	case "ne":
		return !equalJSON(left, right), nil
	case "and":
		return truthy(left) && truthy(right), nil
	case "or":
		return truthy(left) || truthy(right), nil
	}
	a, okA := number(left)
	b, okB := number(right)
	if !okA || !okB {
		return nil, fmt.Errorf("%s expects numeric operands", op)
	}
	switch op {
	case "add":
		return a + b, nil
	case "sub":
		return a - b, nil
	case "mul":
		return a * b, nil
	case "div":
		if b == 0 {
			return nil, errors.New("division by zero")
		}
		return a / b, nil
	case "mod":
		if b == 0 {
			return nil, errors.New("division by zero")
		}
		return math.Mod(a, b), nil
	case "lt":
		return a < b, nil
	case "lte":
		return a <= b, nil
	case "gt":
		return a > b, nil
	case "gte":
		return a >= b, nil
	default:
		return nil, fmt.Errorf("unsupported binary operation %q", op)
	}
}

func number(value any) (float64, bool) {
	switch n := value.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case json.Number:
		v, err := n.Float64()
		return v, err == nil
	default:
		return 0, false
	}
}

func integer(value any) (int, bool) {
	n, ok := number(value)
	if !ok || n != math.Trunc(n) || n > math.MaxInt || n < math.MinInt {
		return 0, false
	}
	return int(n), true
}

func truthy(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return v != ""
	case float64:
		return v != 0
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	default:
		return true
	}
}

func equalJSON(left, right any) bool {
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	return string(a) == string(b)
}

func deepCopyValue(value any) any {
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var out any
	if json.Unmarshal(raw, &out) != nil {
		return value
	}
	return out
}

func resolvePath(root map[string]any, path string) (any, error) {
	var current any = root
	for _, part := range strings.Split(strings.TrimSpace(path), ".") {
		if part == "" {
			return nil, fmt.Errorf("invalid empty reference segment in %q", path)
		}
		switch value := current.(type) {
		case map[string]any:
			next, ok := value[part]
			if !ok {
				return nil, fmt.Errorf("reference %q does not exist", path)
			}
			current = next
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(value) {
				return nil, fmt.Errorf("reference %q has invalid array index", path)
			}
			current = value[index]
		default:
			return nil, fmt.Errorf("reference %q traverses a scalar value", path)
		}
	}
	return current, nil
}

func validateJSONType(value, schemaValue any) error {
	schema, ok := schemaValue.(map[string]any)
	if !ok {
		return errors.New("json.validate requires an inline schema object")
	}
	expected, _ := schema["type"].(string)
	ok = expected == "" ||
		(expected == "object" && isObject(value)) ||
		(expected == "array" && isArray(value)) ||
		(expected == "string" && isString(value)) ||
		(expected == "number" && isNumber(value)) ||
		(expected == "integer" && isInteger(value)) ||
		(expected == "boolean" && isBoolean(value)) ||
		(expected == "null" && value == nil)
	if !ok {
		return fmt.Errorf("value does not satisfy schema type %q", expected)
	}
	return nil
}

func isObject(v any) bool  { _, ok := v.(map[string]any); return ok }
func isArray(v any) bool   { _, ok := v.([]any); return ok }
func isString(v any) bool  { _, ok := v.(string); return ok }
func isNumber(v any) bool  { _, ok := number(v); return ok }
func isBoolean(v any) bool { _, ok := v.(bool); return ok }
func isInteger(v any) bool { _, ok := integer(v); return ok }
