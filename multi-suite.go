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

type MultiSuiteWorker struct {
	TestTags []string
	Suite    *TestSuite
	TestFile string
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
	startTime := time.Now()

	if t.Verbose {
		fmt.Printf("Executing tests across %v threads...\n\n", threads)
	}

	var results []MultiSuiteResult
	aggregateStatus := true

	wg := sync.WaitGroup{}
	testCount := len(t.Suites)
	workerResults := make(chan MultiSuiteResult, threads)
	workerMessages := make(chan MultiSuiteWorker, testCount)

	wg.Add(threads)
	for i := 0; i < threads; i++ {
		go func() {
			for {
				m, ok := <-workerMessages
				if !ok {
					wg.Done()
					return
				}
				if t.Verbose {
					fmt.Printf("> In Progress: %v\n", m.TestFile)
				}
				status, result, err := m.Suite.ExecuteTests(m.TestTags)
				r := MultiSuiteResult{
					Passed:      status,
					Error:       err,
					TestFile:    m.TestFile,
					TestResults: result,
				}

				workerResults <- r
			}
		}()
	}

	for k := range t.Suites {
		msg := MultiSuiteWorker{
			TestTags: testTags,
			Suite:    t.Suites[k],
			TestFile: k,
		}
		workerMessages <- msg
	}
	close(workerMessages)
	defer close(workerResults)

	for i := 0; i < testCount; i++ {
		d := <-workerResults
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
	wg.Wait()
	duration := time.Since(startTime)
	return aggregateStatus, results, duration, nil
}
