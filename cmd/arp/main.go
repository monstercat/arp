package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	. "github.com/monstercat/arp"
)

type varFlags []string

func (v *varFlags) String() string {
	return strings.Join(*v, ",")
}

func (v *varFlags) Set(value string) error {
	*v = append(*v, strings.TrimSpace(value))
	return nil
}

type testTags []string

func (t *testTags) String() string {
	return strings.Join(*t, ",")
}

func (t *testTags) Set(value string) error {
	*t = append(*t, strings.TrimSpace((value)))
	return nil
}

type ProgramArgs struct {
	Fixtures     *string
	TestRoot     *string
	TestFile     *string
	Threads      *int
	Short        *bool
	Tiny         *bool
	ShortErrors  *bool
	ErrorsOnly   *bool
	PrintHeaders *bool
	Colorize     *bool
	Interactive  *bool
	Variables    varFlags
	Tags         testTags
}

func (p *ProgramArgs) Init() {
	// somewhat alphabetical order...
	p.PrintHeaders = flag.Bool("always-headers", false, "Always print the request and response headers in long test report output whether any matchers are defined for them or not.")
	p.Colorize = flag.Bool("colors", true, "Print test report with colors.")
	p.ErrorsOnly = flag.Bool("error-report", false, "Generate a test report that only contain failing test results.")
	p.TestFile = flag.String("file", "", "Path to an individual test file to execute.")
	p.Fixtures = flag.String("fixtures", "", "Path to yaml file with data to include into the test scope via test variables.")
	p.Short = flag.Bool("short", true, "Print a short report for executed tests containing only the validation results.")
	p.ShortErrors = flag.Bool("short-fail", false, "Keep the report short when errors are encountered rather than expanding with details.")
	p.Interactive = flag.Bool("step", false, "Run tests in interactive mode. Requires a test file to be provided with '-file'")

	flag.Var(&p.Tags, "tag", "Only execute tests with tags matching this value. Tag input supports comma separated values which will execute "+
		"tests that contain any on of those values. Subsequent tag parameters will AND with previous tag inputs "+
		"to determine what tests will be run. Specifying no tag parameters will execute all tests.")

	p.TestRoot = flag.String("test-root", "", "Folder path containing all the test files to execute.")
	p.Threads = flag.Int("threads", 16, "Max number of test files to execute concurrently.")
	p.Tiny = flag.Bool("tiny", false, "Print an even tinier report output than what the short flag provides. "+
		"Only prints test status, name, and description. Failed tests will still be expanded.")

	flag.Var(&p.Variables, "var", "Prepopulate the tests data store with a single KEY=VALUE pair. Multiple -var parameters can be provided for additional key/value pairs.")

	if len(os.Args) <= 1 {
		flag.Usage()
		os.Exit(0)
	}
	flag.Parse()

	if *p.Threads < 0 {
		def := 1
		p.Threads = &def
	}
}

func populateDataStore(ds *DataStore, vars varFlags) {
	(*ds)["host"] = "http://localhost"
	for _, v := range vars {
		pair := strings.SplitN(v, "=", 2)

		if len(pair) < 2 {
			fmt.Printf("Badly formatted var excluded from test data store: %v\n", v)
			continue
		}
		(*ds)[pair[0]] = pair[1]
	}
}

