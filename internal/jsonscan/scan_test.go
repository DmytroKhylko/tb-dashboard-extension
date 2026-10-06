package jsonscan

import (
	"testing"
)

func TestStringValuesSkipsKeysAndHandlesEscapes(t *testing.T) {
	doc := []byte(`{"url": "a\"b", "list": ["x\\", {"url": "y"}], "n": 1}`)
	values, err := StringValues(doc)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{`a"b`, `x\`, "y"}
	if len(values) != len(want) {
		t.Fatalf("got %d values, want %d: %+v", len(values), len(want), values)
	}
	for i, v := range values {
		if v.Value != want[i] {
			t.Errorf("value %d = %q, want %q", i, v.Value, want[i])
		}
		if got := string(doc[v.Span.Start]) + string(doc[v.Span.End-1]); got != `""` {
			t.Errorf("span %d does not cover the literal: %q", i, doc[v.Span.Start:v.Span.End])
		}
	}
}

func TestObjectMembers(t *testing.T) {
	doc := []byte(` { "a": [1, {"b": "]"}], "c": "x", "d": null } `)
	members, err := ObjectMembers(doc, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"a": `[1, {"b": "]"}]`, "c": `"x"`, "d": "null"}
	if len(members) != len(want) {
		t.Fatalf("got %d members, want %d", len(members), len(want))
	}
	for _, m := range members {
		if got := string(doc[m.Value.Start:m.Value.End]); got != want[m.Key] {
			t.Errorf("member %q = %q, want %q", m.Key, got, want[m.Key])
		}
	}
}

func TestArrayElements(t *testing.T) {
	doc := []byte(`[ {"x": "[,]"}, 2 ,"s" ]`)
	elements, err := ArrayElements(doc, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{`{"x": "[,]"}`, "2", `"s"`}
	if len(elements) != len(want) {
		t.Fatalf("got %d elements, want %d", len(elements), len(want))
	}
	for i, e := range elements {
		if got := string(doc[e.Start:e.End]); got != want[i] {
			t.Errorf("element %d = %q, want %q", i, got, want[i])
		}
	}

	empty, err := ArrayElements([]byte(" [ ] "), 0)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty array: %v, %v", empty, err)
	}
}

func TestStringValuesRejectsInvalidJSON(t *testing.T) {
	if _, err := StringValues([]byte(`{"a": `)); err == nil {
		t.Fatal("expected an error")
	}
}

func TestLineIndent(t *testing.T) {
	doc := []byte("{\n    \"a\": 1\n}")
	if got := string(LineIndent(doc, 6)); got != "    " {
		t.Fatalf("got %q", got)
	}
}
