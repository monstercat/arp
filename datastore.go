package arp

import (
	"fmt"
	"strconv"
	"strings"
)

type DataStore map[string]interface{}

type VarStackFrame struct {
	StartPos int
	EndPos   int
	VarName  string
	Nested   int
}

type VarStack struct {
	Frames []VarStackFrame
	Extra  string
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

func (s *VarStack) Push(f VarStackFrame) {
	s.Frames = append(s.Frames, f)
}

func (s *VarStack) Pop() *VarStackFrame {
	if len(s.Frames) == 0 {
		return nil
	}
	result := s.Frames[len(s.Frames)-1]
	s.Frames = s.Frames[:len(s.Frames)-1]
	return &result
}

func (f *VarStackFrame) IsValid() bool {
	return f.StartPos != f.EndPos && f.VarName != ""
}

func parseVar(input string) VarStack {
	varStack := VarStack{}
	resultStack := VarStack{}
	var curStackFrame *VarStackFrame
	for i := 0; i < len(input); i++ {
		char := input[i]
		if char == '@' && i+1 < len(input) && input[i+1] == '{' {
			nestLevel := 0
			if curStackFrame != nil {
				varStack.Push(*curStackFrame)
				nestLevel = curStackFrame.Nested + 1
			}
			curStackFrame = &VarStackFrame{}
			curStackFrame.StartPos = i
			curStackFrame.Nested = nestLevel
		} else if curStackFrame != nil && char == '}' {
			curStackFrame.EndPos = i
			curStackFrame.VarName = input[curStackFrame.StartPos : curStackFrame.EndPos+1]
			resultStack.Push(*curStackFrame)
			curStackFrame = varStack.Pop()
		} else if curStackFrame == nil {
			resultStack.Extra += string(char)
		}
	}

	return resultStack
}

func (t *DataStore) resolveVariable(variable string) (interface{}, error) {
	keys := strings.Split(variable, ".")

	// Extract array indexing from the keys as their own key for iterating the datastore.
	var expandedKeys []string
	for _, k := range keys {
		hasIndex := false
		index := ""
		for i, c := range k {
			if c == '[' {
				hasIndex = true
				expandedKeys = append(expandedKeys, k[:i])
				continue
			}

			if c != ']' && hasIndex {
				index += string(c)
			} else if c == ']' {
				expandedKeys = append(expandedKeys, index)
			}
		}
		if !hasIndex {
			expandedKeys = append(expandedKeys, k)
		}
	}

	var node interface{}
	node = *t
	for _, k := range expandedKeys {
		switch v := node.(type) {
		case DataStore:
			if nextNode, ok := v[k]; !ok {
				return "", fmt.Errorf(MissingDSKeyFmt, variable)
			} else {
				node = nextNode
			}
		case map[string]interface{}:
			if nextNode, ok := v[k]; !ok {
				return "", fmt.Errorf(MissingDSKeyFmt, variable)
			} else {
				node = nextNode
			}
		case []interface{}:
			idx, err := strconv.ParseUint(k, 10, 64)
			if err != nil {
				// should catch non integer and negative value
				return "", fmt.Errorf(BadIndexDSFmt, variable)
			}
			if idx > uint64(len(v)) {
				return "", fmt.Errorf(IndexExceedsDSFmt, variable)
			}

			node = v[idx]
		}
	}

	return node, nil
}

func (t *DataStore) ExpandVariable(input string) (interface{}, error) {
	var result interface{}
	var outputString string
	variables := parseVar(input)

	if len(variables.Frames) == 0 {
		return input, nil
	}

	if variables.Extra != "" {
		outputString = input
	}

	type ExtendedStackFrame struct {
		VarStackFrame
		ResolvedVarName string
	}

	toResolve := []ExtendedStackFrame{}
	for _, v := range variables.Frames {
		toResolve = append(toResolve, ExtendedStackFrame{
			VarStackFrame:   v,
			ResolvedVarName: v.VarName,
		})
	}

	for i, v := range toResolve {
		var resolvedVar interface{}
		// make sure we are only resolving strings that are variables and not strings that were already resolved from
		// variables.
		if strings.HasPrefix(v.ResolvedVarName, "@{") && strings.HasSuffix(v.ResolvedVarName, "}") {
			var err error
			varKey := strings.ReplaceAll(strings.ReplaceAll(v.ResolvedVarName, "@{", ""), "}", "")
			resolvedVar, err = t.resolveVariable(varKey)
			if err != nil {
				return nil, err
			}
		}

		if v.Nested == 0 {
			// if the input contains more text than just the variable, we can assume that it is intended to be replaced
			// within the string
			if outputString != "" {
				outputString = strings.ReplaceAll(outputString, v.VarName, varToString(resolvedVar))
			} else {
				// otherwise, just return the node and it'll be converted as needed
				result = resolvedVar
			}
		}
		// once variable is resolved, we want to expand the other variables that might be composed with it
		for offset := i + 1; offset < len(toResolve); offset++ {
			frame := toResolve[offset]

			if !strings.Contains(frame.ResolvedVarName, v.VarName) {
				continue
			}

			if _, ok := resolvedVar.(string); !ok {
				return nil, fmt.Errorf("failed to resolve %v as %v does not resolve to a string: %v", frame.VarName, v.VarName, resolvedVar)
			}
			// Assumes that people's variables are resolving to proper strings. If not, then they'll get a message
			// indicating their variable couldn't be resolved anyway.
			frame.ResolvedVarName = strings.ReplaceAll(frame.ResolvedVarName, v.VarName, varToString(resolvedVar))
			toResolve[offset] = frame
		}

	}
	if outputString != "" {
		result = outputString
	}

	return result, nil
}

func (t *DataStore) resolveDataStoreVarRecursive(input interface{}) (interface{}, error) {
	if input == nil {
		return nil, nil
	}

	switch n := input.(type) {
	case map[interface{}]interface{}:
		for k := range n {
			if node, err := t.resolveDataStoreVarRecursive(n[k]); err != nil {
				return nil, err
			} else {
				n[k] = node
			}

		}
		return n, nil
	case map[string]interface{}:
		for k := range n {
			if node, err := t.resolveDataStoreVarRecursive(n[k]); err != nil {
				return nil, err
			} else {
				n[k] = node
			}

		}
		return n, nil
	case []interface{}:
		for i, e := range n {
			if node, err := t.resolveDataStoreVarRecursive(e); err != nil {
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
