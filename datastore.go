package arp

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	VAR_PREFIX = "@{"
	VAR_SUFFIX = "}"
)

type DataStore map[string]interface{}

type VariableKey struct {
	Name    string
	IsArray bool
	IsLast  bool
}

func varToString(variable interface{}, def ...string) string {
	if variable == nil {
		if len(def) > 0 {
			return def[0]
		} else {
			return ""
		}
	}
	return fmt.Sprintf("%v", variable)
}

func extractVariablePath(variableName string) []VariableKey {
	keys := strings.Split(variableName, ".")

	// Extract array indexing from the keys as their own key for iterating the datastore.
	var expandedKeys []VariableKey
	for _, k := range keys {
		hasIndex := false
		index := ""
		for i, c := range k {
			if c == '[' {
				if hasIndex {
					index = ""
				} else {
					n := strings.TrimSpace(k[:i])
					expandedKeys = append(expandedKeys, VariableKey{Name: n, IsArray: true})
				}
				hasIndex = true

				continue
			}

			if c != ']' && hasIndex {
				index += string(c)
			} else if c == ']' {
				n := strings.TrimSpace(index)
				expandedKeys = append(expandedKeys, VariableKey{Name: n})
			}
		}
		if !hasIndex {
			n := strings.TrimSpace(k)
			expandedKeys = append(expandedKeys, VariableKey{Name: n})
		}
	}
	expandedKeys[len(expandedKeys)-1].IsLast = true

	return expandedKeys
}

func (t *DataStore) resolveVariable(variable string) (interface{}, error) {
	// Extract array indexing from the keys as their own key for iterating the datastore.
	cleanedVar := variable[len(VAR_PREFIX) : len(variable)-len(VAR_SUFFIX)]
	expandedKeys := extractVariablePath(cleanedVar)

	var node interface{}
	node = *t
	for _, k := range expandedKeys {
		key := k.Name
		switch v := node.(type) {
		case DataStore:
			if nextNode, ok := v[key]; !ok {
				return "", fmt.Errorf(MissingDSKeyFmt, cleanedVar)
			} else {
				node = nextNode
			}
		case map[string]interface{}:
			if nextNode, ok := v[key]; !ok {
				return "", fmt.Errorf(MissingDSKeyFmt, cleanedVar)
			} else {
				node = nextNode
			}
		case []interface{}:
			idx, err := strconv.ParseUint(key, 10, 64)
			if err != nil {
				// should catch non integer and negative value
				return "", fmt.Errorf(BadIndexDSFmt, cleanedVar)
			}
			if idx > uint64(len(v)) {
				return "", fmt.Errorf(IndexExceedsDSFmt, cleanedVar)
			}

			node = v[idx]
		default:
			return "", fmt.Errorf(MissingDSKeyFmt, cleanedVar)
		}
	}

	return node, nil
}

// PutVariable Given a variable name (or path in a JSON object) store the value for said path.
func (t *DataStore) PutVariable(variable string, value interface{}) error {
	type Noodle struct {
		Parent      interface{}
		Node        interface{}
		ParentKey   string
		ParentIndex int64
	}

	expandedKeys := extractVariablePath(variable)
	node := Noodle{
		Node:   *t,
		Parent: *t,
	}

	for _, k := range expandedKeys {
		key := k.Name
		var temp interface{}
		switch v := node.Node.(type) {
		case DataStore:
			if nextNode, ok := v[key]; !ok {
				// insert values if it doesn't exist
				if k.IsLast {
					v[key] = value
					return nil
				} else if k.IsArray {
					temp = make([]interface{}, 1)
				} else {
					temp = make(map[string]interface{})
				}
				v[key] = temp
				node = Noodle{
					Node:        temp,
					Parent:      &v,
					ParentKey:   key,
					ParentIndex: -1,
				}
			} else {
				// otherwise overwrite existing ones
				if k.IsLast {
					v[key] = value
					return nil
				}
				node = Noodle{
					Node:        nextNode,
					Parent:      &v,
					ParentKey:   key,
					ParentIndex: -1,
				}
			}
		case map[string]interface{}:
			if nextNode, ok := v[key]; !ok {
				if k.IsLast {
					v[key] = value
					return nil
				} else if k.IsArray {
					temp = make([]interface{}, 1)
				} else {
					temp = make(map[string]interface{})
				}
				v[key] = temp
				node = Noodle{
					Node:        temp,
					Parent:      &v,
					ParentKey:   key,
					ParentIndex: -1,
				}
			} else {
				// otherwise overwrite existing ones
				if k.IsLast {
					v[key] = value
					return nil
				}
				node = Noodle{
					Node:        nextNode,
					Parent:      &v,
					ParentKey:   key,
					ParentIndex: -1,
				}
			}
		case []interface{}:
			idx, err := strconv.ParseUint(key, 10, 64)
			if err != nil {
				return fmt.Errorf(BadIndexDSFmt, variable)
			}
			// if the index is out of range, then we'll resize the array just enough to fix the index
			if idx > uint64(len(v)) {
				newArray := v[:]
				delta := (idx - uint64(len(v))) + 1
				for delta > 0 {
					delta--
					newArray = append(newArray, nil)

				}
				if node.ParentKey != "" {
					n := node.Parent.(*map[string]interface{})
					(*n)[node.ParentKey] = newArray
				} else if node.ParentIndex >= 0 {
					n := node.Parent.(*[]interface{})
					(*n)[node.ParentIndex] = newArray
				}
				v = newArray
			}
			if v[idx] == nil {
				if k.IsLast {
					v[idx] = value
					return nil
				} else if k.IsArray {
					temp = make([]interface{}, 1)
				} else {
					temp = make(map[string]interface{})
				}
				v[idx] = temp
			} else {
				if k.IsLast {
					v[idx] = value
					return nil
				}
			}
			node = Noodle{
				Node:        v[idx],
				Parent:      &v,
				ParentIndex: int64(idx),
			}
		}
	}
	return nil
}

