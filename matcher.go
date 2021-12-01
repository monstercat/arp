package arp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	Any      = "$any"
	NotEmpty = "$notEmpty"
	LT       = "$<"
	LTE      = "$<="
	GT       = "$>"
	GTE      = "$>="
	EQ       = "$="

	FIELD_KEY_PREFIX = "$."

	// special keywords used in validation object definitions
	TEST_KEY_TYPE       = "type"
	TEST_KEY_PROPERTIES = "properties"
	TEST_KEY_LENGTH     = "length"
	TEST_KEY_ITEMS      = "items"
	TEST_KEY_SORTED     = "sorted"
	TEST_KEY_STORE      = "storeAs"
	TEST_KEY_PRIORITY   = "priority"
	TEST_KEY_MATCHES    = "matches"
	TEST_KEY_EXISTS     = "exists"

	TEST_EXEC_KEY_RETURN_CODE = "returns"
	TEST_EXEC_KEY_BIN_PATH    = "bin"
	TEST_EXEC_KEY_ARGS        = "args"

	ValueErrFmt            = "Expected value '%v' did not match the actual value '%v'"
	PatternErrFmt          = "Failed to match actual value '%v' with expected pattern: '%v'"
	NotEmptyErrFmt         = "Expected non-empty value, but got value '%v' instead."
	ArrayLengthErrFmt      = "Expected array with length %v %v but found length %v instead."
	ReceivedNullErrFmt     = "Received null value when non-null value was expected"
	ExpectedNullErrFmt     = "Expected null value when non-null value was returned"
	ExpectedNullSuccessFmt = "[Expected] %v"
	MalformedDefinitionFmt = "\nMalformed '%v' field detected on %v"
	MismatchedMatcher      = "Test expected a value type matching '%v' but response field is of type '%v'."
	BadVarMatcherFmt       = "Failed to resolve variable within matcher: %v"
	NumExpressionErrFmt    = "Expected a result evaluating to: %v %v but got %v instead"
	BadArrayElementFmt     = "\nExpected elements on '%v' to be objects"
	BadObjectFmt           = "\nExpected property '%v' to map to an object"

	// available field matchers
	TYPE_INT   = "integer"
	TYPE_NUM   = "number"
	TYPE_STR   = "string"
	TYPE_ARRAY = "array"
	TYPE_OBJ   = "object"
	TYPE_BOOL  = "bool"
	TYPE_EXEC  = "external"

	DEFAULT_PRIORITY = 9999
)

type FieldMatcher interface {
	GetPriority() int
	Parse(parentNode interface{}, node map[interface{}]interface{}) error
	Match(field interface{}, datastore *DataStore) (bool, DataStore, error)
	Error() string
	SetError(error string)
}

type IntegerMatcher struct {
	Value    *int64
	Pattern  *string
	Exists   bool
	ErrorStr string
	DSName   string
	Priority int
}

type FloatMatcher struct {
	Value    *float64
	Pattern  *string
	Exists   bool
	ErrorStr string
	DSName   string
	Priority int
}

type BoolMatcher struct {
	Value    *bool
	Pattern  *string
	ErrorStr string
	Exists   bool
	DSName   string
	Priority int
}

type StringMatcher struct {
	Value    *string
	ErrorStr string
	Exists   bool
	DSName   string
	Priority int
}

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

type ObjectMatcher struct {
	Properties map[interface{}]interface{}
	ErrorStr   string
	Exists     bool
	DSName     string
	Sorted     bool
	Priority   int
}

type ExecutableMatcher struct {
	ReturnCode *int
	BinPath    string
	PrgmArgs   []string
	ErrorStr   string
	Exists     bool
	DSName     string
	Priority   int
}

type FieldMatcherKey struct {
	Name    string
	RealKey JsonKey
}

func (f *FieldMatcherKey) GetDisplayName() string {
	return f.Name
}

func (f *FieldMatcherKey) GetJsonKey() string {
	return f.RealKey.Name
}

type FieldMatcherPath struct {
	Keys         []FieldMatcherKey
	Sorted       bool
	IsExecutable bool
}

