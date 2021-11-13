package arp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

const (
	MissingDSKeyFmt   = "Attempted to retrieve data from data store that does not exist: key: %v"
	BadIndexDSFmt     = "Attempted to index into a data store value with a non-positive or non-integer value: %v"
	IndexExceedsDSFmt = "Index for data store value exceeds its max length: %v"
	StatusCodePath    = "response.StatusCode"
	HeadersPath       = "response.Header"
)

type TestSuiteCfg struct {
	Tests []TestCaseCfg `yaml:"tests"`
}

type TestSuite struct {
	Tests           []*TestCase
	GlobalDataStore DataStore
	Verbose         bool
}

type SuiteResult struct {
	Results  []*TestResult
	Passed   int
	Failed   int
	Total    int
	Duration time.Duration
}

func NewTestSuite(testFile string, fixtures string) (*TestSuite, error) {
	suite := &TestSuite{
		GlobalDataStore: DataStore{},
	}

	err := suite.InitializeDataStore(fixtures)
	if err != nil {
		return suite, err
	}

	status, err := suite.LoadTests(testFile)

	if !status && err == nil {
		return nil, nil
	} else if err != nil {
		return suite, fmt.Errorf("failed to initialize test suite: %v", err)
	}

	return suite, nil
}

func (t *TestSuite) ReloadFile(testFile string) (bool, error) {
	t.Tests = make([]*TestCase, 0)
	return t.LoadTests(testFile)
}

func (t *TestSuite) InitializeDataStore(fixtures string) error {
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

func (t *TestSuite) LoadFixtures(fixtures string) (map[string]interface{}, error) {
	var config map[interface{}]interface{}

	if fixtures == "" {
		return nil, nil
	}

	fileInfo, err := os.Stat(fixtures)
	if err != nil {
		return nil, fmt.Errorf("failed to stat fixture file: %v - %v", fixtures, err)
	}

	if fileInfo.IsDir() {
		return nil, fmt.Errorf("fixtures must be a file, not a directory: %v - %v", fixtures, err)
	}

	data, err := os.ReadFile(fixtures)
	if err != nil {
		return nil, fmt.Errorf("failed to read fixtures file: %v - %v", fixtures, err)
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal fixture file: %v - %v", fixtures, err)
	}

	return YamlToJson(config).(map[string]interface{}), nil
}

func (t *TestSuite) Close() {
	for _, test := range t.Tests {
		test.CloseWebsocket()
	}
}

func (t *TestSuite) LoadTests(testFile string) (bool, error) {
	data, err := os.ReadFile(testFile)
	if err != nil {
		return false, fmt.Errorf("failed to load test file: %v - %v", testFile, err)
	}
	t.GlobalDataStore["TEST_DIR"], _ = filepath.Abs(filepath.Dir(testFile))

	var testSuiteCfg TestSuiteCfg

	err = yaml.Unmarshal(data, &testSuiteCfg)
	if err != nil {
		return false, fmt.Errorf("failed to load test file: %v - %v", testFile, err)
	}

	for _, test := range testSuiteCfg.Tests {
		tCase := TestCase{
			GlobalDataStore: &t.GlobalDataStore,
		}

		err = tCase.LoadConfig(&test)
		if err != nil {
			return false, fmt.Errorf("failed to load test file: %v - %v", testFile, err)
		}

		t.Tests = append(t.Tests, &tCase)
	}

	return true, nil
}

func (t *TestSuite) ExecuteTests(testTags []string) (bool, SuiteResult, error) {
	defer t.Close()

	anyFailed := false

	suiteResults := SuiteResult{
		Results: []*TestResult{},
		Total:   len(t.Tests),
	}

	for _, test := range t.Tests {
		if test.Config.ExitOnRun {
			break
		}

		if t.Verbose {
			fmt.Printf(">> In Progress: %v\n", test.Config.Name)
		}

		passed, results, err := test.Execute(testTags)
		if err != nil {
			fmt.Printf("<< Done: [Fail] %v\n", test.Config.Name)
			return false, suiteResults, err
		}

		if passed {
			suiteResults.Passed += 1
		} else {
			anyFailed = true
			suiteResults.Failed += 1
		}

		if t.Verbose {
			statusStr := "Pass"
			if !passed {
				statusStr = "Fail"
			}
			fmt.Printf("<< Done: [%v] %v\n", statusStr, test.Config.Name)
		}

		suiteResults.Duration += results.EndTime.Sub(results.StartTime)
		suiteResults.Results = append(suiteResults.Results, results)
	}

	return !anyFailed, suiteResults, nil
}
