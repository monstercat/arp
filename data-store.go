package arp

import (
	"fmt"
	"strings"
)

const (
	VAR_PREFIX = "@{"
	VAR_SUFFIX = "}"
)

type DataStore struct {
	Store map[string]interface{}
}

func isVar(input string) bool {
	return strings.HasPrefix(input, VAR_PREFIX) && strings.HasSuffix(input, VAR_SUFFIX)
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

func NewDataStore() DataStore {
	return DataStore{
		Store: make(map[string]interface{}),
	}
}

func (t *DataStore) Put(key string, value interface{}) {
	t.Store[key] = value
}

func (t *DataStore) Get(key string) interface{} {
	return t.Store[key]
}

func (t *DataStore) resolveVariable(variable string) (interface{}, error) {
	cleanedVar := variable[len(VAR_PREFIX) : len(variable)-len(VAR_SUFFIX)]
	return GetJsonValue(t.Store, cleanedVar)
}

// PutVariable Given a variable name (or path in a JSON object) store the value for said path.
func (t *DataStore) PutVariable(variable string, value interface{}) error {
	return PutJsonValue(t.Store, variable, value)
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
