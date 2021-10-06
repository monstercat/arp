package apivalidator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"gopkg.in/yaml.v2"
)

type TestCase struct {
	ExitOnRun       bool
	Skip            bool
	Name            string
	Description     string
	Method          string
	Route           string
	StatusCode      int
	Input           map[interface{}]interface{}
	Headers         map[interface{}]interface{}
	ResponseMatcher ResponseMatcher
	GlobalDataStore *DataStore
}

type TestHook func(test *TestCase)
type BeforeEachTest func(test *TestCase, request *http.Request)
type AfterEachTest func(test *TestCase, response *http.Response, status bool, results []*FieldMatcherResult)

type TestSuite struct {
	Tests           []TestCase
	GlobalDataStore DataStore
	BeforeEachTest  BeforeEachTest
	AfterEachTest   AfterEachTest
	Client          http.Client
}

type TestResult struct {
	TestCase      TestCase
	Fields        []*FieldMatcherResult
	Passed        bool
	Response      map[string]interface{}
	ResolvedRoute string
	StatusCode    int
}

type SuiteResult struct {
	Results []*TestResult
	Passed  int
	Failed  int
	Total   int
}

func YamlToJson(i interface{}) interface{} {
	switch x := i.(type) {
	case map[interface{}]interface{}:
		m2 := map[string]interface{}{}
		for k, v := range x {
			m2[k.(string)] = YamlToJson(v)
		}
		return m2
	case []interface{}:
		for i, v := range x {
			x[i] = YamlToJson(v)
		}
	}
	return i
}

func (t *DataStore) resolveVariable(variable string) (string, error) {
	keys := strings.Split(variable, ".")
	var node interface{}
	node = *t
	for _, k := range keys {
		switch v := node.(type) {
		case DataStore:
			if nextNode, ok := v[k]; !ok {
				return "", fmt.Errorf("Test will fail. Attempted to retrieve data from global store that does not exist: key: %v", variable)
			} else {
				node = nextNode
			}
		case map[string]interface{}:
			if nextNode, ok := v[k]; !ok {
				return "", fmt.Errorf("Test will fail. Attempted to retrieve data from global store that does not exist: key: %v", variable)
			} else {
				node = nextNode
			}
		}
	}

	return fmt.Sprintf("%v", node), nil
}

type VarStackFrame struct {
	StartPos int
	EndPos   int
	VarName  string
	Nested   int
}

type VarStack struct {
	Frames []VarStackFrame
}

func (s *VarStack) Push(f VarStackFrame) {
	s.Frames = append(s.Frames, f)
}

func (s *VarStack) Pop() *VarStackFrame {
	if len(s.Frames) == 0 {
		return nil
	}
	result := s.Frames[len(s.Frames)-1]
	s.Frames = s.Frames[:len(s.Frames)-1]
	return &result
}

func (f *VarStackFrame) IsValid() bool {
	return f.StartPos != f.EndPos && f.VarName != ""
}

func parseVar(input string) VarStack {
	varStack := VarStack{}
	resultStack := VarStack{}
	var curStackFrame *VarStackFrame
	for i := 0; i < len(input); i++ {
		char := input[i]
		if char == '@' && i+1 < len(input) && input[i+1] == '{' {
			nestLevel := 0
			if curStackFrame != nil {
				varStack.Push(*curStackFrame)
				nestLevel = curStackFrame.Nested + 1
			}
			curStackFrame = &VarStackFrame{}
			curStackFrame.StartPos = i
			curStackFrame.Nested = nestLevel
		} else if curStackFrame != nil && char == '}' {
			curStackFrame.EndPos = i
			curStackFrame.VarName = input[curStackFrame.StartPos : curStackFrame.EndPos+1]
			resultStack.Push(*curStackFrame)
			curStackFrame = varStack.Pop()
		}
	}

	return resultStack
}

func (t *DataStore) ExpandVariable(input string) (string, error) {
	outputString := input
	variables := parseVar(input)

	type ExtendedStackFrame struct {
		VarStackFrame
		ResolvedVarName string
	}

	toResolve := []ExtendedStackFrame{}
	for _, v := range variables.Frames {
		toResolve = append(toResolve, ExtendedStackFrame{
			VarStackFrame:   v,
			ResolvedVarName: v.VarName,
		})
	}

	for i, v := range toResolve {
		varKey := strings.ReplaceAll(strings.ReplaceAll(v.ResolvedVarName, "@{", ""), "}", "")
		resolvedVar, err := t.resolveVariable(varKey)
		if err != nil {
			return "", err
		}

		if v.Nested == 0 {
			outputString = strings.ReplaceAll(outputString, v.VarName, resolvedVar)

		}

		// once variable is resolved, we want to replace it in all the other variables (if they reference it)
		for offset := i + 1; offset < len(toResolve); offset++ {
			frame := toResolve[offset]
			frame.ResolvedVarName = strings.ReplaceAll(frame.VarName, v.VarName, resolvedVar)
			toResolve[offset] = frame
		}
	}

	return outputString, nil
}

