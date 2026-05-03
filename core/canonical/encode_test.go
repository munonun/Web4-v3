package canonical

import (
	"bytes"
	"testing"

	"web4-v3/core/crypto"
)

func TestEncodeFieldsStable(t *testing.T) {
	a, err := EncodeFields(Field{Name: "name", Value: "web4"}, Field{Name: "n", Value: int64(1)})
	if err != nil {
		t.Fatalf("encode fields: %v", err)
	}
	b, err := EncodeFields(Field{Name: "name", Value: "web4"}, Field{Name: "n", Value: int64(1)})
	if err != nil {
		t.Fatalf("encode fields: %v", err)
	}

	if !bytes.Equal(a, b) {
		t.Fatal("same fields produced different bytes")
	}
}

func TestEncodeFieldsOrderMatters(t *testing.T) {
	a, err := EncodeFields(Field{Name: "a", Value: "1"}, Field{Name: "b", Value: "2"})
	if err != nil {
		t.Fatalf("encode fields: %v", err)
	}
	b, err := EncodeFields(Field{Name: "b", Value: "2"}, Field{Name: "a", Value: "1"})
	if err != nil {
		t.Fatalf("encode fields: %v", err)
	}

	if bytes.Equal(a, b) {
		t.Fatal("different field order produced same bytes")
	}
}

func TestEncodeMapsDeterministically(t *testing.T) {
	a, err := EncodeFields(Field{Name: "m", Value: map[string]any{"b": "2", "a": "1"}})
	if err != nil {
		t.Fatalf("encode map a: %v", err)
	}
	b, err := EncodeFields(Field{Name: "m", Value: map[string]any{"a": "1", "b": "2"}})
	if err != nil {
		t.Fatalf("encode map b: %v", err)
	}

	if !bytes.Equal(a, b) {
		t.Fatal("same map content produced different bytes")
	}
}

func TestEncodeUnsupportedTypesReturnErrors(t *testing.T) {
	if _, err := EncodeFields(Field{Name: "bad", Value: true}); err == nil {
		t.Fatal("expected unsupported bool error")
	}
	if _, err := EncodeFields(Field{Name: "bad", Value: map[int]string{1: "x"}}); err == nil {
		t.Fatal("expected unsupported map key error")
	}
}

func TestEncodeCanBeHashedForStableIDs(t *testing.T) {
	encoded, err := EncodeFields(
		Field{Name: "kind", Value: "value"},
		Field{Name: "owner", Value: []byte{1, 2, 3}},
		Field{Name: "parts", Value: []string{"a", "b"}},
	)
	if err != nil {
		t.Fatalf("encode fields: %v", err)
	}

	id1 := crypto.HashBytes(encoded)
	id2 := crypto.HashBytes(encoded)
	if id1 != id2 {
		t.Fatal("hashing canonical bytes was not stable")
	}
}
