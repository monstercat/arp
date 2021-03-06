package arp

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
)

type BoolMatcher struct {
	Value   *bool
	Pattern *string
	FieldMatcherProps
}

func (m *BoolMatcher) Parse(parentNode interface{}, node map[interface{}]interface{}) error {
	if v, ok := node[TEST_KEY_MATCHES]; ok {
		switch val := v.(type) {
		case bool:
			m.Value = &val
		case string:
			m.Pattern = &val
		default:
			return errors.New(ObjectPrintf(fmt.Sprintf(MalformedDefinitionFmt, TEST_KEY_MATCHES, TYPE_BOOL), parentNode))
		}
	}
	return m.ParseProps(node)
}

func (m *BoolMatcher) Match(responseValue interface{}, datastore *DataStore) (bool, DataStore, error) {
	store := NewDataStore()
	m.ErrorStr = ""
	typedResponseValue, ok := responseValue.(bool)
	if !ok {
		m.ErrorStr = fmt.Sprintf(MismatchedMatcher, TYPE_BOOL, reflect.TypeOf(responseValue))
		return false, store, nil
	}

	var status bool
	var err error

	if m.Value != nil {
		status = *m.Value == typedResponseValue
		if !status {
			m.ErrorStr = fmt.Sprintf(ValueErrFmt, *m.Value, typedResponseValue)
		}
	} else if m.Pattern != nil {
		resolved, err := (*datastore).ExpandVariable(*m.Pattern)
		if err != nil {
			return false, store, fmt.Errorf(BadVarMatcherFmt, *m.Pattern)
		}
		resolvedStr := varToString(resolved, *m.Pattern)
		if resolvedStr == Any {
			status = true
		} else {
			var res bool
			res, err = strconv.ParseBool(resolvedStr)
			result := res == typedResponseValue
			if !result {
				m.ErrorStr = fmt.Sprintf(ValueErrFmt, res, typedResponseValue)
			}
			status = err != nil && result
		}
	}

	if status {
		m.ErrorStr = fmt.Sprintf("%v", typedResponseValue)
	}

	if status && m.DSName != "" {
		err = store.PutVariable(m.DSName, responseValue)
	}
	return status, store, err
}