func runTests(args ProgramArgs) bool {
	var passed bool
	var err error
	var results []MultiSuiteResult
	var testingDuration time.Duration

	if *args.TestFile != "" {
		suite, sErr := NewTestSuite(*args.TestFile, *args.Fixtures)
		if sErr != nil {
			fmt.Printf("%v\n", sErr)
			return false
		}
		suite.Verbose = true
		populateDataStore(&suite.GlobalDataStore, args.Variables)

		r := MultiSuiteResult{
			TestFile: *args.TestFile,
		}
		r.Passed, r.TestResults, r.Error = suite.ExecuteTests(args.Tags)
		results = append(results, r)
		passed = r.Passed
		err = r.Error
		testingDuration = r.TestResults.Duration

	} else if *args.TestRoot != "" {
		var multiTestSuite *MultiTestSuite
		multiTestSuite, err = NewMultiSuiteTest(*args.TestRoot, *args.Fixtures)
		if err != nil {
			fmt.Printf("%v\n", err)
			os.Exit(1)
		}

		for _, suite := range multiTestSuite.Suites {
			populateDataStore(&suite.GlobalDataStore, args.Variables)
		}

		passed, results, testingDuration, err = multiTestSuite.ExecuteTests(*args.Threads, args.Tags)
	}

	if err != nil {
		fmt.Printf("Failed to execute tests: %v\n", err)
		os.Exit(1)
	}

	if len(results) == 0 {
		fmt.Printf("No tests found.")
		os.Exit(1)
	}

	path := *args.TestRoot
	if path == "" {
		path = *args.TestFile
	}

	opts := ReportOptions{
		Tiny:               *args.Tiny,
		ShortErrors:        *args.ShortErrors,
		Short:              *args.Short,
		TestsPath:          path,
		AlwaysPrintHeaders: *args.PrintHeaders,
		ErrorsOnly:         *args.ErrorsOnly,
		Colors: Colorizer{
			Enabled: *args.Colorize,
		},
	}

	PrintReport(opts, passed, testingDuration, results)
	return passed
}

type StepInput struct {
	FallThrough        bool
	StepThroughToError bool
	Exit               bool
	Retry              bool
	HotReload          bool
}

func interactivePrompt(showOpts bool, canRetry bool, websocketMode bool) {
	nextMsg := "Execute next test"
	if websocketMode {
		nextMsg = "Send next websocket message"
	}

	options := []string{
		fmt.Sprintf("n) %s", nextMsg),
		"r) Retry test",
		"e) Halt further testing and exit program",
		"f) Exit interactive mode and automatically run remaining tests",
		"d) Dump all values in data store",
		"x) Step through tests until next failure",
		"q) Hot reload test file",
		"*) Expand typed variable. e.g. @{host}",
	}

	if showOpts {
		fmt.Printf("\nInput options:\n")
		for _, o := range options {
			if strings.HasPrefix(o, "r)") && !canRetry {
				continue
			}

			PrintIndentedLn(1, "%v\n", o)
		}
	}
	fmt.Printf("\nCommand: ")
}

func interactiveInput(tests []*TestCase, curTest int, subTest bool, result *TestResult) StepInput {
	nextTestNo := curTest + 1
	canRetry := true && !subTest
	websocketPrompt := tests[curTest].Config.Websocket && subTest

	if result == nil {
		nextTestNo = curTest
		canRetry = false
	}
	if websocketPrompt {
		fmt.Printf("Next test: Send next websocket request.")
	} else if nextTestNo < len(tests) {
		fmt.Printf("Next test: %v - %v\n", tests[nextTestNo].Config.Name, tests[nextTestNo].Config.Description)
	} else {
		fmt.Printf("No more tests")
	}
	interactivePrompt(true, canRetry, websocketPrompt)

	for {
		input := ""
		fmt.Scanln(&input)

		if input == "" {
			return StepInput{}
		}

		switch strings.ReplaceAll(input, "\n", "") {
		case "n":
			return StepInput{}
		case "e":
			return StepInput{Exit: true}
		case "f":
			return StepInput{FallThrough: true}
		case "r":
			if canRetry {
				return StepInput{Retry: true}
			}
		case "d":
			pretty, _ := json.MarshalIndent(tests[curTest].GlobalDataStore, "", IndentStr(1))
			fmt.Printf("%v\n", string(pretty))
		case "x":
			return StepInput{FallThrough: true, StepThroughToError: true}
		case "q":
			return StepInput{HotReload: true}
		default:
			expanded, err := tests[curTest].GlobalDataStore.ExpandVariable(input)
			if err != nil {
				fmt.Printf("\nFailed to expand variable: %v\n", err)
			} else {
				if _, ok := expanded.(string); !ok {
					data, _ := json.MarshalIndent(expanded, "", IndentStr(1))
					expanded = string(data)
				}

				fmt.Printf("%v -> %v\n", input, expanded)
			}
		}

		interactivePrompt(false, true, websocketPrompt)
	}
}

