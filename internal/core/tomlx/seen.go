package tomlx

import (
	"bytes"
	"fmt"

	"github.com/pelletier/go-toml/v2/unstable"
)

// seenTracker is go-toml's internal/tracker.SeenTracker (the expression
// variant): it rejects keys defined twice and tables redefined, the checks
// the parser leaves to the decoder.

type keyKind uint8

const (
	invalidKind keyKind = iota
	valueKind
	kvTableKind // made by a dotted key
	tableKind
	arrayTableKind
	anonymousKind // an inline table or array inside an array
)

func (k keyKind) String() string {
	switch k {
	case valueKind:
		return "value"
	case kvTableKind:
		return "kv-table"
	case tableKind:
		return "table"
	case arrayTableKind:
		return "array-table"
	case anonymousKind:
		return "anonymous"
	}
	return "invalid"
}

type seenEntry struct {
	parent   int32
	kind     keyKind
	explicit bool
	name     []byte
}

type seenTracker struct {
	entries      []seenEntry // entries[0] is the root table
	currentTable int32
}

func (s *seenTracker) find(parent int32, name []byte) int32 {
	for i := int(parent) + 1; i < len(s.entries); i++ {
		e := &s.entries[i]
		if e.parent == parent && e.kind != anonymousKind && bytes.Equal(e.name, name) {
			return int32(i) //nolint:gosec // bounded by the document's size
		}
	}
	return -1
}

func (s *seenTracker) create(parent int32, name []byte, kind keyKind, explicit bool) int32 {
	s.entries = append(s.entries, seenEntry{parent: parent, kind: kind, explicit: explicit, name: name})
	return int32(len(s.entries) - 1) //nolint:gosec // bounded by the document's size
}

// clear forgets everything under id, so a new [[array]] element starts empty.
func (s *seenTracker) clear(id int32) {
	removed := make([]bool, len(s.entries))
	remap := make([]int32, len(s.entries))
	n := int32(0)
	for i := range s.entries {
		parent := s.entries[i].parent
		if parent >= 0 && (parent == id && s.entries[i].kind != invalidKind || removed[parent]) {
			removed[i] = true
			continue
		}
		remap[i] = n
		if int32(i) != n { //nolint:gosec // bounded by the document's size
			e := s.entries[i]
			e.parent = remap[e.parent]
			s.entries[n] = e
		}
		n++
	}
	s.entries = s.entries[:n]
}

// checkExpression validates one expression; for tables it reports whether
// the header is new.
func (s *seenTracker) checkExpression(node *unstable.Node) (bool, error) {
	if len(s.entries) == 0 {
		s.entries = append(s.entries, seenEntry{parent: -1, kind: tableKind})
	}
	switch node.Kind {
	case unstable.KeyValue:
		return false, s.checkKeyValue(s.currentTable, node)
	case unstable.Table:
		return s.checkTable(node)
	case unstable.ArrayTable:
		return s.checkArrayTable(node)
	}
	return false, fmt.Errorf("toml: unexpected expression kind %s", node.Kind)
}

// parentTable walks a header's leading parts, creating implicit tables.
func (s *seenTracker) parentTable(parent int32, name []byte) (int32, error) {
	i := s.find(parent, name)
	if i < 0 {
		return s.create(parent, name, tableKind, false), nil
	}
	switch s.entries[i].kind {
	case tableKind, arrayTableKind, kvTableKind:
		return i, nil
	}
	return 0, fmt.Errorf("toml: key %s already exists as a value", name)
}

func (s *seenTracker) checkTable(node *unstable.Node) (bool, error) {
	parent := int32(0)
	for it := node.Key(); it.Next(); {
		name := it.Node().Data
		if !it.IsLast() {
			var err error
			if parent, err = s.parentTable(parent, name); err != nil {
				return false, err
			}
			continue
		}
		i := s.find(parent, name)
		if i < 0 {
			s.currentTable = s.create(parent, name, tableKind, true)
			return true, nil
		}
		e := &s.entries[i]
		switch e.kind {
		case tableKind:
			if e.explicit {
				return false, fmt.Errorf("toml: table %s already exists", name)
			}
			e.explicit = true
			s.currentTable = i
			return false, nil
		case kvTableKind:
			return false, fmt.Errorf("toml: table %s already exists as defined by a dotted key", name)
		case arrayTableKind:
			return false, fmt.Errorf("toml: table %s already exists as an array of tables", name)
		}
		return false, fmt.Errorf("toml: key %s should be a table, not a %s", name, e.kind)
	}
	return false, fmt.Errorf("toml: table expression without key")
}

func (s *seenTracker) checkArrayTable(node *unstable.Node) (bool, error) {
	parent := int32(0)
	for it := node.Key(); it.Next(); {
		name := it.Node().Data
		if !it.IsLast() {
			var err error
			if parent, err = s.parentTable(parent, name); err != nil {
				return false, err
			}
			continue
		}
		i := s.find(parent, name)
		if i < 0 {
			s.currentTable = s.create(parent, name, arrayTableKind, true)
			return true, nil
		}
		if s.entries[i].kind != arrayTableKind {
			return false, fmt.Errorf("toml: key %s already exists as a %s, but should be an array table", name, s.entries[i].kind)
		}
		s.clear(i)
		s.currentTable = i
		return false, nil
	}
	return false, fmt.Errorf("toml: array table expression without key")
}

func (s *seenTracker) checkKeyValue(parent int32, node *unstable.Node) error {
	for it := node.Key(); it.Next(); {
		name := it.Node().Data
		if it.IsLast() {
			if s.find(parent, name) >= 0 {
				return fmt.Errorf("toml: key %s is already defined", name)
			}
			return s.checkValue(s.create(parent, name, valueKind, false), node.Value())
		}
		i := s.find(parent, name)
		if i < 0 {
			i = s.create(parent, name, kvTableKind, false)
		} else if s.entries[i].kind != kvTableKind {
			return fmt.Errorf("toml: key %s is already defined", name)
		}
		parent = i
	}
	return fmt.Errorf("toml: key-value expression without key")
}

func (s *seenTracker) checkValue(id int32, value *unstable.Node) error {
	switch value.Kind {
	case unstable.InlineTable:
		for it := value.Children(); it.Next(); {
			if err := s.checkKeyValue(id, it.Node()); err != nil {
				return err
			}
		}
	case unstable.Array:
		for it := value.Children(); it.Next(); {
			if elem := it.Node(); elem.Kind == unstable.InlineTable || elem.Kind == unstable.Array {
				if err := s.checkValue(s.create(id, nil, anonymousKind, false), elem); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
