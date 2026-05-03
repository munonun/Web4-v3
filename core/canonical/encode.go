package canonical

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"sort"
)

const (
	tagFields = 'f'
	tagString = 's'
	tagBytes  = 'b'
	tagInt    = 'i'
	tagUint   = 'u'
	tagList   = 'l'
	tagMap    = 'm'
)

// Field is an explicitly ordered field in a canonical Web4 preimage.
type Field struct {
	Name  string
	Value any
}

// EncodeFields encodes fields into deterministic bytes suitable for IDs and signatures.
// Field order is caller-defined. Map keys are sorted lexicographically.
func EncodeFields(fields ...Field) ([]byte, error) {
	var buf bytes.Buffer

	buf.WriteByte(tagFields)
	writeUvarint(&buf, uint64(len(fields)))

	for _, field := range fields {
		encodeString(&buf, field.Name)
		if err := encodeValue(&buf, field.Value); err != nil {
			return nil, fmt.Errorf("field %q: %w", field.Name, err)
		}
	}

	return buf.Bytes(), nil
}

func encodeValue(buf *bytes.Buffer, v any) error {
	if v == nil {
		return fmt.Errorf("unsupported type <nil>")
	}

	switch x := v.(type) {
	case string:
		encodeString(buf, x)
		return nil
	case []byte:
		encodeBytes(buf, x)
		return nil
	case int:
		encodeInt(buf, int64(x))
		return nil
	case int8:
		encodeInt(buf, int64(x))
		return nil
	case int16:
		encodeInt(buf, int64(x))
		return nil
	case int32:
		encodeInt(buf, int64(x))
		return nil
	case int64:
		encodeInt(buf, x)
		return nil
	case uint:
		encodeUint(buf, uint64(x))
		return nil
	case uint8:
		encodeUint(buf, uint64(x))
		return nil
	case uint16:
		encodeUint(buf, uint64(x))
		return nil
	case uint32:
		encodeUint(buf, uint64(x))
		return nil
	case uint64:
		encodeUint(buf, x)
		return nil
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Array:
		return encodeList(buf, rv)
	case reflect.Slice:
		return encodeList(buf, rv)
	case reflect.Map:
		return encodeMap(buf, rv)
	default:
		return fmt.Errorf("unsupported type %T", v)
	}
}

func encodeString(buf *bytes.Buffer, s string) {
	buf.WriteByte(tagString)
	writeUvarint(buf, uint64(len(s)))
	buf.WriteString(s)
}

func encodeBytes(buf *bytes.Buffer, b []byte) {
	buf.WriteByte(tagBytes)
	writeUvarint(buf, uint64(len(b)))
	buf.Write(b)
}

func encodeInt(buf *bytes.Buffer, n int64) {
	buf.WriteByte(tagInt)
	writeUvarint(buf, uint64(n<<1)^uint64(n>>63))
}

func encodeUint(buf *bytes.Buffer, n uint64) {
	buf.WriteByte(tagUint)
	writeUvarint(buf, n)
}

func encodeList(buf *bytes.Buffer, rv reflect.Value) error {
	buf.WriteByte(tagList)
	writeUvarint(buf, uint64(rv.Len()))

	for i := 0; i < rv.Len(); i++ {
		if err := encodeValue(buf, rv.Index(i).Interface()); err != nil {
			return fmt.Errorf("index %d: %w", i, err)
		}
	}

	return nil
}

func encodeMap(buf *bytes.Buffer, rv reflect.Value) error {
	if rv.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("unsupported map key type %s", rv.Type().Key())
	}

	keys := make([]string, 0, rv.Len())
	for _, key := range rv.MapKeys() {
		keys = append(keys, key.String())
	}
	sort.Strings(keys)

	buf.WriteByte(tagMap)
	writeUvarint(buf, uint64(len(keys)))

	for _, key := range keys {
		encodeString(buf, key)
		mapKey := reflect.ValueOf(key).Convert(rv.Type().Key())
		if err := encodeValue(buf, rv.MapIndex(mapKey).Interface()); err != nil {
			return fmt.Errorf("map key %q: %w", key, err)
		}
	}

	return nil
}

func writeUvarint(buf *bytes.Buffer, n uint64) {
	var tmp [binary.MaxVarintLen64]byte
	buf.Write(tmp[:binary.PutUvarint(tmp[:], n)])
}