func interactiveMode(args ProgramArgs) bool {
	opts := ReportOptions{
		Tiny:               *args.Tiny,
		ShortErrors:        *args.ShortErrors,
		Short:              *args.Short,
		TestsPath:          *args.TestFile,
		AlwaysPrintHeaders: *args.PrintHeaders,
		Colors: Colorizer{
			Enabled: *args.Colorize,
		},
	}

	suite, err := NewTestSuite(*args.TestFile, *args.Fixtures)
	if err != nil {
		fmt.Printf("Failed to initialize test file: %v\n", err)
		return false
	}
	defer suite.Close()

	populateDataStore(&suite.GlobalDataStore, args.Variables)

	allPassed := true
	var stepInput StepInput
	testNo := 0
	stepInput = interactiveInput(suite.Tests, 0, false, nil)

	// Using range will create a slice copy of the tests which won't allow us
	// to hot reload them.
	for !stepInput.Exit && testNo < len(suite.Tests) {
		test := suite.Tests[testNo]

		var passed bool
		var result *TestResult
		var err error

		// If test is a websocket, lets step through each request/response
		if test.Config.Websocket && !test.Config.Skip && !test.SkipTestOnTags(args.Tags) {
			totalSteps := 1
			result = &TestResult{
				TestCase:  *test,
				StartTime: time.Now().UTC(),
			}

			wsStep := 0
			testName := test.Config.Name
			finalPassed := true
			for !stepInput.Exit && !stepInput.HotReload && totalSteps > 0 {
				var remaining int
				passed, remaining, err = test.StepExecWebsocket(wsStep, result)

				finalPassed = passed
				totalSteps = remaining
				result.TestCase.Config.Name = fmt.Sprintf("%v [%v/%v]", testName, wsStep+1, wsStep+totalSteps+1)

				opts.InProgress = remaining != 0

				if opts.InProgress {
					PrintSingleTestReport(opts, result)
					if err != nil {
						PrintIndentedLn(1, opts.Colors.BrightRed("Some tests failed to execute:\n"))
						PrintIndentedLn(1, "%v\n", err)
						return false
					}
					if !stepInput.FallThrough {
						stepInput = interactiveInput(suite.Tests, testNo, remaining != 0, result)
					}
					// No retry support for individual websocket messages. Must retry the entire test
					wsStep += 1
				}
			}
			allPassed = allPassed && finalPassed
		} else {
			passed, result, err = test.Execute(args.Tags)
			allPassed = allPassed && passed
		}

		if !stepInput.HotReload && !stepInput.Exit {
			PrintSingleTestReport(opts, result)
			if err != nil {
				PrintIndentedLn(1, opts.Colors.BrightRed("Some tests failed to execute:\n"))
				PrintIndentedLn(1, "%v\n", err)
				return allPassed
			}

			if !passed && stepInput.StepThroughToError {
				stepInput.FallThrough = false
			}

			if !stepInput.FallThrough {
				stepInput = interactiveInput(suite.Tests, testNo, false, result)
				if !stepInput.Retry && !stepInput.HotReload {
					testNo += 1
				}
				fmt.Print("\033[H\033[2J")
			} else {
				testNo += 1
			}
		} else if stepInput.HotReload {
			loaded := false

			for !loaded {
				loaded, err = suite.ReloadFile(*args.TestFile)
				if err != nil {
					fmt.Printf("Hot reload error: %v\nPlease correct your file then press 'enter' to reload...\n", err)
					input := ""
					fmt.Scanln(&input)
				}
			}

			stepInput.HotReload = false
			fmt.Print("\033[H\033[2J")
			fmt.Printf("Reloaded test file. Resuming tests from last started test case.\n\n")
		}
	}

	return allPassed
}

func main() {
	args := ProgramArgs{}
	args.Init()

	var passed bool
	if *args.Interactive {
		passed = interactiveMode(args)
	} else {
		passed = runTests(args)
	}

	if !passed {
		os.Exit(1)
	}
	os.Exit(0)
}
