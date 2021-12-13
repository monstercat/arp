package arp

import (
	"errors"
	"fmt"
	"reflect"
)

type ObjectMatcher struct {
	Properties map[interface{}]interface{}
	Sorted     bool
	FieldMatcherProps
}

func (m *ObjectMatcher) Parse(parentNode interface{}, node map[interface{}]interface{}) error {
	if node[TEST_KEY_PROPERTIES] != nil {
		if properties, ok := node[TEST_KEY_PROPERTIES].(map[interface{}]interface{}); ok {
			m.Properties = properties
		} else {
			return errors.New(ObjectPrintf(fmt.Sprintf(MalformedDefinitionFmt, TEST_KEY_PROPERTIES, TYPE_OBJ), parentNode))
		}
	}

	return m.ParseProps(node)
}

func (m *ObjectMatcher) Match(responseValue interface{}, datastore *DataStore) (bool, DataStore, error) {
	var err error
	store := NewDataStore()
	m.ErrorStr = ""
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