func (f *FieldMatcherPath) getObjectPath(length int) (string, []FieldMatcherKey) {
	var jsonKeys []JsonKey
	for _, k := range f.Keys {
		jsonKeys = append(jsonKeys, k.RealKey)
	}

	p, remaining := GetJsonPath(jsonKeys, length)

	totalKeys := len(f.Keys)
	found := len(remaining)
	remainingFieldKey := f.Keys[totalKeys-found:]

	return p, remainingFieldKey
}

func (f *FieldMatcherPath) GetPath() string {
	path, _ := f.getObjectPath(len(f.Keys))
	return path
}

func (f *FieldMatcherPath) GetDisplayPath() string {
	var jsonKeys []JsonKey
	for _, k := range f.Keys {
		newKey := k.RealKey
		newKey.Name = k.GetDisplayName()
		jsonKeys = append(jsonKeys, newKey)
	}

	p, _ := GetJsonPath(jsonKeys, len(f.Keys))
	return p
}

type FieldMatcherConfig struct {
	Matcher       FieldMatcher
	ObjectKeyPath FieldMatcherPath
}

type FieldMatcherResult struct {
	Status          bool
	ObjectKeyPath   string
	Error           string
	ShowExtendedMsg bool
	IgnoreResult    bool
}

type ResponseMatcher struct {
	DS        *DataStore
	Config    []*FieldMatcherConfig
	NodeCache NodeCache
}

type ResponseMatcherResults struct {
	Status     bool
	Results    []*FieldMatcherResult
	DeferCheck bool
	Err        error
}

type DepthMatchResponseNode struct {
	Status         bool
	Node           interface{}
	NodePath       string
	MatchedNodeKey bool
}
type DepthMatchResponse struct {
	FoundNode DepthMatchResponseNode
	NodeChain []*DepthMatchResponseNode
}

type NodeCacheObj struct {
	Node interface{}
}

type NodeCache struct {
	Cache map[string]NodeCacheObj
}

func (nc *NodeCache) LookUp(matcher *FieldMatcherConfig) (interface{}, []FieldMatcherKey) {
	var node interface{}

	nodePath, keys := matcher.ObjectKeyPath.getObjectPath(len(matcher.ObjectKeyPath.Keys))
	distance := 0
	for nodePath != "" && len(matcher.ObjectKeyPath.Keys)-1-distance >= 0 {
		if cachedNode, ok := nc.Cache[nodePath]; ok {
			node = cachedNode.Node
			if distance == 0 {
				// exact node match means we can skip trying to iterate on its sub nodes below
				keys = []FieldMatcherKey{}
			}
			break
		}
		distance++
		nodePath, keys = matcher.ObjectKeyPath.getObjectPath(len(matcher.ObjectKeyPath.Keys) - 1 - distance)
	}

	return node, keys
}

func matchPattern(pattern string, field []byte) (bool, error) {
	return regexp.Match(pattern, field)
}

func getExistsFlag(node map[interface{}]interface{}) (bool, error) {
	if v, ok := node[TEST_KEY_EXISTS]; ok {
		switch val := v.(type) {
		case string:
			return strconv.ParseBool(val)
		case bool:
			return val, nil
		}
	}
	return true, nil
}

func getDataStoreName(node map[interface{}]interface{}) string {
	if v, ok := node[TEST_KEY_STORE]; ok {
		return v.(string)
	}
	return ""
}

func getMatcherPriority(node map[interface{}]interface{}) int {
	if v, ok := node[TEST_KEY_PRIORITY]; ok {
		switch val := v.(type) {
		case int:
			return val
		case float64:
			return int(val)
		case int64:
			return int(val)
		}
	}

	// default to a low priority to make setting high priority matchers easier
	return DEFAULT_PRIORITY
}

func handleExistence(node interface{}, exists bool, canBeNull bool) (bool, bool, string) {
	if node == nil && exists && !canBeNull {
		return false, false, ReceivedNullErrFmt
	} else if node == nil && !exists {
		return true, false, fmt.Sprintf(ExpectedNullSuccessFmt, node)
	} else if node != nil && !exists {
		return false, false, ExpectedNullErrFmt
	}
	// status, passthrough, message
	return false, true, ""
}