func (t *DataStore) resolveDataStoreVarRecursive(input interface{}) (interface{}, error) {
	if input == nil {
		return nil, nil
	}

	switch n := input.(type) {
	case map[interface{}]interface{}:
		for k := range n {
			if node, err := t.resolveDataStoreVarRecursive(n[k]); err != nil {
				return nil, err
			} else {
				n[k] = node
			}

		}
		return n, nil
	case map[string]interface{}:
		for k := range n {
			if node, err := t.resolveDataStoreVarRecursive(n[k]); err != nil {
				return nil, err
			} else {
				n[k] = node
			}

		}
		return n, nil
	case []interface{}:
		for i, e := range n {
			if node, err := t.resolveDataStoreVarRecursive(e); err != nil {
				return nil, err
			} else {
				n[i] = node
			}
		}

		return n, nil
	case string:
		return t.ExpandVariable(n)
	}

	return input, nil
}

func (t *TestCase) LoadConfig(json map[interface{}]interface{}) error {
	t.ResponseMatcher.DS = t.GlobalDataStore
	if name, ok := json["name"]; ok {
		t.Name = name.(string)
	}

	if desc, ok := json["description"]; ok {
		t.Description = desc.(string)
	}

	if method, ok := json["method"]; ok {
		t.Method = method.(string)
	}

	if route, ok := json["route"]; ok {
		t.Route = route.(string)
	}

	if input, ok := json["input"]; ok {
		t.Input = input.(map[interface{}]interface{})
	}

	if headers, ok := json["headers"]; ok {
		t.Headers = headers.(map[interface{}]interface{})
	}

	if skip, ok := json["skip"]; ok {
		t.Skip = skip.(bool)
	}

	if exit, ok := json["exit"]; ok {
		t.ExitOnRun = exit.(bool)
	}

	responseConfig, ok := json["response"]
	if !ok {
		return nil
	}

	responseJson := responseConfig.(map[interface{}]interface{})
	if statusCode, sOk := responseJson["code"]; sOk {
		switch c := statusCode.(type) {
		case float64:
			t.StatusCode = int(c)
		case int:
			t.StatusCode = c
		}
	} else {
		t.StatusCode = 200
	}

	if payload, ok := responseJson["payload"]; ok {

		return t.ResponseMatcher.loadObjectFields(payload.(map[interface{}]interface{}), FieldMatcherPath{})
	}

	return nil
}

func (t *TestCase) GetTestRoute() (string, error) {
	resolvedRoute, err := t.GlobalDataStore.ExpandVariable(t.Route)
	if err != nil {
		return "", err
	}
	return resolvedRoute, nil
}

func (t *TestCase) GetTestInput() (io.Reader, error) {
	node, err := t.GlobalDataStore.resolveDataStoreVarRecursive(t.Input)
	if err != nil {
		return nil, err
	}

	jsonNode := YamlToJson(node)

	b, err := json.Marshal(jsonNode)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}

func (t *TestCase) GetTestHeaders() (interface{}, error) {
	node, err := t.GlobalDataStore.resolveDataStoreVarRecursive(t.Headers)
	if err != nil {
		return nil, err
	}

	return node, nil
}

func (t *TestCase) Validate(statusCode int, response map[string]interface{}) (bool, error, []*FieldMatcherResult) {
	statusCodeResult := &FieldMatcherResult{
		ObjectKeyPath: "response.StatusCode",
		Status:        true,
	}
	if t.StatusCode != statusCode {
		statusCodeResult.Status = false
		statusCodeResult.Error = fmt.Sprintf("Expected response with status code '%v'. Got '%v' instead.",
			t.StatusCode, statusCode)
	} else {
		statusCodeResult.Error = fmt.Sprintf("%v", t.StatusCode)
	}

	status, err, results := t.ResponseMatcher.Match(response)
	if err != nil {
		return false, err, results
	}

	if status && statusCodeResult.Status {
		for k := range *t.ResponseMatcher.DS {
			(*t.GlobalDataStore)[k] = (*t.ResponseMatcher.DS)[k]
		}
	}

	// order the results so the status code is always first
	var newResults = []*FieldMatcherResult{}
	newResults = append(newResults, statusCodeResult)
	newResults = append(newResults, results...)

	return status && statusCodeResult.Status, err, newResults
}

func NewTestSuite(defaultHost string, testFile string, fixtures string) (*TestSuite, error) {
	suite := &TestSuite{
		GlobalDataStore: DataStore{},
		Client:          http.Client{},
	}

	err := suite.InitializeDataStore(defaultHost, fixtures)
	if err != nil {
		return suite, err
	}

	status, err := suite.LoadTests(testFile)

	if !status && err == nil {
		return nil, nil
	} else if err != nil {
		return suite, fmt.Errorf("Failed to initialize test suite: %v", err)
	}

	return suite, nil
}

