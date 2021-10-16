package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
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

type ProgramArgs struct {
	Fixtures    *string
	TestRoot    *string
	TestFile    *string
	Threads     *int
	Short       *bool
	Tiny        *bool
	ShortErrors *bool
	Colorize    *bool
	Interactive *bool
	Variables   varFlags
}

func (p *ProgramArgs) Init() {
	p.Fixtures = flag.String("fixtures", "", "Path to yaml file with data to include into the test scope via test variables.")
	p.TestRoot = flag.String("test-root", "", "File path to scan and execute test files from")
	p.Threads = flag.Int("threads", 16, "Max number of test files to execute concurrently")
	p.Short = flag.Bool("short", true, "Print a short report for executed tests containing only the validation results")
	p.Tiny = flag.Bool("tiny", false, "Print an even tinier report output than what the short flag provides. "+
		"Only prints test status, name, and description. Failed tests will still be expanded")
	p.ShortErrors = flag.Bool("short-fail", false, "Keep the report short when errors are encountered rather than expanding with details")
	p.Colorize = flag.Bool("colors", true, "Print test report with colors")
	p.TestFile = flag.String("file", "", "Single file path to a test suite to execute.")
	p.Interactive = flag.Bool("step", false, "Execute a single test file in interactive mode. "+
		"Requires a test file to be provided with '-file'")

	flag.Var(&p.Variables, "var", "Prepopulate the tests data store with a KEY=VALUE pair.")

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

type Colorizer struct {
	Enabled bool
}

func (c *Colorizer) Underline(input string) string {
	return c.colorizeStr(input, "\033[4m")
}

func (c *Colorizer) BrightGrey(input string) string {
	return c.colorizeStr(input, "\033[37;1m")
}

func (c *Colorizer) BrightRed(input string) string {
	return c.colorizeStr(input, "\033[31;1m")
}

func (c *Colorizer) BrightWhite(input string) string {
	return c.colorizeStr(input, "\033[37;1m")
}

func (c *Colorizer) BrightYellow(input string) string {
	return c.colorizeStr(input, "\033[33;1m")
}

func (c *Colorizer) BrightCyan(input string) string {
	return c.colorizeStr(input, "\033[36;1m")
}

func (c *Colorizer) BrightBlue(input string) string {
	return c.colorizeStr(input, "\033[34;1m")
}
func (c *Colorizer) Cyan(input string) string {
	return c.colorizeStr(input, "\033[36m")
}

func (c *Colorizer) Red(input string) string {
	return c.colorizeStr(input, "\033[31m")
}

func (c *Colorizer) Green(input string) string {
	return c.colorizeStr(input, "\033[32m")
}

func (c *Colorizer) Yellow(input string) string {
	return c.colorizeStr(input, "\033[33m")
}

func (c *Colorizer) colorizeStr(input string, color string) string {
	if !c.Enabled {
		return input
	}
	return fmt.Sprintf("%v%v%v", color, input, "\033[0m")
}

func indentStr(level int) string {
	indents := ""
	for i := 0; i < level; i++ {
		indents += " "
	}

	return indents
}

func printIndentedLn(indentLevel int, format string, args ...interface{}) {
	indentFmt := "%[1]v"

	for i := 0; i < len(format); i++ {
		indentFmt += string(format[i])
		// if we reach a newline character and there are more characters after it, indent
		// the next line to the same level
		if format[i] == '\n' && i+1 < len(format) {
			indentFmt += "%[1]v"
		}
	}

	var newArgs []interface{}
	newArgs = append(newArgs, indentStr(indentLevel))
	for _, a := range args {
		newArgs = append(newArgs, a)
	}

	fmt.Printf(indentFmt, newArgs...)
}

func separator(c Colorizer) string {
	sep := ""
	for i := 0; i < 80; i++ {
		sep += "-"
	}

	return c.BrightWhite(sep)
}

func getSuccessString(c Colorizer, status bool, style string) string {
	switch style {
	default:
		fallthrough
	case "test":
		if status {
			return c.Green("Passed")
		}

		return c.Red("Failed")
	case "validation":
		if status {
			return c.Green("*")
		}

		return c.Red("x")

	case "skipped":
		return c.BrightGrey("Skipped")
	}
}

func printSingleTestReport(c Colorizer, args ProgramArgs, test *TestResult) {
	showErrors := false
	if !test.Passed {
		showErrors = !*args.ShortErrors
	}

	showExtendedReport := !(*args.Short) || showErrors
	showFieldValidations := showExtendedReport || !*args.Tiny

	details := test.TestCase
	routeStr := fmt.Sprintf("[%v] %v", c.BrightCyan(details.Method), c.BrightWhite(details.Route))
	statusStyle := ""
	if test.TestCase.Skip {
		statusStyle = "skipped"
	}

	delta := test.EndTime.Sub(test.StartTime)
	timeStr := fmt.Sprintf("%v: %v", c.BrightWhite("Test Duration"), delta)

	printIndentedLn(1, "[%v] %v - %v\n", getSuccessString(c, test.Passed, statusStyle),
		c.BrightWhite(details.Name), details.Description)
	printIndentedLn(2, "%v\n", timeStr)
	printIndentedLn(1, "%v\n", routeStr)
	if showFieldValidations {
		sort.Slice(test.Fields, func(i, j int) bool {
			a := test.Fields[i].ObjectKeyPath
			b := test.Fields[j].ObjectKeyPath

			return a[0] != b[0] || a < b
		})
		for _, f := range test.Fields {
			fieldStr := f.ObjectKeyPath
			errStr := f.Error
			if !f.Status {
				fieldStr = c.Cyan(fieldStr)
				errStr = c.BrightYellow(errStr)
			} else {
				fieldStr = c.BrightBlue(fieldStr)
			}

			printIndentedLn(2, "[%v] %v: %v\n", getSuccessString(c, f.Status, "validation"),
				fieldStr, errStr)
		}
	}
	fmt.Printf("\n")

	if showExtendedReport {
		printIndentedLn(2, "Route: %v\n", test.ResolvedRoute)
		printIndentedLn(2, "Status Code: %v\n", test.StatusCode)

		input := YamlToJson(test.TestCase.Input)
		inputJson, _ := json.MarshalIndent(input, indentStr(2), " ")
		printIndentedLn(2, "Input: %v\n", string(inputJson))

		data, _ := json.MarshalIndent(test.Response, indentStr(2), " ")
		printIndentedLn(2, "Response: %v\n\n", string(data))
		fmt.Printf(c.BrightWhite("---\n"))
	}
}

func printReport(c Colorizer, args ProgramArgs, passed bool, results []MultiSuiteResult) {
	globalFailed := 0
	globalPassed := 0
	var globalTestDuration time.Duration

	fmt.Printf("\n\n")
	for _, r := range results {
		printIndentedLn(0, "[%v] %v\n", getSuccessString(c, r.Passed, ""),
			c.Underline(c.BrightWhite(r.TestFile)))
		printIndentedLn(1, "Suite Duration: %v\n", r.TestResults.Duration)
		printIndentedLn(1, "Passed: %v, Failed: %v, Total:%v\n", r.TestResults.Passed,
			r.TestResults.Failed, r.TestResults.Total)

		globalFailed += r.TestResults.Failed
		globalPassed += r.TestResults.Passed
		globalTestDuration += r.TestResults.Duration

		fmt.Printf("%v\n", separator(c))

		for _, test := range r.TestResults.Results {
			printSingleTestReport(c, args, test)
		}

		if r.Error != nil {
			printIndentedLn(1, c.BrightRed("Some tests failed to execute:\n"))
			printIndentedLn(1, "%v\n", r.Error)
		}

	}

	fmt.Printf("%v\n", separator(c))
	path := *args.TestRoot
	if path == "" {
		path = *args.TestFile
	}

	printIndentedLn(0, "[%v] %v\n", getSuccessString(c, passed, ""), c.BrightWhite(path))
	printIndentedLn(0, "%-6[2]d:Total Tests\n%-6[3]d:Passed\n%-6[4]d:Failed\n", globalPassed+globalFailed, globalPassed, globalFailed)
	printIndentedLn(0, "\nTotal Execution Time: %v\n", globalTestDuration)
	fmt.Printf("%v\n", separator(c))

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

	if *args.TestFile != "" {
		suite, err := NewTestSuite(*args.TestFile, *args.Fixtures)
		if err != nil {
			fmt.Printf("%v\n", err)
			return false
		}
		populateDataStore(&suite.GlobalDataStore, args.Variables)

		r := MultiSuiteResult{
			TestFile: *args.TestFile,
		}
		r.Passed, r.Error, r.TestResults = suite.ExecuteTests()
		results = append(results, r)
		passed = r.Passed
		err = r.Error
	} else if *args.TestRoot != "" {
		multiTestSuite, err := NewMultiSuiteTest(*args.TestRoot, *args.Fixtures)
		if err != nil {
			fmt.Printf("%v\n", err)
			os.Exit(1)
		}

		for _, suite := range multiTestSuite.Suites {
			populateDataStore(&suite.GlobalDataStore, args.Variables)
		}

		passed, err, results = multiTestSuite.ExecuteTests(*args.Threads)
	}

	if err != nil {
		fmt.Printf("Failed to execute tests: %v\n", err)
		os.Exit(1)
	}

	if len(results) == 0 {
		fmt.Printf("No tests found.")
		os.Exit(1)
	}

	colorizer := Colorizer{
		Enabled: *args.Colorize,
	}

	printReport(colorizer, args, passed, results)
	return passed
}

type StepInput struct {
	FallThrough        bool
	StepThroughToError bool
	Exit               bool
	Retry              bool
}

func interactivePrompt(showOpts bool, canRetry bool) {
	options := []string{
		"n) Execute next test",
		"r) Retry test",
		"e) Halt further testing and exit program",
		"f) Exit interactive mode and automatically run remaining tests",
		"d) Dump all values in data store",
		"x) Step through tests until next failure",
		"*) Expand typed variable. e.g. @{host}",
	}

	if showOpts {
		fmt.Printf("\nInput options:\n")
		for _, o := range options {
			if strings.HasPrefix(o, "r)") && !canRetry {
				continue
			}

			printIndentedLn(1, "%v\n", o)
		}
	}
	fmt.Printf("\nCommand: ")
}

func interactiveInput(tests []TestCase, curTest int, result *TestResult) StepInput {
	nextTestNo := curTest + 1
	canRetry := true

	if result == nil {
		nextTestNo = curTest
		canRetry = false
	}

	if nextTestNo < len(tests) {
		fmt.Printf("Next test: %v - %v\n", tests[nextTestNo].Name, tests[nextTestNo].Description)
	} else {
		fmt.Printf("No more tests")
	}
	interactivePrompt(true, canRetry)

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
			pretty, _ := json.MarshalIndent(tests[curTest].GlobalDataStore, "", indentStr(1))
			fmt.Printf("%v\n", string(pretty))
		case "x":
			return StepInput{FallThrough: true, StepThroughToError: true}
		default:
			expanded, err := tests[curTest].GlobalDataStore.ExpandVariable(input)
			if err != nil {
				fmt.Printf("\nFailed to expand variable: %v\n", err)
			} else {
				if _, ok := expanded.(string); !ok {
					data, _ := json.MarshalIndent(expanded, "", indentStr(1))
					expanded = string(data)
				}

				fmt.Printf("%v -> %v\n", input, expanded)
			}
		}

		interactivePrompt(false, true)
	}
}

func interactiveMode(args ProgramArgs) bool {
	c := Colorizer{
		Enabled: *args.Colorize,
	}

	suite, err := NewTestSuite(*args.TestFile, *args.Fixtures)
	if err != nil {
		fmt.Printf("Failed to initialize test file: %v\n", err)
		return false
	}
	populateDataStore(&suite.GlobalDataStore, args.Variables)

	allPassed := true
	var stepInput StepInput

	testNo := 0
	stepInput = interactiveInput(suite.Tests, 0, nil)
	for !stepInput.Exit && testNo < len(suite.Tests) {
		test := suite.Tests[testNo]
		passed, err, result := suite.ExecuteTest(&test)
		allPassed = allPassed && passed

		printSingleTestReport(c, args, result)
		if err != nil {
			printIndentedLn(1, c.BrightRed("Some tests failed to execute:\n"))
			printIndentedLn(1, "%v\n", err)
			return allPassed
		}

		if !passed && stepInput.StepThroughToError {
			stepInput.FallThrough = false
		}

		if !stepInput.FallThrough {
			stepInput = interactiveInput(suite.Tests, testNo, result)
			if !stepInput.Retry {
				testNo += 1
			}
			fmt.Print("\033[H\033[2J")
		} else {
			testNo += 1
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
