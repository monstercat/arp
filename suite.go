package arp

import (
	"fmt"
	"io"
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
	File            string
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
		GlobalDataStore: NewDataStore(),
		File:            testFile,
	}

	err := suite.InitializeDataStore(fixtures)
	if err != nil {
		return suite, err
	}

	status, err := suite.LoadTests(fixtures)
	if !status && err == nil {
		return nil, nil
	} else if err != nil {
		return suite, fmt.Errorf("failed to initialize test suite: %v", err)
	}

	return suite, nil
}

func (t *TestSuite) ReloadFile(fixtures string) (bool, error) {
	t.Tests = make([]*TestCase, 0)
	return t.LoadTests(fixtures)
}

func (t *TestSuite) InitializeDataStore(fixtures string) error {
	f, err := t.LoadFixtures(fixtures)
	if err != nil {
		return err
	}

	for k := range f {
		t.GlobalDataStore.Put(k, f[k])
	}

	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		t.GlobalDataStore.Put(pair[0], pair[1])
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

func (t *TestSuite) LoadTests(fixtures string) (bool, error) {
	var readers []io.Reader

	if fixtures != "" {
		fix, err := os.Open(fixtures)
		if err != nil {
			return false, fmt.Errorf("failed to open fixture file: %v - %v", fixtures, err)
		}

		readers = append(readers, fix)
	}

	var tests *os.File
	var err error
	if t.File == "-" {
		tests = os.Stdin
	} else {
		tests, err = os.Open(t.File)
	}
	if err != nil {
		return false, fmt.Errorf("failed to open test file: %v - %v", t.File, err)
	}
	readers = append(readers, tests)

	// combine fixtures and test file into a single source so tests can utilize yaml anchors defined in
	// the fixtures file
	multiReader := io.MultiReader(readers...)

	data, err := io.ReadAll(multiReader)
	if err != nil {
		return false, fmt.Errorf("failed to load test file: %v - %v", t.File, err)
	}
	fp, _ := filepath.Abs(filepath.Dir(t.File))
	t.GlobalDataStore.Put("TEST_DIR", fp)

	var testSuiteCfg TestSuiteCfg

	err = yaml.Unmarshal(data, &testSuiteCfg)
	if err != nil {
		return false, fmt.Errorf("failed to load test file: %v - %v", t.File, err)
	}

	for _, test := range testSuiteCfg.Tests {
		tCase := TestCase{
			GlobalDataStore: &t.GlobalDataStore,
		}

		err = tCase.LoadConfig(&test)
		if err != nil {
			return false, fmt.Errorf("failed to load test file: %v - %v", t.File, err)
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

	for testIndex, test := range t.Tests {
		if test.Config.ExitOnRun {
			break
		}

		if t.Verbose {
			fmt.Printf(">> In Progress: %v\n", test.Config.Name)
		}

		passed, results, err := test.Execute(testTags)
		if err != nil {
			fmt.Printf("<< Done: [Fail] %v -> %v\n", t.File, test.Config.Name)
			suiteResults.Failed += len(t.Tests) - testIndex
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
			fmt.Printf("<< Done: [%v] %v -> %v\n", statusStr, t.File, test.Config.Name)
		}

		suiteResults.Duration += results.EndTime.Sub(results.StartTime)
		suiteResults.Results = append(suiteResults.Results, results)
	}

	return !anyFailed, suiteResults, nil
}
