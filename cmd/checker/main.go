package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	. "github.com/monstercat/integration-checker"
)

type ProgramArgs struct {
	DefaultHost *string
	Fixtures    *string
	TestRoot    *string
	Threads     *int
	Short       *bool
	Colorize    *bool
}

func (p *ProgramArgs) Init() {
	p.DefaultHost = flag.String("host", "http://localhost", "Default host url to use with tests. Populates the @{host} variable.")
	p.Fixtures = flag.String("fixtures", "./fixtures.yaml", "Path to yaml file with data to include into the test scope via test variables.")
	p.TestRoot = flag.String("test-root", ".", "File path to scan and execute test files from")
	p.Threads = flag.Int("threads", 16, "Number of test files to execute at a time.")
	p.Short = flag.Bool("short", true, "Whether or not to print out a short or extended report")
	p.Colorize = flag.Bool("colors", true, "Whether to print test report with colors")

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

func printReport(c Colorizer, shortReport bool, testRoot string, passed bool, results []MultiSuiteResult) {
	globalFailed := 0
	globalPassed := 0

	fmt.Printf("\n")
	for _, r := range results {
		fmt.Printf("\n%v[%v] %v\n", indentStr(0), getSuccessString(c, r.Passed, ""),
			c.Underline(c.BrightWhite(r.TestFile)))

		fmt.Printf("%[1]vPassed: %v, Failed: %v, Total:%v\n", indentStr(1), r.TestResults.Passed,
			r.TestResults.Failed, r.TestResults.Total)

		fmt.Printf("%v\n", separator(c))

		for _, test := range r.TestResults.Results {
			details := test.TestCase
			routeStr := c.BrightWhite(fmt.Sprintf("[%v] %v", details.Method, details.Route))

			if test.Passed {
				globalPassed += 1
			} else {
				globalFailed += 1
			}

			statusStyle := ""
			if test.TestCase.Skip {
				statusStyle = "skipped"
			}

			fmt.Printf("%v[%v]: %v - %v -> %v\n", indentStr(1), getSuccessString(c, test.Passed, statusStyle),
				c.BrightWhite(details.Name), details.Description, routeStr)

			for _, f := range test.Fields {
				fieldStr := f.ObjectKeyPath
				errStr := f.Error
				if !f.Status {
					fieldStr = c.Cyan(fieldStr)
					errStr = c.BrightYellow(errStr)
				} else {
					fieldStr = c.BrightBlue(fieldStr)
				}

				fmt.Printf("%v[%v] %v: %v\n", indentStr(2), getSuccessString(c, f.Status, "validation"),
					fieldStr, errStr)
			}
			fmt.Printf("\n")

			if !shortReport {
				input := YamlToJson(test.TestCase.Input)
				inputJson, _ := json.MarshalIndent(input, indentStr(2), " ")
				fmt.Printf("%vInput: %v\n", indentStr(2), string(inputJson))

				data, _ := json.MarshalIndent(test.Response, indentStr(2), " ")
				fmt.Printf("%vResponse: %v\n", indentStr(2), string(data))
			}
		}

		if r.Error != nil {
			errStr := fmt.Sprintf("%vSome tests failed to execute:\n", indentStr(1))

			fmt.Printf("%v%v\n", c.BrightRed(errStr), r.Error)
		}

	}

	fmt.Printf("%v\n", separator(c))
	fmt.Printf("[%v] %v\n", getSuccessString(c, passed, ""), c.BrightWhite(testRoot))
	fmt.Printf("%-6d:Total Tests\n%-6d:Passed\n%-6d:Failed\n", globalFailed+globalPassed, globalPassed, globalFailed)
	fmt.Printf("%v\n", separator(c))

}

func runTests(args ProgramArgs) bool {
	multiTestSuite, err := NewMultiSuiteTest(*args.DefaultHost, *args.TestRoot, *args.Fixtures)
	if err != nil {
		fmt.Printf("Failed to load tests: %v\n", err)
		os.Exit(1)
	}

	passed, err, results := multiTestSuite.ExecuteTests(*args.Threads)
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

	printReport(colorizer, *args.Short, *args.TestRoot, passed, results)
	return passed
}

func main() {
	args := ProgramArgs{}
	args.Init()

	if passed := runTests(args); !passed {
		os.Exit(1)
	}
	os.Exit(0)
}