func isVar(input string) bool {
	return strings.HasPrefix(input, VAR_PREFIX) && strings.HasSuffix(input, VAR_SUFFIX)
}

func (t *DataStore) ExpandVariable(input string) (interface{}, error) {
	var result interface{}
	var outputString string
	variables := TokenStack{}
	variables.Parse(input, VAR_PREFIX, VAR_SUFFIX)

	if len(variables.Frames) == 0 {
		return input, nil
	}

	if variables.Extra != "" {
		outputString = input
	}

	type ExtendedStackFrame struct {
		TokenStackFrame
		ResolvedVarName string
	}

	toResolve := []ExtendedStackFrame{}
	for _, v := range variables.Frames {
		toResolve = append(toResolve, ExtendedStackFrame{
			TokenStackFrame: v,
			ResolvedVarName: v.Token,
		})
	}

	for i, v := range toResolve {
		var resolvedVar interface{}
		// make sure we are only resolving strings that are variables and not values that were already resolved from
		// variables.
		if isVar(v.ResolvedVarName) {
			var err error
			resolvedVar, err = t.resolveVariable(v.ResolvedVarName)
			if err != nil {
				return nil, err
			}
		}

		if v.Nested == 0 {
			// if the input contains more text than just the variable, we can assume that it is intended to be replaced
			// within the string
			if outputString != "" {
				outputString = strings.ReplaceAll(outputString, v.Token, varToString(resolvedVar))
			} else {
				// otherwise, just return the node and it'll be converted as needed
				result = resolvedVar
			}
		}
		// once variable is resolved, we want to expand the other variables that might be composed with it
		for offset := i + 1; offset < len(toResolve); offset++ {
			frame := toResolve[offset]

			if !strings.Contains(frame.ResolvedVarName, v.Token) {
				continue
			}

			if _, ok := resolvedVar.(string); !ok {
				return nil, fmt.Errorf("failed to resolve %v as %v does not resolve to a string: %v", frame.Token, v.Token, resolvedVar)
			}
			// Assumes that people's variables are resolving to proper strings. If not, then they'll get a message
			// indicating their variable couldn't be resolved anyway.
			frame.ResolvedVarName = strings.ReplaceAll(frame.ResolvedVarName, v.Token, varToString(resolvedVar))
			toResolve[offset] = frame
		}

	}
	if outputString != "" {
		result = outputString
	}

	return result, nil
}

func (t *DataStore) RecursiveResolveVariables(input interface{}) (interface{}, error) {
	if input == nil {
		return nil, nil
	}

	switch n := input.(type) {
	case map[interface{}]interface{}:
		for k := range n {
			if node, err := t.RecursiveResolveVariables(n[k]); err != nil {
				return nil, err
			} else {
				n[k] = node
			}

		}
		return n, nil
	case map[string]interface{}:
		for k := range n {
			if node, err := t.RecursiveResolveVariables(n[k]); err != nil {
				return nil, err
			} else {
				n[k] = node
			}

		}
		return n, nil
	case []interface{}:
		for i, e := range n {
			if node, err := t.RecursiveResolveVariables(e); err != nil {
				return nil, err
			} else {
				n[i] = node
			}
		}

		return n, nil
	case []string:
		var newElements []interface{}
		for _, e := range n {
			res, err := t.ExpandVariable(e)
			if err != nil {
				return nil, err
			}
			newElements = append(newElements, res)
		}
		return newElements, nil
	case string:
		res, err := t.ExpandVariable(n)
		if res == nil {
			return input, nil
		}
		return res, err
	}

	return input, nil
}
