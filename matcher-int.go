package arp

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
)

type IntegerMatcher struct {
	Value    *int64
	Pattern  *string
	Exists   bool
	ErrorStr string
	DSName   string
	Priority int
}

func (m *IntegerMatcher) Parse(parentNode interface{}, node map[interface{}]interface{}) error {
	if v, ok := node[TEST_KEY_MATCHES]; ok {
		switch val := v.(type) {
		case float64:
			intVal := int64(val)
			m.Value = &intVal
		case int:
			intVal := int64(val)
			m.Value = &intVal
		case string:
			m.Pattern = &val
		default:
			return errors.New(ObjectPrintf(fmt.Sprintf(MalformedDefinitionFmt, TEST_KEY_MATCHES, TYPE_INT), parentNode))
		}
	}
	m.DSName = getDataStoreName(node)
	m.Priority = getMatcherPriority(node)

	var err error
	m.Exists, err = getExistsFlag(node)
	return err
}

func (m *IntegerMatcher) Match(responseValue interface{}, datastore *DataStore) (bool, DataStore, error) {
	store := NewDataStore()
	m.ErrorStr = ""
	if status, passthrough, message := handleExistence(responseValue, m.Exists, false); !passthrough {
		m.ErrorStr = message
		return status, store, nil
	}

	var status bool
	var err error

	var typedResponseValue int64
	switch t := responseValue.(type) {
	case float64:
		typedResponseValue = int64(t)
	case int:
		typedResponseValue = int64(t)
	case int64:
		typedResponseValue = t
	default:
		m.ErrorStr = fmt.Sprintf(MismatchedMatcher, TYPE_INT, reflect.TypeOf(responseValue))
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
			var evaluated bool
			status, evaluated, m.ErrorStr, err = evaluateNumExpr(resolvedStr, typedResponseValue)
			if !evaluated {
				status, err = matchPattern(resolvedStr,
					[]byte(strconv.FormatInt(typedResponseValue, 10)))
				if !status {
					m.ErrorStr = fmt.Sprintf(PatternErrFmt, typedResponseValue, resolvedStr)
				}
			}
		}
	}

	if status {
		m.ErrorStr = fmt.Sprintf("%d", int64(typedResponseValue))
	}

	if status && m.DSName != "" {
		err = store.PutVariable(m.DSName, responseValue)
	}

	return status, store, err
}

func (m *IntegerMatcher) Error() string {
	return m.ErrorStr
}

func (m *IntegerMatcher) GetPriority() int {
	return m.Priority
}

func (m *IntegerMatcher) SetError(error string) {
	m.ErrorStr = error
}
