package arp

import (
	"errors"
	"fmt"
	"reflect"
)

type StringMatcher struct {
	Value *string
	FieldMatcherProps
}

func (m *StringMatcher) Parse(parentNode interface{}, node map[interface{}]interface{}) error {
	if v, ok := node[TEST_KEY_MATCHES]; ok {
		switch val := v.(type) {
		case string:
			m.Value = &val
		default:
			return errors.New(ObjectPrintf(fmt.Sprintf(MalformedDefinitionFmt, TEST_KEY_MATCHES, TYPE_STR), parentNode))
		}
	}

	return m.ParseProps(node)
}

func (m *StringMatcher) Match(responseValue interface{}, datastore *DataStore) (bool, DataStore, error) {
	store := NewDataStore()
	typedResponseValue, ok := responseValue.(string)
	if !ok {
		m.ErrorStr = fmt.Sprintf(MismatchedMatcher, TYPE_STR, reflect.TypeOf(responseValue))
		return false, store, nil
	}

	var status bool
	var err error

	if m.Value != nil {
		resolved, err := (*datastore).ExpandVariable(*m.Value)
		if err != nil {
			return false, store, fmt.Errorf(BadVarMatcherFmt, *m.Value)
		}
		resolvedStr := varToString(resolved, *m.Value)

		switch resolvedStr {
		case Any:
			status = true
		case NotEmpty:
			status = typedResponseValue != ""
			if !status {
				m.ErrorStr = fmt.Sprintf(NotEmptyErrFmt, typedResponseValue)
			}
		default:
			status, _ = matchPattern(resolvedStr, []byte(typedResponseValue))
			if !status {
				m.ErrorStr = fmt.Sprintf(PatternErrFmt, typedResponseValue, resolvedStr)
			}
		}
	}

	if status {
		m.ErrorStr = typedResponseValue
	}
	if status && m.DSName != "" {
		err = store.PutVariable(m.DSName, responseValue)
	}
	return status, store, err
}

func (m *StringMatcher) SetError(error string) {
	m.ErrorStr = fmt.Sprintf("%v (matching '%v')", error, *m.Value)
}
