package arp

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
)

type FloatMatcher struct {
	Value    *float64
	Pattern  *string
	Exists   bool
	ErrorStr string
	DSName   string
	Priority int
}

func (m *FloatMatcher) Parse(parentNode interface{}, node map[interface{}]interface{}) error {
	if v, ok := node[TEST_KEY_MATCHES]; ok {
		switch val := v.(type) {
		case float64:
			m.Value = &val
		case int:
			floatVal := float64(val)
			m.Value = &floatVal
		case string:
			m.Pattern = &val
		default:
			return errors.New(ObjectPrintf(fmt.Sprintf(MalformedDefinitionFmt, TEST_KEY_MATCHES, TYPE_NUM), parentNode))
		}
	}
	m.DSName = getDataStoreName(node)
	m.Priority = getMatcherPriority(node)

	var err error
	m.Exists, err = getExistsFlag(node)
	return err
}

func (m *FloatMatcher) Match(responseValue interface{}, datastore *DataStore) (bool, DataStore, error) {
	store := NewDataStore()
	m.ErrorStr = ""
	if status, passthrough, message := handleExistence(responseValue, m.Exists, false); !passthrough {
		m.ErrorStr = message
		return status, store, nil
	}

	var status bool
	var err error

	typedResponseValue, ok := responseValue.(float64)
	if !ok {
		m.ErrorStr = fmt.Sprintf(MismatchedMatcher, TYPE_NUM, reflect.TypeOf(responseValue))
		return false, store, nil
	}

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
			status, err = matchPattern(resolvedStr,
				[]byte(strconv.FormatFloat(typedResponseValue, 'f', -1, 64)))

			if !status {
				m.ErrorStr = fmt.Sprintf(PatternErrFmt, typedResponseValue, resolvedStr)
			}
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

func (m *FloatMatcher) Error() string {
	return m.ErrorStr
}

func (m *FloatMatcher) GetPriority() int {
	return m.Priority
}

func (m *FloatMatcher) SetError(error string) {
	m.ErrorStr = error
}