func evaluateNumExpr(exprStr string, number int64) (bool, bool, string, error) {
	var err error
	var status bool
	var evaluated bool
	message := ""
	// order from longest string to shortest
	for _, op := range []string{GTE, LTE, GT, LT} {
		if strings.HasPrefix(exprStr, op) {
			evaluated = true
			var val int64
			val, err = strconv.ParseInt(strings.TrimSpace(strings.ReplaceAll(exprStr, op, "")), 10, 32)
			if err != nil {
				return false, evaluated, "", err
			}
			switch op {
			case LT:
				status = number < val
			case LTE:
				status = number <= val
			case GT:
				status = number > val
			case GTE:
				status = number >= val
			}

			if !status {
				op := strings.TrimPrefix(op, "$")
				message = fmt.Sprintf(NumExpressionErrFmt, op, val, number)
			}
		}
	}

	return status, evaluated, message, err
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
	m.DSName = getDataStoreName(node)
	m.Priority = getMatcherPriority(node)

	var err error
	m.Exists, err = getExistsFlag(node)
	return err
}

func (m *BoolMatcher) Match(responseValue interface{}, datastore *DataStore) (bool, DataStore, error) {
	store := NewDataStore()
	m.ErrorStr = ""
	if status, passthrough, message := handleExistence(responseValue, m.Exists, false); !passthrough {
		m.ErrorStr = message
		return status, store, nil
	}

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

func (m *BoolMatcher) Error() string {
	return m.ErrorStr
}

func (m *BoolMatcher) GetPriority() int {
	return m.Priority
}

func (m *BoolMatcher) SetError(error string) {
	m.ErrorStr = error
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

	m.DSName = getDataStoreName(node)
	m.Priority = getMatcherPriority(node)

	var err error
	m.Exists, err = getExistsFlag(node)
	return err
}

func (m *StringMatcher) Match(responseValue interface{}, datastore *DataStore) (bool, DataStore, error) {
	store := NewDataStore()
	if status, passthrough, message := handleExistence(responseValue, m.Exists, false); !passthrough {
		m.ErrorStr = message
		return status, store, nil
	}

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

func (m *StringMatcher) Error() string {
	return m.ErrorStr
}

func (m *StringMatcher) GetPriority() int {
	return m.Priority
}

func (m *StringMatcher) SetError(error string) {
	m.ErrorStr = fmt.Sprintf("%v (matching '%v')", error, *m.Value)
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

func (m *ExecutableMatcher) Parse(parentNode interface{}, node map[interface{}]interface{}) error {
	// expected return value of the programs execution
	if code, ok := node[TEST_EXEC_KEY_RETURN_CODE]; ok {
		if codeInt, cOk := code.(int); cOk {
			m.ReturnCode = &codeInt
		} else {
			return errors.New(ObjectPrintf(fmt.Sprintf(MalformedDefinitionFmt, TEST_EXEC_KEY_RETURN_CODE, TYPE_INT), parentNode))
		}
	}

	// path of the program to execute
	if binPath, ok := node[TEST_EXEC_KEY_BIN_PATH]; ok {
		if p, pOk := binPath.(string); pOk {
			m.BinPath = p
		} else {
			return errors.New(ObjectPrintf(fmt.Sprintf(MalformedDefinitionFmt, TEST_EXEC_KEY_BIN_PATH, TYPE_STR), parentNode))
		}
	}

	// collect the arguments to run
	if prgmArgs, ok := node[TEST_EXEC_KEY_ARGS]; ok {
		if args, aOk := prgmArgs.([]interface{}); aOk {
			for _, a := range args {
				if curArg, cAOk := a.(string); cAOk {
					m.PrgmArgs = append(m.PrgmArgs, curArg)
				} else {
					return errors.New(ObjectPrintf(fmt.Sprintf(MalformedDefinitionFmt, TEST_EXEC_KEY_ARGS, TYPE_STR), parentNode))
				}
			}
		} else {
			return errors.New(ObjectPrintf(fmt.Sprintf(MalformedDefinitionFmt, TEST_EXEC_KEY_ARGS, TYPE_ARRAY), parentNode))
		}
	}

	m.DSName = getDataStoreName(node)
	m.Priority = getMatcherPriority(node)

	var err error
	m.Exists, err = getExistsFlag(node)
	return err
}

func (m *ExecutableMatcher) Match(responseValue interface{}, datastore *DataStore) (bool, DataStore, error) {
	store := NewDataStore()
	m.ErrorStr = ""
	if status, passthrough, message := handleExistence(responseValue, m.Exists, false); !passthrough {
		m.ErrorStr = message
		return status, store, nil
	}
	// expect all inputs to be formatted as a string to pass as an input to the program
	typedResponseValue := fmt.Sprintf("%v", responseValue)

	// immediately store value into datastore so it can be resolved as a variable for program inputs
	if m.DSName != "" {
		if err := (*datastore).PutVariable(m.DSName, typedResponseValue); err != nil {
			return false, store, err
		}
	}

	resolvedBinPath, err := datastore.ExpandVariable(m.BinPath)
	if err != nil {
		return false, store, fmt.Errorf(BadVarMatcherFmt, m.BinPath)
	}

	// resolve variables in the program
	resolvedArgs, argErr := datastore.RecursiveResolveVariables(m.PrgmArgs)
	if argErr != nil {
		return false, store, fmt.Errorf(BadVarMatcherFmt, m.PrgmArgs)
	}

	argArray, aOk := resolvedArgs.([]interface{})
	if !aOk {
		m.ErrorStr = fmt.Sprintf(MismatchedMatcher, TYPE_ARRAY, reflect.TypeOf(resolvedArgs))
		return false, store, nil
	}

	var argStrings []string
	for _, aA := range argArray {
		if s, isStr := aA.(string); isStr {
			argStrings = append(argStrings, s)
		} else {
			b, _ := json.Marshal(aA)
			argStrings = append(argStrings, string(b))
		}
	}

	status := true
	cmd := exec.Command(resolvedBinPath.(string), argStrings...)

	result, err := cmd.CombinedOutput()
	sanitizedResult := string(result)

	if m.ReturnCode != nil {
		status = *m.ReturnCode == cmd.ProcessState.ExitCode()
	}

	if !status && err != nil {
		m.ErrorStr = fmt.Sprintf("[%v]\n %v", err.Error(), sanitizedResult)
		status = false
	} else {
		m.ErrorStr = sanitizedResult
	}

	return status, store, nil
}

func (m *ExecutableMatcher) Error() string {
	return m.ErrorStr
}

func (m *ExecutableMatcher) GetPriority() int {
	return m.Priority
}

func (m *ExecutableMatcher) SetError(error string) {
	m.ErrorStr = error
}

func NewResponseMatcher(ds *DataStore) ResponseMatcher {
	return ResponseMatcher{
		DS: ds,
		NodeCache: NodeCache{
			Cache: make(map[string]NodeCacheObj),
		},
	}
}

func (r *ResponseMatcher) AddMatcherConfig(config *FieldMatcherConfig) {
	// Do a dumb check for a duplicate matcher. This can happen
	// when a config contains a mix of short json path defined matchers
	// and the long exploded form of matchers.
	searchKey := config.ObjectKeyPath.GetPath()

	exists := false
	for _, c := range r.Config {
		// ToDo: At some point we should check if we're adding or skipping a
		// more specific matcher. At the moment it's possible to override a more specific
		// matcher with a generalized one.
		if c.ObjectKeyPath.GetPath() == searchKey {
			exists = true
			break
		}
	}

	if !exists {
		r.Config = append(r.Config, config)
	}
}

// If the field matcher is defined as an object, we'll parse the data to create our matchers
func (r *ResponseMatcher) loadField(parentNode interface{}, fieldNode map[interface{}]interface{}, paths FieldMatcherPath) error {
	// No 'simplified' version of objects since there is a possibility that our 'type' key used for parsing may collide with a 'type'
	// field in the data structure that is unrelated to the test definition.
	// This could be avoided by using some scoped key like '$arp_type' or something. Will need to collect feedback on what people prefer.
	typeField, ok := fieldNode[TEST_KEY_TYPE]
	if !ok {
		return fmt.Errorf(ObjectPrintf(
			fmt.Sprintf("Failed to parse response validation. Missing field '%v'", TEST_KEY_TYPE), parentNode))
	}

	typeStr, ok := typeField.(string)
	if !ok {
		return fmt.Errorf(ObjectPrintf(
			fmt.Sprintf("Failed to parse response validation. Field '%v' must be a string", TEST_KEY_TYPE), parentNode))
	}

	var foundMatcher FieldMatcher
	switch typeStr {
	case TYPE_INT:
		intMatcher := &IntegerMatcher{}
		if err := intMatcher.Parse(parentNode, fieldNode); err != nil {
			return err
		}
		foundMatcher = intMatcher
	case TYPE_NUM:
		floatMatcher := &FloatMatcher{}
		if err := floatMatcher.Parse(parentNode, fieldNode); err != nil {
			return err
		}
		foundMatcher = floatMatcher
	case TYPE_STR:
		strMatcher := &StringMatcher{}
		if err := strMatcher.Parse(parentNode, fieldNode); err != nil {
			return err
		}
		foundMatcher = strMatcher
	case TYPE_BOOL:
		boolMatcher := &BoolMatcher{}
		if err := boolMatcher.Parse(parentNode, fieldNode); err != nil {
			return err
		}
		foundMatcher = boolMatcher
	case TYPE_ARRAY:
		arrayMatcher := &ArrayMatcher{}
		if err := arrayMatcher.Parse(parentNode, fieldNode); err != nil {
			return err
		}
		foundMatcher = arrayMatcher
	case TYPE_OBJ:
		objMatcher := &ObjectMatcher{}
		if err := objMatcher.Parse(parentNode, fieldNode); err != nil {
			return err
		}
		foundMatcher = objMatcher
	case TYPE_EXEC:
		execMatcher := &ExecutableMatcher{}
		if err := execMatcher.Parse(parentNode, fieldNode); err != nil {
			return err
		}
		foundMatcher = execMatcher
		paths.IsExecutable = true

	default:
		return errors.New(ObjectPrintf(fmt.Sprintf(MalformedDefinitionFmt, TEST_KEY_TYPE, "definition"), fieldNode))
	}

	if foundMatcher != nil {
		r.AddMatcherConfig(&FieldMatcherConfig{
			Matcher:       foundMatcher,
			ObjectKeyPath: paths,
		})

		// visit array elements AFTER we have added the array to the config
		switch val := foundMatcher.(type) {
		case *ArrayMatcher:
			if err := r.loadArrayFields(val, parentNode, val.Items, paths); err != nil {
				return err
			}
		case *ObjectMatcher:
			last := &paths.Keys[len(paths.Keys)-1]
			last.RealKey.IsObject = true
			if err := r.loadObjectFields(parentNode, val.Properties, paths); err != nil {
				return err
			}
		}
	}

	return nil
}

// If our field matcher is NOT defined as an object, we'll just create a default "exact" matcher based on the type of the value in the definition.
// This cannot support resolution of datastore variables since it won't be able to determine what type matcher to use from the resolved value until the value
// is resolved at run time.
func (r *ResponseMatcher) loadSimplifiedField(parentNode interface{}, fieldNode interface{}, paths FieldMatcherPath) error {
	typeCheck := fieldNode
	var foundMatcher FieldMatcher
	switch v := typeCheck.(type) {
	case string:
		foundMatcher = &StringMatcher{
			Value:    &v,
			Exists:   true,
			Priority: DEFAULT_PRIORITY,
		}
	case float64:
		foundMatcher = &FloatMatcher{
			Value:    &v,
			Exists:   true,
			Priority: DEFAULT_PRIORITY,
		}
	case int:
		foundInt := int64(v)
		foundMatcher = &IntegerMatcher{
			Value:    &foundInt,
			Exists:   true,
			Priority: DEFAULT_PRIORITY,
		}
	case bool:
		foundMatcher = &BoolMatcher{
			Value:    &v,
			Exists:   true,
			Priority: DEFAULT_PRIORITY,
		}
	case []interface{}:
		defaultLength := NotEmpty
		foundMatcher = &ArrayMatcher{
			LengthStr: &defaultLength,
			Items:     v,
			Exists:    true,
			Sorted:    true,
			Priority:  DEFAULT_PRIORITY,
		}
	case map[string]interface{}:
		newMap := make(map[interface{}]interface{})
		for key, val := range v {
			newMap[key] = val
		}

		parent := make(map[interface{}]interface{})
		parent[TEST_KEY_PROPERTIES] = newMap

		objMatcher := &ObjectMatcher{}
		if err := objMatcher.Parse(parentNode, parent); err != nil {
			return err
		}
		foundMatcher = objMatcher
	}

	if foundMatcher != nil {
		r.AddMatcherConfig(&FieldMatcherConfig{
			Matcher:       foundMatcher,
			ObjectKeyPath: paths,
		})
	}

	switch val := foundMatcher.(type) {
	case *ArrayMatcher:
		if err := r.loadArrayFields(val, parentNode, val.Items, paths); err != nil {
			return err
		}
	case *ObjectMatcher:
		last := &paths.Keys[len(paths.Keys)-1]
		last.RealKey.IsObject = true
		if err := r.loadObjectFields(parentNode, val.Properties, paths); err != nil {
			return err
		}
	}

	return nil
}

func (r *ResponseMatcher) loadArrayFields(m *ArrayMatcher, parentNode interface{}, fields []interface{}, paths FieldMatcherPath) error {
	for i, arrayNode := range fields {
		var pathStack []FieldMatcherKey
		pathStack = append(pathStack, paths.Keys...)

		k := fmt.Sprintf("%v", i)
		pathStack = append(pathStack, FieldMatcherKey{
			Name: k,
			RealKey: JsonKey{
				Name:           k,
				IsArrayElement: true,
			},
		})

		newPaths := FieldMatcherPath{
			Keys:   pathStack,
			Sorted: m.Sorted,
		}

		if arrayNode == nil {
			continue
		}

		fieldNode, ok := arrayNode.(map[interface{}]interface{})
		if !ok {
			if err := r.loadSimplifiedField(parentNode, arrayNode, newPaths); err != nil {
				return err
			}
		} else {
			if err := r.loadField(parentNode, fieldNode, newPaths); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *ResponseMatcher) loadObjectFields(parentNode interface{}, fields map[interface{}]interface{}, paths FieldMatcherPath) error {

	for k := range fields {
		var pathStack []FieldMatcherKey
		pathStack = append(pathStack, paths.Keys...)

		var target interface{}
		keyDisplayName := k.(string)
		realKey := keyDisplayName

		if strings.HasPrefix(keyDisplayName, FIELD_KEY_PREFIX) {
			sanitized := strings.TrimPrefix(keyDisplayName, FIELD_KEY_PREFIX)
			keys := SplitJsonPath(sanitized)
			realKey = keys[0].Name
			keyDisplayName = realKey
			if strings.ContainsAny(keyDisplayName, JSON_RESERVED_CHARS) {
				keyDisplayName = "`" + keyDisplayName + "`"
			}

			// Create a new brnach of the test config with the exploded keys pointing to our value
			// that will be iterated to generate matchers for.
			tempStore := make(map[string]interface{})
			PutJsonValue(tempStore, sanitized, fields[k])
			target = tempStore[realKey]
		} else {
			target = fields[k]

		}
		pathStack = append(pathStack, FieldMatcherKey{
			Name: keyDisplayName,
			RealKey: JsonKey{
				Name: realKey,
			},
		})

		newPaths := FieldMatcherPath{
			Keys:   pathStack,
			Sorted: paths.Sorted,
		}

		// only yaml defined objects should use the non-simplified loading
		// json objects can bypass this since those are internally generated in specific
		// cases.
		fieldNode, ok := target.(map[interface{}]interface{})
		if !ok {
			if err := r.loadSimplifiedField(parentNode, target, newPaths); err != nil {
				return err
			}
		} else {
			if err := r.loadField(parentNode, fieldNode, newPaths); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *ResponseMatcher) depthMatch(node interface{}, matcher *FieldMatcherConfig, path string, key string) DepthMatchResponse {
	status, _, _ := matcher.Matcher.Match(node, r.DS)
	if status {
		result := DepthMatchResponse{
			FoundNode: DepthMatchResponseNode{
				Status:         status,
				Node:           node,
				NodePath:       path,
				MatchedNodeKey: false,
			},
		}

		result.NodeChain = append(result.NodeChain, &result.FoundNode)
		return result
	}

	switch n := node.(type) {
	case map[string]interface{}:
		for k := range n {
			result := r.depthMatch(n[k], matcher, path+"."+k, key)
			if result.FoundNode.Status {
				if !result.FoundNode.MatchedNodeKey && k == key {
					result.FoundNode.MatchedNodeKey = true
				}
				result.NodeChain = append(result.NodeChain, &DepthMatchResponseNode{
					Node:     node,
					NodePath: path + "{}",
				})

				return result
			}
		}
	case []interface{}:
		for index, i := range n {
			result := r.depthMatch(i, matcher, path+fmt.Sprintf("[%v]", index), key)
			if result.FoundNode.Status {
				result.NodeChain = append(result.NodeChain, &DepthMatchResponseNode{
					Node:     node,
					NodePath: path,
				})

				return result
			}
		}
	}

	return DepthMatchResponse{
		FoundNode: DepthMatchResponseNode{
			Status: false,
		},
	}
}

func (r *ResponseMatcher) SortConfigs() {
	// Sort configs by key length (parent objects get evaluated first) AND
	// by priority ordering of the matchers within that key length
	configs := r.Config[:]
	sort.Slice(configs, func(i, j int) bool {
		a := configs[i]
		b := configs[j]
		return a.Matcher.GetPriority() <= b.Matcher.GetPriority() && len(a.ObjectKeyPath.Keys) <= len(b.ObjectKeyPath.Keys)

	})
	r.Config = configs
}

func (r *ResponseMatcher) validateEmpty(response interface{}) (isValid bool) {
	// if no validation is provided on the response, ignore it even if we get a response from the API
	// To validate non-existence, the TEST_KEY_EXISTS flag should be used on the validation definition
	if len(r.Config) == 0 {
		return true
	}

	if response == nil {
		return false
	}

	// Look for non-empty responses. We can partially validate a response so we don't want to
	// do a straight up len(r.Config) = len(response)
	if obj, ok := response.(map[string]interface{}); ok {
		return len(obj) > 0
	}

	if ary, ok := response.([]interface{}); ok {
		return len(ary) > 0
	}

	return false
}

// Given an input key, return a JSON node representing the key contents
type KeyProcessor func(key FieldMatcherKey) interface{}

func (r *ResponseMatcher) MatchConfig(matcher *FieldMatcherConfig, response interface{}, keyProcessor KeyProcessor) ResponseMatcherResults {
	var results []*FieldMatcherResult
	var node interface{}
	node = response
	pathStr := ""

	// look up any cached nodes from the most specific path to the most generic
	lookupNode, keys := r.NodeCache.LookUp(matcher)
	if lookupNode != nil {
		node = lookupNode
	}
	_, isObjMatcher := matcher.Matcher.(*ObjectMatcher)

	// If we are looking for an object in an unsorted array, we need to locate the object using the
	// more specific property field matchers within it.
	// Once we find a match based on those properties, then
	// we can get the cached node result associated with them.
	// Until then, we defer the check on the object itself.
	if isObjMatcher && !matcher.ObjectKeyPath.Sorted {
		return ResponseMatcherResults{false, nil, true, nil}
	}

	for _, p := range keys {

		jsonKey := p.RealKey

		// If a key process is provided, utilize that to locate specific nodes
		// If no node is returned, then fallback to regular object iteration
		// for the current key.
		if keyProcessor != nil {
			if keyResult := keyProcessor(p); keyResult != nil {
				node = keyResult
				continue
			}
		}

		switch t := node.(type) {
		case map[string]interface{}:
			if tempNode, ok := t[jsonKey.Name]; ok {
				node = tempNode
			} else {
				node = nil
				break
			}
		case []interface{}:
			if matcher.ObjectKeyPath.Sorted {
				index, err := strconv.ParseInt(jsonKey.Name, 10, 32)
				if err != nil {
					return ResponseMatcherResults{false, results, false, err}
				}
				pathStr += fmt.Sprintf("[%v]", index)
				if int(index) < len(t) {
					node = t[index]
				} else {
					node = nil
				}
			} else {
				// For unsorted arrays, we end up performing a depth first search until we find a node that passes
				// the validation.
				// We will cache the node that was found so that subsequent validations on the same object
				// will actually be performed on the node that matched the previous validation. Otherwise, generic
				// validations may pick out other nodes that are not related to what was expected.
				result := r.depthMatch(t, matcher, pathStr, jsonKey.Name)
				if result.FoundNode.Status && result.FoundNode.MatchedNodeKey {
					node = result.FoundNode.Node
					pathStr = result.FoundNode.NodePath
					// add all parent nodes leading up to the result to our cafmt.che so we can
					// look them up without having to search again.
					for i, chainNode := range result.NodeChain {

						cachepath, _ := matcher.ObjectKeyPath.getObjectPath(len(result.NodeChain) - i)

						r.NodeCache.Cache[cachepath] = NodeCacheObj{
							Node: chainNode.Node,
						}
					}
				} else {
					matcher.Matcher.SetError("Failed locate node")
				}
			}
		}
	}

	status, ds, err := matcher.Matcher.Match(node, r.DS)
	if err != nil {
		return ResponseMatcherResults{false, results, false, err}
	}

	for k := range ds.Store {
		(*r.DS).Put(k, ds.Store[k])
	}

	results = append(results, &FieldMatcherResult{
		ObjectKeyPath:   matcher.ObjectKeyPath.GetDisplayPath(),
		Status:          status,
		Error:           matcher.Matcher.Error(),
		ShowExtendedMsg: matcher.ObjectKeyPath.IsExecutable || len(matcher.Matcher.Error()) >= 64,

		// if we have an object matcher, ignore any successful results since those are basically implied
		// by the presence of having matchers defined on its properties.
		// The only reason an object matcher exists is to add validation for root node existence, and the ability
		// to save the result as a value.
		IgnoreResult: isObjMatcher && status,
	})

	return ResponseMatcherResults{status, results, false, err}
}

type MatcherProcessor func(matcher *FieldMatcherConfig, response interface{}) ResponseMatcherResults

// Match Validates our test pattern against the actual JSON response
func (r *ResponseMatcher) Match(response interface{}) (bool, []*FieldMatcherResult, error) {
	// if we are expecting a payload and get non, throw an error
	if !r.validateEmpty(response) {
		return false, []*FieldMatcherResult{
			{
				ObjectKeyPath: "response",
				Error:         "Expected a non-null response payload.",
				Status:        false,
			},
		}, nil
	}

	return r.MatchBase(response, func(matcher *FieldMatcherConfig, response interface{}) ResponseMatcherResults {
		return r.MatchConfig(matcher, response, nil)
	})
}

func (r *ResponseMatcher) MatchBase(response interface{}, matcherProcessor MatcherProcessor) (bool, []*FieldMatcherResult, error) {
	// make sure we're running everything in the correct object and priority order
	r.SortConfigs()
	var results []*FieldMatcherResult
	aggregatedStatus := true

	for mIndex := 0; mIndex < len(r.Config); mIndex++ {
		matcher := r.Config[mIndex]

		mR := matcherProcessor(matcher, response)
		status := mR.Status
		fieldResults := mR.Results
		deferCheck := mR.DeferCheck
		err := mR.Err

		results = append(results, fieldResults...)
		if err != nil {
			return false, results, err
		}
		if deferCheck {
			matcher.ObjectKeyPath.Sorted = true
			// add this matcher to the end of our validation, we'll process it once we've located the node
			r.Config = append(r.Config, matcher)
			// then remove the matcher from the current position so we don't have a duplicate in our results
			r.Config = append(r.Config[:mIndex], r.Config[mIndex+1:]...)
			mIndex--
			continue
		}

		aggregatedStatus = aggregatedStatus && status
	}

	return aggregatedStatus, results, nil
}
