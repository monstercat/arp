package apivalidator

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	Any      = "$any"
	NotEmpty = "$notEmpty"
	LT       = "$<"
	LTE      = "$<="
	GT       = "$>"
	GTE      = "$>="
	EQ       = "$="

	ValueErrFmt        = "Expected value '%v' did not match the actual value '%v'"
	PatternErrFmt      = "Failed to match actual value '%v' with expected pattern: '%v'"
	NotEmptyErrFmt     = "Expected non-empty value, but got value '%v' instead."
	ArrayLengthErrFmt  = "Expected array with length %v %v but found length %v instead."
	RecievedNullErrFmt = "Received null value when non-null value was expected"
	ExpectedNullErrFmt = "Expected null value when non-null value was returned"
)

type GenericMap map[interface{}]interface{}
type DataStore map[string]interface{}

type FieldMatcher interface {
	GetPriority() int
	Parse(node map[interface{}]interface{}) error
	Match(field interface{}, datastore *DataStore) (bool, error, DataStore)
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

type FieldPathKey struct {
	Key          string
	IsArrayIndex bool
}

type FieldMatcherPath struct {
	Keys           []FieldPathKey
	IsArrayElement bool
	Sorted         bool
}

func (f *FieldMatcherPath) getObjectPath(length int) string {
	pathStr := ""
	for index, k := range f.Keys {
		if index >= length {
			return pathStr
		}

		if k.IsArrayIndex {
			pathStr += fmt.Sprintf("[%v]", k.Key)
		} else {
			pathStr += "." + k.Key
		}
	}

	return pathStr
}

func (f *FieldMatcherPath) GetParentPath() string {
	return f.getObjectPath(len(f.Keys) - 1)
}

func (f *FieldMatcherPath) GetPath() string {
	return f.getObjectPath(len(f.Keys))
}

type FieldMatcherConfig struct {
	Matcher       FieldMatcher
	ObjectKeyPath FieldMatcherPath
}

type FieldMatcherResult struct {
	Status        bool
	ObjectKeyPath string
	Error         string
}

type ResponseMatcher struct {
	DS     *DataStore
	Config []*FieldMatcherConfig
}

func (r *ResponseMatcher) SortConfigs() {
	// Sort configs by key length (parent objects get evaluated first) AND
	// by priority ordering of the matchers within that key length
	configs := r.Config[:]
	sort.Slice(configs, func(i, j int) bool {
		a := configs[i]
		b := configs[j]
		return len(a.ObjectKeyPath.Keys) <= len(b.ObjectKeyPath.Keys) &&
			a.Matcher.GetPriority() <= b.Matcher.GetPriority()
	})
	r.Config = configs
}

func matchPattern(pattern string, field []byte) (bool, error) {
	return regexp.Match(pattern, field)
}

func getExistsFlag(node map[interface{}]interface{}) (bool, error) {
	if v, ok := node["exists"]; ok {
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
	if v, ok := node["storeAs"]; ok {
		return v.(string)
	}
	return ""
}

func getMatcherPriority(node map[interface{}]interface{}) int {
	if v, ok := node["priority"]; ok {
		switch val := v.(type) {
		case int:
			return val
		case float64:
			return int(val)
		case int64:
			return int(val)
		}

	}

	return 0
}

func handleExistence(node interface{}, exists bool, canBeNull bool) (bool, bool, string) {
	if node == nil && exists && !canBeNull {
		return false, false, fmt.Sprintf(RecievedNullErrFmt)
	} else if node == nil && !exists {
		return true, false, ""
	} else if node != nil && !exists {
		return false, false, fmt.Sprintf(ExpectedNullErrFmt)
	}
	// status, passthrough, message
	return false, true, ""
}

func (m *IntegerMatcher) Parse(node map[interface{}]interface{}) error {
	if v, ok := node["matches"]; ok {
		switch val := v.(type) {
		case float64:
			intVal := int64(val)
			m.Value = &intVal
		case int:
			intVal := int64(val)
			m.Value = &intVal
		case string:
			m.Pattern = &val
		}
	}
	m.DSName = getDataStoreName(node)
	m.Priority = getMatcherPriority(node)

	var err error
	m.Exists, err = getExistsFlag(node)
	return err
}

func (m *IntegerMatcher) Match(responseValue interface{}, datastore *DataStore) (bool, error, DataStore) {
	store := DataStore{}
	m.ErrorStr = ""
	if status, passthrough, message := handleExistence(responseValue, m.Exists, false); !passthrough {
		m.ErrorStr = message
		return status, nil, store
	}

	var status bool
	var err error

	typedResponseValue, ok := responseValue.(float64)
	if !ok {
		return false, nil, nil
	}

	if m.Value != nil {
		status = *m.Value == int64(typedResponseValue)
		if !status {
			m.ErrorStr = fmt.Sprintf(ValueErrFmt, *m.Value, typedResponseValue)
		}
	} else if m.Pattern != nil {
		resolved, err := (*datastore).ExpandVariable(*m.Pattern)
		if err != nil {
			return false, fmt.Errorf("Failed to resolve variable within matcher: %v", *m.Pattern), store
		}

		if resolved == Any {
			status = true
		} else {
			status, err = matchPattern(resolved,
				[]byte(strconv.FormatInt(int64(typedResponseValue), 10)))

			if !status {
				m.ErrorStr = fmt.Sprintf(PatternErrFmt, typedResponseValue, *m.Pattern)
			}
		}
	}

	if status {
		m.ErrorStr = fmt.Sprintf("%v", typedResponseValue)
	}

	if status && m.DSName != "" {
		store[m.DSName] = responseValue
	}

	return status, err, store
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

func (m *BoolMatcher) Parse(node map[interface{}]interface{}) error {
	if v, ok := node["matches"]; ok {
		switch val := v.(type) {
		case bool:
			m.Value = &val
		case string:
			m.Pattern = &val
		}
	}
	m.DSName = getDataStoreName(node)
	m.Priority = getMatcherPriority(node)

	var err error
	m.Exists, err = getExistsFlag(node)
	return err
}

func (m *BoolMatcher) Match(responseValue interface{}, datastore *DataStore) (bool, error, DataStore) {
	store := DataStore{}
	m.ErrorStr = ""
	if status, passthrough, message := handleExistence(responseValue, m.Exists, false); !passthrough {
		m.ErrorStr = message
		return status, nil, store
	}

	typedResponseValue, ok := responseValue.(bool)
	if !ok {
		return false, nil, nil
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
			return false, fmt.Errorf("Failed to resolve variable within matcher: %v", *m.Pattern), store
		}

		if resolved == Any {
			status = true
		} else {
			var res bool
			res, err = strconv.ParseBool(resolved)
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
		store[m.DSName] = responseValue
	}
	return status, err, store
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

func (m *StringMatcher) Parse(node map[interface{}]interface{}) error {
	if v, ok := node["matches"]; ok {
		switch val := v.(type) {
		case string:
			m.Value = &val
		}
	}

	m.DSName = getDataStoreName(node)
	m.Priority = getMatcherPriority(node)

	var err error
	m.Exists, err = getExistsFlag(node)
	return err
}

func (m *StringMatcher) Match(responseValue interface{}, datastore *DataStore) (bool, error, DataStore) {
	store := DataStore{}
	if status, passthrough, message := handleExistence(responseValue, m.Exists, false); !passthrough {
		m.ErrorStr = message
		return status, nil, store
	}

	typedResponseValue, ok := responseValue.(string)
	if !ok {
		return false, nil, nil
	}

	var status bool
	var err error

	if m.Value != nil {
		resolved, err := (*datastore).ExpandVariable(*m.Value)
		if err != nil {
			return false, fmt.Errorf("Failed to resolve variable within matcher: %v", *m.Value), store
		}

		switch resolved {
		case Any:
			status = true
		case NotEmpty:
			status = typedResponseValue != ""
			if !status {
				m.ErrorStr = fmt.Sprintf(NotEmptyErrFmt, typedResponseValue)
			}
		default:
			status, err = matchPattern(resolved, []byte(typedResponseValue))
			if !status {
				m.ErrorStr = fmt.Sprintf(PatternErrFmt, typedResponseValue, resolved)
			}
		}
	}

	if status {
		m.ErrorStr = typedResponseValue
	}
	if status && m.DSName != "" {
		store[m.DSName] = responseValue
	}
	return status, err, store
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

func (m *ArrayMatcher) Parse(node map[interface{}]interface{}) error {
	var err error
	m.Exists, err = getExistsFlag(node)
	if err != nil {
		return err
	}

	if v, ok := node["length"]; ok {
		switch val := v.(type) {
		case int:
			intVal := int64(val)
			m.Length = &intVal
		case float64:
			intVal := int64(val)
			m.Length = &intVal
		case string:
			m.LengthStr = &val
		}
	}

	if v, ok := node["items"]; ok && m.Exists {
		m.Items = v.([]interface{})
	}

	if v, ok := node["sorted"]; ok {
		m.Sorted = v.(bool)
	} else {
		m.Sorted = true
	}

	m.Priority = getMatcherPriority(node)
	m.DSName = getDataStoreName(node)
	return nil
}

func (m *ArrayMatcher) Match(responseValue interface{}, datastore *DataStore) (bool, error, DataStore) {
	store := DataStore{}
	if status, passthrough, message := handleExistence(responseValue, m.Exists, true); !passthrough {
		m.ErrorStr = message
		return status, nil, store
	}

	var typedResponseValue []interface{}
	if responseValue == nil {
		// if nil, we can still validate the length in case a non-0 value was expected
		typedResponseValue = []interface{}{}
	} else {
		var ok bool
		typedResponseValue, ok = responseValue.([]interface{})
		if !ok {
			return false, nil, nil
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
			return false, fmt.Errorf("Failed to resolve variable within matcher: %v", *m.LengthStr), store
		}
		s := resolved

		switch s {
		case NotEmpty:
			status = responseLength > 0
		case Any:
			status = true
		default:
			// order from longest string to shortest
			for _, op := range []string{GTE, LTE, GT, LT} {
				if strings.HasPrefix(s, op) {
					var length int64
					length, err = strconv.ParseInt(strings.TrimSpace(strings.ReplaceAll(s, op, "")), 10, 32)
					if err != nil {
						return false, err, store
					}
					switch op {
					case LT:
						status = responseLength < length
					case LTE:
						status = responseLength <= length
					case GT:
						status = responseLength > length
					case GTE:
						status = responseLength >= length
					}

					if !status {
						sign := strings.ReplaceAll(op, "$", "")
						m.ErrorStr = fmt.Sprintf(ArrayLengthErrFmt, sign, length, responseLength)
					}
				}
			}
		}
	}
	if status {
		m.ErrorStr = fmt.Sprintf("[length] %v", responseLength)
	}

	if status && m.DSName != "" {
		store[m.DSName] = responseValue
	}
	return status, err, store
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

func (r *ResponseMatcher) loadField(fieldNode map[interface{}]interface{}, paths FieldMatcherPath) error {
	typeField, ok := fieldNode["type"]
	if !ok {
		return fmt.Errorf("Failed to parse response validation. No type field provided: %v", paths.Keys)
	}

	typeStr, ok := typeField.(string)
	if !ok {
		return fmt.Errorf("Failed to parse response validation. Type must be a string: %v", paths.Keys)
	}

	var foundMatcher FieldMatcher
	switch typeStr {
	case "integer":
		intMatcher := &IntegerMatcher{}
		if err := intMatcher.Parse(fieldNode); err != nil {
			return err
		}
		foundMatcher = intMatcher
	case "string":
		strMatcher := &StringMatcher{}
		if err := strMatcher.Parse(fieldNode); err != nil {
			return err
		}
		foundMatcher = strMatcher
	case "bool":
		boolMatcher := &BoolMatcher{}
		if err := boolMatcher.Parse(fieldNode); err != nil {
			return err
		}
		foundMatcher = boolMatcher
	case "array":
		arrayMatcher := &ArrayMatcher{}
		if err := arrayMatcher.Parse(fieldNode); err != nil {
			return err
		}
		foundMatcher = arrayMatcher
	case "object":
		subObjectNode := fieldNode["properties"].(map[interface{}]interface{})
		if err := r.loadObjectFields(subObjectNode, paths); err != nil {
			return err
		}
	}

	if foundMatcher != nil {
		config := &FieldMatcherConfig{
			Matcher:       foundMatcher,
			ObjectKeyPath: paths,
		}
		r.Config = append(r.Config, config)

		// visit array elements AFTER we have added the array to the config
		switch val := foundMatcher.(type) {
		case *ArrayMatcher:
			if err := r.loadArrayFields(val, val.Items, paths); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *ResponseMatcher) loadArrayFields(m *ArrayMatcher, fields []interface{}, paths FieldMatcherPath) error {
	for i, arrayNode := range fields {
		fieldNode := arrayNode.(map[interface{}]interface{})
		var pathStack []FieldPathKey
		pathStack = append(pathStack, paths.Keys...)
		pathStack = append(pathStack, FieldPathKey{
			Key:          fmt.Sprintf("%v", i),
			IsArrayIndex: true,
		})

		newPaths := FieldMatcherPath{
			Keys:           pathStack,
			IsArrayElement: true,
			Sorted:         m.Sorted,
		}

		if err := r.loadField(fieldNode, newPaths); err != nil {
			return err
		}
	}
	return nil
}

func (r *ResponseMatcher) loadObjectFields(fields map[interface{}]interface{}, paths FieldMatcherPath) error {
	for k := range fields {
		fieldNode := fields[k].(map[interface{}]interface{})

		var pathStack []FieldPathKey
		pathStack = append(pathStack, paths.Keys...)
		pathStack = append(pathStack, FieldPathKey{
			Key:          k.(string),
			IsArrayIndex: false,
		})

		newPaths := FieldMatcherPath{
			Keys:           pathStack,
			IsArrayElement: paths.IsArrayElement,
			Sorted:         paths.Sorted,
		}
		if err := r.loadField(fieldNode, newPaths); err != nil {
			return err
		}
	}
	return nil
}

type DepthMatchResponse struct {
	Status         bool
	Node           interface{}
	NodePath       string
	MatchedNodeKey bool
	ParentNode     interface{}
}

func (r *ResponseMatcher) depthMatch(node interface{}, matcher *FieldMatcherConfig, path string, key string) DepthMatchResponse {
	status, _, _ := matcher.Matcher.Match(node, r.DS)
	if status {
		return DepthMatchResponse{
			Status:         status,
			Node:           node,
			NodePath:       path,
			MatchedNodeKey: false,
			ParentNode:     nil,
		}
	}

	switch n := node.(type) {
	case map[string]interface{}:
		for k := range n {
			result := r.depthMatch(n[k], matcher, path+"."+k, key)
			if result.Status {
				// make sure our validation succeeded against the object key we were looking for
				bubbleParent := result.ParentNode
				if bubbleParent == nil {
					bubbleParent = n
				}

				if !result.MatchedNodeKey && k == key {
					return DepthMatchResponse{
						Status:         result.Status,
						Node:           result.Node,
						ParentNode:     bubbleParent,
						MatchedNodeKey: true,
						NodePath:       result.NodePath,
					}
				}
			}
		}
	case []interface{}:
		for index, i := range n {
			result := r.depthMatch(i, matcher, path+fmt.Sprintf("[%v]", index), key)
			if result.Status {
				bubbleParent := result.ParentNode
				if bubbleParent == nil {
					bubbleParent = n
				}

				return DepthMatchResponse{
					Status:         result.Status,
					Node:           result.Node,
					ParentNode:     bubbleParent,
					MatchedNodeKey: result.MatchedNodeKey,
					NodePath:       result.NodePath,
				}
			}
		}
	}

	return DepthMatchResponse{
		Status:         false,
		Node:           nil,
		NodePath:       "",
		MatchedNodeKey: false,
		ParentNode:     nil,
	}
}

type NodeCacheObj struct {
	Node      interface{}
	PathIndex int
}

// Match Validates our test pattern against the actual JSON response
func (r *ResponseMatcher) Match(response map[string]interface{}) (bool, error, []*FieldMatcherResult) {
	// if we are expecting a payload and get non, throw an error
	if len(r.Config) > 0 && len(response) == 0 {
		return false, nil, []*FieldMatcherResult{
			{
				ObjectKeyPath: "response",
				Error:         "Expected a non-null response payload.",
				Status:        false,
			},
		}
	}

	// make sure we're running everything in the correct object and priority order
	r.SortConfigs()

	var results []*FieldMatcherResult
	aggregatedStatus := true
	sharedNodes := make(map[string]NodeCacheObj)

	for _, matcher := range r.Config {
		var node interface{}
		nodeParentKey := matcher.ObjectKeyPath.GetParentPath()
		node = response
		pathStr := ""

		keys := matcher.ObjectKeyPath.Keys

		if cachedNode, ok := sharedNodes[nodeParentKey]; ok {
			node = cachedNode.Node
			keys = keys[cachedNode.PathIndex:]
		}

		for pathIndex, p := range keys {
			switch t := node.(type) {
			case map[string]interface{}:
				if tempNode, ok := t[p.Key]; ok {
					node = tempNode
				} else {
					node = nil
					break
				}
			case []interface{}:
				if matcher.ObjectKeyPath.Sorted {
					index, err := strconv.ParseInt(p.Key, 10, 32)
					if err != nil {
						return false, err, results
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
					result := r.depthMatch(t, matcher, pathStr, p.Key)
					if result.Status && result.MatchedNodeKey {
						node = result.Node
						pathStr = result.NodePath
						sharedNodes[nodeParentKey] = NodeCacheObj{
							Node:      result.ParentNode,
							PathIndex: pathIndex,
						}
					} else {
						matcher.Matcher.SetError(fmt.Sprintf("Failed locate node"))
					}
				}

			}

		}

		status, err, ds := matcher.Matcher.Match(node, r.DS)
		if err != nil {
			return false, err, results
		}

		for k := range ds {
			(*r.DS)[k] = ds[k]
		}

		if node == nil && matcher.ObjectKeyPath.IsArrayElement {
			pathStr += "[x]"
		}

		results = append(results, &FieldMatcherResult{
			ObjectKeyPath: matcher.ObjectKeyPath.GetPath(),
			Status:        status,
			Error:         matcher.Matcher.Error(),
		})

		aggregatedStatus = aggregatedStatus && status
	}

	return aggregatedStatus, nil, results
}
