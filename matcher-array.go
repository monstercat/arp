package arp

import (
	"errors"
	"fmt"
	"reflect"
)

type ArrayMatcher struct {
	Length    *int64
	LengthStr *string
	Items     []interface{}
	ErrorStr  string
	Exists    bool
	DSName    string
	Sorted    bool
	Priority  int
}

func (m *ArrayMatcher) Parse(parentNode interface{}, node map[interface{}]interface{}) error {
	var err error
	m.Exists, err = getExistsFlag(node)
	if err != nil {
		return err
	}

	if v, ok := node[TEST_KEY_LENGTH]; ok {
		switch val := v.(type) {
		case int:
			intVal := int64(val)
			m.Length = &intVal
		case float64:
			intVal := int64(val)
			m.Length = &intVal
		case string:
			m.LengthStr = &val
		default:
			return errors.New(ObjectPrintf(fmt.Sprintf(MalformedDefinitionFmt, TEST_KEY_LENGTH, TYPE_ARRAY), parentNode))
		}
	}

	if v, ok := node[TEST_KEY_ITEMS]; ok && m.Exists {
		if m.Items, ok = v.([]interface{}); !ok {
			return errors.New(ObjectPrintf(fmt.Sprintf(MalformedDefinitionFmt, TEST_KEY_ITEMS, TYPE_ARRAY), parentNode))
		}
	}

	if v, ok := node[TEST_KEY_SORTED]; ok {
		m.Sorted = v.(bool)
	} else {
		m.Sorted = true
	}

	m.Priority = getMatcherPriority(node)
	m.DSName = getDataStoreName(node)
	return nil
}

func (m *ArrayMatcher) Match(responseValue interface{}, datastore *DataStore) (bool, DataStore, error) {
	store := NewDataStore()
	if status, passthrough, message := handleExistence(responseValue, m.Exists, true); !passthrough {
		m.ErrorStr = message
		return status, store, nil
	}

	var typedResponseValue []interface{}
	if responseValue == nil {
		// if nil, we can still validate the length in case a non-0 value was expected
		typedResponseValue = []interface{}{}
	} else {
		var ok bool
		typedResponseValue, ok = responseValue.([]interface{})
		if !ok {
			m.ErrorStr = fmt.Sprintf(MismatchedMatcher, TYPE_ARRAY, reflect.TypeOf(responseValue))
			return false, store, nil
		}
	}
	var status bool
	var err error

	responseLength := int64(len(typedResponseValue))
	if m.Length != nil {
		status = responseLength == *m.Length
		if !status {
			m.ErrorStr = fmt.Sprintf(ArrayLengthErrFmt, "=", *m.Length, responseLength)
		}
	} else if m.LengthStr != nil {
		resolved, err := (*datastore).ExpandVariable(*m.LengthStr)
		if err != nil {
			return false, store, fmt.Errorf(BadVarMatcherFmt, *m.LengthStr)
		}
		s := varToString(resolved, *m.LengthStr)

		switch s {
		case NotEmpty:
			status = responseLength > 0
		case Any:
			status = true
		default:
			var evaluated bool
			status, evaluated, m.ErrorStr, err = evaluateNumExpr(s, responseLength)
			if evaluated && !status {
				m.ErrorStr = fmt.Sprintf("[%v] %v", TEST_KEY_LENGTH, m.ErrorStr)
			}
		}
	}
	if status {
		m.ErrorStr = fmt.Sprintf("[%v] %v", TEST_KEY_LENGTH, responseLength)
	}

	if status && m.DSName != "" {
		err = store.PutVariable(m.DSName, responseValue)
	}
	return status, store, err
}

func (m *ArrayMatcher) Error() string {
	return m.ErrorStr
}
func (m *ArrayMatcher) GetPriority() int {
	return m.Priority
}

func (m *ArrayMatcher) SetError(error string) {
	m.ErrorStr = error
}
