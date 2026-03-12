package gotk

import "testing"

func TestPayload_String(t *testing.T) {
	p := NewPayload(map[string]any{"name": "Alice", "count": 42})
	if got := p.String("name"); got != "Alice" {
		t.Errorf("String(name) = %q, want Alice", got)
	}
	if got := p.String("count"); got != "42" {
		t.Errorf("String(count) = %q, want 42", got)
	}
	if got := p.String("missing"); got != "" {
		t.Errorf("String(missing) = %q, want empty", got)
	}
}

func TestPayload_Int(t *testing.T) {
	p := NewPayload(map[string]any{"a": 42, "b": "7", "c": 3.9, "d": "bad"})
	if got := p.Int("a"); got != 42 {
		t.Errorf("Int(a) = %d, want 42", got)
	}
	if got := p.Int("b"); got != 7 {
		t.Errorf("Int(b) = %d, want 7", got)
	}
	if got := p.Int("c"); got != 3 {
		t.Errorf("Int(c) = %d, want 3", got)
	}
	if got := p.Int("d"); got != 0 {
		t.Errorf("Int(d) = %d, want 0", got)
	}
	if got := p.Int("missing"); got != 0 {
		t.Errorf("Int(missing) = %d, want 0", got)
	}
}

func TestPayload_Float(t *testing.T) {
	p := NewPayload(map[string]any{"a": 3.14, "b": "2.5", "c": 7})
	if got := p.Float("a"); got != 3.14 {
		t.Errorf("Float(a) = %f, want 3.14", got)
	}
	if got := p.Float("b"); got != 2.5 {
		t.Errorf("Float(b) = %f, want 2.5", got)
	}
	if got := p.Float("c"); got != 7.0 {
		t.Errorf("Float(c) = %f, want 7.0", got)
	}
}

func TestPayload_Bool(t *testing.T) {
	p := NewPayload(map[string]any{"a": true, "b": "true", "c": "1", "d": "false", "e": false})
	if !p.Bool("a") {
		t.Error("Bool(a) should be true")
	}
	if !p.Bool("b") {
		t.Error("Bool(b) should be true")
	}
	if !p.Bool("c") {
		t.Error("Bool(c) should be true")
	}
	if p.Bool("d") {
		t.Error("Bool(d) should be false")
	}
	if p.Bool("e") {
		t.Error("Bool(e) should be false")
	}
	if p.Bool("missing") {
		t.Error("Bool(missing) should be false")
	}
}

func TestPayload_NilMap(t *testing.T) {
	p := NewPayload(nil)
	if got := p.String("x"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	if p.Map() == nil {
		t.Error("Map() should not be nil after NewPayload(nil)")
	}
}
