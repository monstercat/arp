package arp

import (
	"errors"
	"fmt"
	"reflect"
)

type ObjectMatcher struct {
	Properties map[interface{}]interface{}
	ErrorStr   string
	Exists     bool
	DSName     string
	Sorted     bool
	Priority   int
}

func (m *ObjectMatcher) Parse(parentNode interface{}, node map[interface{}]interface{}) error {
	m.DSName = getDataStoreName(node)
	m.Priority = getMatcherPriority(node)

	if node[TEST_KEY_PROPERTIES] != nil {
		if properties, ok := node[TEST_KEY_PROPERTIES].(map[interface{}]interface{}); ok {
			m.Properties = properties
		} else {
			return errors.New(ObjectPrintf(fmt.Sprintf(MalformedDefinitionFmt, TEST_KEY_PROPERTIES, TYPE_OBJ), parentNode))
		}
	}

	var err error
	m.Exists, err = getExistsFlag(node)
	return err
}

func (m *ObjectMatcher) Match(responseValue interface{}, datastore *DataStore) (bool, DataStore, error) {
	var err error
	store := NewDataStore()
	m.ErrorStr = ""
	if status, passthrough, message := handleExistence(responseValue, m.Exists, false); !passthrough {
		m.ErrorStr = message
		return status, store, nil
	}

	var typedResponseValue map[string]interface{}
	switch t := responseValue.(type) {
	case map[string]interface{}:
		typedResponseValue = t
	default:
		m.ErrorStr = fmt.Sprintf(MismatchedMatcher, TYPE_OBJ, reflect.TypeOf(responseValue))
		return false, store, nil
	}

	m.ErrorStr = "{}"

	if m.DSName != "" {
		err = store.PutVariable(m.DSName, typedResponseValue)
	}

	return true, store, err
}

func (m *ObjectMatcher) Error() string {
	return m.ErrorStr
}

func (m *ObjectMatcher) GetPriority() int {
	return m.Priority
}

func (m *ObjectMatcher) SetError(error string) {
	m.ErrorStr = error
}
