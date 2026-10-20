//go:build miniapps

package miniapps

import (
	"context"
	"reflect"
	"testing"
)

func TestResolveValuePreservesTypedReferencesAndInterpolates(t *testing.T) {
	refs := map[string]any{
		"inputs": map[string]any{"name": "Foxxy", "count": float64(2)},
		"steps":  map[string]any{"read": map[string]any{"outputs": map[string]any{"result": "ok"}}},
	}
	value, err := resolveValue(Ref{Ref: "inputs.count"}, refs)
	if err != nil || value != float64(2) {
		t.Fatalf("typed reference = %#v, %v", value, err)
	}
	value, err = resolveValue(map[string]any{"greeting": "Hi {{ inputs.name }}", "result": Ref{Ref: "steps.read.outputs.result"}}, refs)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"greeting": "Hi Foxxy", "result": "ok"}
	if !reflect.DeepEqual(value, want) {
		t.Fatalf("resolved map = %#v, want %#v", value, want)
	}
}

func TestEvaluateConditionOperatorsAndErrors(t *testing.T) {
	refs := map[string]any{"inputs": map[string]any{"name": "Foxxy", "count": float64(2)}}
	cases := []struct {
		name string
		cond Condition
		want bool
	}{
		{"eq", Condition{Op: "eq", Left: Ref{Ref: "inputs.name"}, Right: "Foxxy"}, true},
		{"gt", Condition{Op: "gt", Left: Ref{Ref: "inputs.count"}, Right: 1}, true},
		{"contains", Condition{Op: "contains", Left: "Foxxy", Right: "ox"}, true},
		{"empty", Condition{Op: "empty", Value: ""}, true},
		{"not", Condition{Op: "not", Args: []Condition{{Op: "eq", Left: 1, Right: 2}}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := evaluateCondition(tc.cond, refs)
			if err != nil || got != tc.want {
				t.Fatalf("got %v, %v; want %v", got, err, tc.want)
			}
		})
	}
	if _, err := evaluateCondition(Condition{Op: "not"}, refs); err == nil {
		t.Fatal("invalid not condition accepted")
	}
	if _, err := resolveValue(Ref{Ref: "inputs.missing"}, refs); err == nil {
		t.Fatal("missing reference resolved")
	}
	if _, err := resolveValue("{{ inputs.missing }}", refs); err == nil {
		t.Fatal("missing interpolation resolved")
	}
	if _, err := ExecuteCondition(context.Background(), Condition{Op: "eq", Left: 1, Right: 1}, refs); err != nil {
		t.Fatal(err)
	}
}

func TestValidateJSONTypeChecksNestedSchema(t *testing.T) {
	schema := map[string]any{
		"type": "object", "required": []any{"items"},
		"properties": map[string]any{"items": map[string]any{
			"type": "array", "items": map[string]any{
				"type": "object", "required": []any{"score"},
				"properties": map[string]any{"score": map[string]any{"type": "number", "minimum": 1.0}},
			},
		}},
	}
	if err := validateJSONType(map[string]any{"items": []any{map[string]any{"score": 2.0}}}, schema); err != nil {
		t.Fatal(err)
	}
	if err := validateJSONType(map[string]any{"items": []any{map[string]any{"score": 0.0}}}, schema); err == nil {
		t.Fatal("nested schema minimum was not enforced")
	}
}
