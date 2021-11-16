package arp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type MultiTestSuite struct {
	Suites  map[string]*TestSuite
	Verbose bool
}

type MultiSuiteResult struct {
	Passed      bool
	Error       error
	TestResults SuiteResult
	TestFile    string
}

func NewMultiSuiteTest(testDir string, fixtures string) (*MultiTestSuite, error) {
	multiSuite := &MultiTestSuite{
		Suites:  map[string]*TestSuite{},
		Verbose: true,
	}
	err := multiSuite.LoadTests(testDir, fixtures)
	return multiSuite, err
}

func (t *MultiTestSuite) LoadTests(testDir string, fixtures string) error {
	err := filepath.Walk(testDir, func(path string, info os.FileInfo, err error) error {
		if strings.HasSuffix(path, ".yaml") {
			suite, err := NewTestSuite(path, fixtures)
			if err != nil {
				return err
			}
			if len(suite.Tests) == 0 {
				return nil
			}

			if suite != nil {
				t.Suites[path] = suite
			}
			return nil
		}

		return nil
	})

	return err
}

func (t *MultiTestSuite) ExecuteTests(threads int, testTags []string) (bool, []MultiSuiteResult, time.Duration, error) {
	if t.Verbose {
		fmt.Printf("Executing tests across %v threads...\n\n", threads)
	}
	startTime := time.Now()

	var results []MultiSuiteResult
	aggregateStatus := true

	testCount := len(t.Suites)
	channels := make(chan MultiSuiteResult, threads)
	wg := sync.WaitGroup{}

	for k := range t.Suites {
		suite := t.Suites[k]
		wg.Add(1)
		go func(file string) {
			defer wg.Done()

			if t.Verbose {
				fmt.Printf("> In Progress: %v\n", file)
			}
			status, result, err := suite.ExecuteTests(testTags)
			r := MultiSuiteResult{
				Passed:      status,
				Error:       err,
				TestFile:    file,
				TestResults: result,
			}

			channels <- r
		}(k)
	}

	//	for d := range channels {
	for i := 0; i < testCount; i++ {
		d := <-channels
		results = append(results, d)
		aggregateStatus = aggregateStatus && d.Passed

		if t.Verbose {
			statusStr := "Pass"
			if !d.Passed {
				statusStr = "Fail"
			}

			fmt.Printf("< Done: [%v] %v\n", statusStr, d.TestFile)
		}
	}
	duration := time.Since(startTime)
	return aggregateStatus, results, duration, nil
}
