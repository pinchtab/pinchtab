package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// This file renders a config write as a PATCH over the bytes already on disk rather
// than as a fresh marshal of the struct.
//
// Marshalling a map[string]any would satisfy "write only changed keys" but sorts every
// key alphabetically, so a user's hand-ordered file comes back shuffled — the same
// unexplained diff this whole card exists to remove, in a different form. Preserving
// order means keeping the original member sequence, which encoding/json cannot do, so
// the document is held as an ordered member list.

type jsonMember struct {
	key   string
	value json.RawMessage
}

// parseJSONObject reads an object into its members, in file order. A non-object or
// unparseable input yields nil, which callers treat as "no shape to preserve".
func parseJSONObject(data []byte) []jsonMember {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil
	}

	var members []jsonMember
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil
		}
		members = append(members, jsonMember{key: key, value: raw})
	}
	return members
}

// patchConfigObject produces the members to write for one object level: every member
// the file already had, in its original position and carrying fc's current value, then
// the keys fc holds that the file lacked and that differ from the shipped baseline.
//
// Existing members are rewritten unconditionally so an edit back to the default value
// still lands; a pure difference-from-baseline render would omit that key and leave the
// stale value in place, which is the one way writing less can lose data.
func patchConfigObject(onDisk []jsonMember, full, shipped map[string]any) []jsonMember {
	seen := make(map[string]bool, len(onDisk))
	// The result often needs room for both collections, but adding their lengths
	// can overflow int before make validates the capacity. Start with the known
	// on-disk size and let append grow the slice for new members.
	out := make([]jsonMember, 0, len(onDisk))

	for _, member := range onDisk {
		seen[member.key] = true
		value, known := full[member.key]
		if !known {
			// A key the struct does not model: preserved verbatim rather than
			// dropped. A save must not delete what it merely failed to parse.
			out = append(out, member)
			continue
		}
		out = append(out, jsonMember{key: member.key, value: patchedValue(member.value, value, shipped[member.key])})
	}

	// Deterministic order for additions, since map iteration is not.
	added := make([]string, 0, len(full))
	for key := range full {
		if !seen[key] {
			added = append(added, key)
		}
	}
	sort.Strings(added)

	for _, key := range added {
		value := full[key]
		if nested, isObj := value.(map[string]any); isObj {
			shippedObj, _ := shipped[key].(map[string]any)
			members := patchConfigObject(nil, nested, shippedObj)
			if len(members) == 0 {
				continue
			}
			out = append(out, jsonMember{key: key, value: encodeJSONObject(members)})
			continue
		}
		if value == nil || sameJSONValue(value, shipped[key]) {
			continue
		}
		raw, err := json.Marshal(value)
		if err != nil {
			continue
		}
		out = append(out, jsonMember{key: key, value: raw})
	}
	return out
}

// patchedValue recurses into objects so a nested section keeps its own member order,
// and replaces anything else with fc's value.
func patchedValue(onDisk json.RawMessage, full, shipped any) json.RawMessage {
	fullObj, isObj := full.(map[string]any)
	if !isObj {
		raw, err := json.Marshal(full)
		if err != nil {
			return onDisk
		}
		return raw
	}
	shippedObj, _ := shipped.(map[string]any)
	return encodeJSONObject(patchConfigObject(parseJSONObject(onDisk), fullObj, shippedObj))
}

// encodeJSONObject serialises members in order, compactly. renderMinimalConfig indents
// the whole document once at the end, so no depth has to be threaded through here.
func encodeJSONObject(members []jsonMember) json.RawMessage {
	if len(members) == 0 {
		return json.RawMessage("{}")
	}
	var buf bytes.Buffer
	buf.WriteString("{")
	for i, member := range members {
		if i > 0 {
			buf.WriteString(",")
		}
		key, err := json.Marshal(member.key)
		if err != nil {
			continue
		}
		buf.Write(key)
		buf.WriteString(":")
		buf.Write(member.value)
	}
	buf.WriteString("}")
	return buf.Bytes()
}

// renderMinimalConfig patches fc over the file's existing bytes and returns indented
// JSON with the original member order intact.
func renderMinimalConfig(fc *FileConfig, existing []byte) ([]byte, error) {
	full, err := configAsMap(fc)
	if err != nil {
		return nil, err
	}
	shipped, err := configAsMap(minimalSaveBaseline())
	if err != nil {
		return nil, err
	}

	members := patchConfigObject(parseJSONObject(existing), full, shipped)
	compact := encodeJSONObject(members)

	var out bytes.Buffer
	if err := json.Indent(&out, compact, "", "  "); err != nil {
		return nil, fmt.Errorf("failed to serialize config: %w", err)
	}
	body := strings.TrimRight(out.String(), "\n")
	return []byte(body + "\n"), nil
}

// sameJSONDocument reports whether two byte slices encode the same JSON value,
// ignoring key order and whitespace. Used to decide that a write would be a no-op.
func sameJSONDocument(a, b []byte) bool {
	var left, right any
	if err := json.Unmarshal(a, &left); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &right); err != nil {
		return false
	}
	return sameJSONValue(left, right)
}
