package json5

import "fmt"

// OrderedMap is a JSON5 object that preserves the insertion order of keys.
// It supports O(1) key lookup and O(1) amortized append.
type OrderedMap struct {
	entries []Entry
	index   map[string]int // key -> position in entries
}

// Entry is a key-value pair in an OrderedMap.
type Entry struct {
	Key   string
	Value any
}

// NewOrderedMap returns a new empty OrderedMap.
func NewOrderedMap() *OrderedMap {
	return &OrderedMap{index: make(map[string]int)}
}

// Len returns the number of entries.
func (m *OrderedMap) Len() int {
	return len(m.entries)
}

// Get returns the value for key and whether it was found.
func (m *OrderedMap) Get(key string) (any, bool) {
	if m.index == nil {
		return nil, false
	}
	i, ok := m.index[key]
	if !ok {
		return nil, false
	}
	return m.entries[i].Value, true
}

// Set adds or updates a key-value pair. If the key already exists, its value
// is updated in place (preserving order). If the key is new, it is appended.
func (m *OrderedMap) Set(key string, value any) {
	if m.index == nil {
		m.index = make(map[string]int)
	}
	if i, ok := m.index[key]; ok {
		m.entries[i].Value = value
		return
	}
	m.index[key] = len(m.entries)
	m.entries = append(m.entries, Entry{Key: key, Value: value})
}

// Delete removes a key. It preserves the order of remaining keys.
func (m *OrderedMap) Delete(key string) {
	i, ok := m.index[key]
	if !ok {
		return
	}
	delete(m.index, key)
	m.entries = append(m.entries[:i], m.entries[i+1:]...)
	// Rebuild index for shifted entries.
	for j := i; j < len(m.entries); j++ {
		m.index[m.entries[j].Key] = j
	}
}

// Keys returns the keys in insertion order.
func (m *OrderedMap) Keys() []string {
	keys := make([]string, len(m.entries))
	for i, e := range m.entries {
		keys[i] = e.Key
	}
	return keys
}

// Entries returns all entries in insertion order.
func (m *OrderedMap) Entries() []Entry {
	return m.entries
}

// MarshalJSON5 implements the Marshaler interface, emitting keys in insertion order.
func (m *OrderedMap) MarshalJSON5() ([]byte, error) {
	e := newEncodeState()
	e.WriteByte('{')
	for i, entry := range m.entries {
		if i > 0 {
			e.WriteByte(',')
		}
		e.encodeString(entry.Key)
		e.WriteByte(':')
		if err := e.marshal(entry.Value); err != nil {
			encodeStatePool.Put(e)
			return nil, err
		}
	}
	e.WriteByte('}')
	buf := append([]byte(nil), e.Bytes()...)
	encodeStatePool.Put(e)
	return buf, nil
}

// UnmarshalJSON5 implements the Unmarshaler interface.
func (m *OrderedMap) UnmarshalJSON5(data []byte) error {
	d := newDecodeState()
	defer decodeStatePool.Put(d)
	d.init(data)
	d.useOrderedMap = true

	tok, err := d.scanner.scan()
	if err != nil {
		return err
	}
	if tok.typ != tokenObjectOpen {
		return &SyntaxError{msg: fmt.Sprintf("expected object, got %q", tok.raw), Offset: int64(tok.pos)}
	}

	result, err := d.orderedMapInterface()
	if err != nil {
		return err
	}
	*m = *result
	return nil
}