func (t *TestSuite) LoadFixtures(fixtures string) (map[string]interface{}, error) {
	var config map[interface{}]interface{}

	if fixtures == "" {
		return nil, nil
	}

	fileInfo, err := os.Stat(fixtures)
	if err != nil {
		return nil, fmt.Errorf("Failed to stat fixture file: %v - %v", fixtures, err)
	}

	if fileInfo.IsDir() {
		return nil, fmt.Errorf("Fixtures must be a file, not a directory: %v - %v", fixtures, err)
	}

	data, err := os.ReadFile(fixtures)
	if err != nil {
		return nil, fmt.Errorf("Failed to read fixtures file: %v - %v", fixtures, err)
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("Failed to unmarshal fixture file: %v - %v", fixtures, err)
	}

	return YamlToJson(config).(map[string]interface{}), nil
}

func (t *TestSuite) InitializeDataStore(defaultHost string, fixtures string) error {
	t.GlobalDataStore["host"] = defaultHost

	f, err := t.LoadFixtures(fixtures)
	if err != nil {
		return err
	}

	for k := range f {
		t.GlobalDataStore[k] = f[k]
	}

	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		t.GlobalDataStore[pair[0]] = pair[1]
	}

	return nil
}

func (t *TestSuite) LoadTests(testFile string) (bool, error) {
	data, err := os.ReadFile(testFile)
	if err != nil {
		return false, fmt.Errorf("Failed to load test file: %v - %v", testFile, err)
	}

	var fileData interface{}
	err = yaml.Unmarshal(data, &fileData)

	if err != nil {
		return false, fmt.Errorf("Failed to load test file: %v - %v", testFile, err)
	}

	mappedData, ok := fileData.(map[interface{}]interface{})
	if !ok {
		return false, fmt.Errorf("Incorrectly formatted test config. Top level needs to be an object.")
	}

	testList, ok := mappedData["tests"]
	if !ok {
		return false, nil
	}

	if tests, ok := testList.([]interface{}); ok {
		for _, test := range tests {
			tCase := TestCase{
				GlobalDataStore: &t.GlobalDataStore,
			}

			err = tCase.LoadConfig(test.(map[interface{}]interface{}))
			if err != nil {
				return false, fmt.Errorf("Failed to load test file: %v - %v", testFile, err)
			}

			t.Tests = append(t.Tests, tCase)
		}
	}

	return true, nil
}

func (t *TestSuite) ExecuteTest(test *TestCase) (bool, error, *TestResult) {
	var request *http.Request
	var response *http.Response
	var err error
	results := &TestResult{
		TestCase: *test,
	}

	if test.Skip {
		results.Fields = []*FieldMatcherResult{
			{
				Error:         "Skipping test as configured",
				ObjectKeyPath: "test.skip",
				Status:        true,
			},
		}
		results.Passed = true
		return true, nil, results
	}

	route, err := test.GetTestRoute()
	if err != nil {
		return false, fmt.Errorf("Failed to determine test route: %v", err), results
	}
	results.ResolvedRoute = route

	input, err := test.GetTestInput()
	if err != nil {
		return false, fmt.Errorf("Failed to get test input: %v", err), results
	}

	switch test.Method {
	case "GET":
		request, err = http.NewRequest("GET", route, input)
	case "POST":
		request, err = http.NewRequest("POST", route, input)
	}

	headers, err := test.GetTestHeaders()
	if err != nil {
		return false, fmt.Errorf("Failed to resolve test headers parameter: %v", err), results
	}
	headersMap := headers.(map[interface{}]interface{})
	for k := range headersMap {
		key := fmt.Sprintf("%v", k)
		val := headersMap[k].(string)
		request.Header.Set(key, val)
	}

	if t.BeforeEachTest != nil {
		t.BeforeEachTest(test, request)
	}

	response, err = t.Client.Do(request)
	if err != nil {
		return false, fmt.Errorf("Failed to fetch API response: %v", err), results
	}

	results.StatusCode = response.StatusCode

	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return false, fmt.Errorf("Failed to fetch API response: %v", err), results
	}

	var responseJson map[string]interface{}
	if len(responseData) > 0 {

		err = json.Unmarshal(responseData, &responseJson)
		if err != nil {
			return false, fmt.Errorf("Failed to unmarshal API response: %v\n%v", err, responseData), results
		}

	}
	results.Response = responseJson
	results.Passed, err, results.Fields = test.Validate(response.StatusCode, responseJson)
	if t.AfterEachTest != nil {
		t.AfterEachTest(test, response, results.Passed, results.Fields)
	}

	return results.Passed, err, results
}

func (t *TestSuite) ExecuteTests() (bool, error, SuiteResult) {
	anyFailed := false

	suiteResults := SuiteResult{
		Results: []*TestResult{},
		Total:   len(t.Tests),
	}

	for _, test := range t.Tests {
		if test.ExitOnRun {
			break
		}
		passed, err, results := t.ExecuteTest(&test)
		if err != nil {
			return false, err, suiteResults
		}

		if passed {
			suiteResults.Passed += 1
		} else {
			anyFailed = true
			suiteResults.Failed += 1
		}

		suiteResults.Results = append(suiteResults.Results, results)
	}

	return !anyFailed, nil, suiteResults
}
