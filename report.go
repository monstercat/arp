package arp

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

type ReportOptions struct {
	ShortErrors        bool
	Short              bool
	Tiny               bool
	AlwaysPrintHeaders bool
	ErrorsOnly         bool
	TestsPath          string
	Colors             Colorizer
	// Any failures while report is printed are suppresed and and indication
	// is provided that the result data may be incomplete
	InProgress bool
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

func IndentStr(level int) string {
	indents := ""
	for i := 0; i < level; i++ {
		indents += " "
	}

	return indents
}

func PrintIndentedLn(indentLevel int, format string, args ...interface{}) {
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
	newArgs = append(newArgs, IndentStr(indentLevel))
	newArgs = append(newArgs, args...)

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
	case "in_progress":
		return c.BrightYellow("In Progress")
	case "partial_validation":
		if status {
			return c.Green("*")
		}
		return c.BrightYellow("o")
	}
}

func ShouldShowReport(opts ReportOptions, test *TestResult) bool {
	return (opts.ErrorsOnly && !test.Passed) || !opts.ErrorsOnly
}

func PrintSingleTestReport(opts ReportOptions, test *TestResult) {
	showErrors := false
	if !test.Passed {
		showErrors = !opts.ShortErrors && !opts.InProgress
	}

	if !ShouldShowReport(opts, test) {
		return
	}

	showExtendedReport := !opts.Short || showErrors
	showFieldValidations := showExtendedReport || !opts.Tiny

	details := test.TestCase
	routeStr := fmt.Sprintf("[%v] %v", opts.Colors.BrightCyan(details.Config.Method), opts.Colors.BrightWhite(details.Config.Route))
	statusStyle := ""
	if test.TestCase.Config.Skip {
		statusStyle = "skipped"
	}
	if opts.InProgress {
		statusStyle = "in_progress"
	}

	delta := test.EndTime.Sub(test.StartTime)
	timeStr := fmt.Sprintf("%v: %v", opts.Colors.BrightWhite("Test Duration"), delta)

	PrintIndentedLn(1, "[%v] %v - %v\n", getSuccessString(opts.Colors, test.Passed, statusStyle),
		opts.Colors.BrightWhite(details.Config.Name), details.Config.Description)
	PrintIndentedLn(2, "%v\n", timeStr)
	PrintIndentedLn(1, "%v\n", routeStr)

	if showFieldValidations {
		sort.Slice(test.Fields, func(i, j int) bool {
			a := test.Fields[i].ObjectKeyPath
			b := test.Fields[j].ObjectKeyPath

			if a[0] == b[0] || (a[0] != '.' && b[0] != '.') {
				return a < b
			} else if a[0] != '.' {
				return true
			} else {
				return false
			}
		})

		for _, f := range test.Fields {
			if f.IgnoreResult {
				continue
			}

			fieldStr := f.ObjectKeyPath

			suffix := "..."
			maxLength := 64
			if len(f.Error) < maxLength {
				maxLength = len(f.Error)
				suffix = ""
			}

			shortStr := ""
			charCounter := 0
			for _, c := range f.Error {
				if charCounter >= maxLength {
					shortStr += suffix
					break
				}
				shortStr += string(c)
				charCounter++
			}
			shortStr = fmt.Sprintf("%q", shortStr)
			if !f.Status {
				fieldStr = opts.Colors.Cyan(fieldStr)
				shortStr = opts.Colors.BrightYellow(shortStr)
			} else {
				fieldStr = opts.Colors.BrightBlue(fieldStr)
			}

			style := "validation"
			if opts.InProgress && f.Error == ReceivedNullErrFmt {
				style = "partial_validation"
				shortStr = opts.Colors.BrightYellow("Pending next websocket message...")
			}

			PrintIndentedLn(2, "[%v] %v: %v\n", getSuccessString(opts.Colors, f.Status, style),
				fieldStr, shortStr)
		}
	}
	fmt.Printf("\n")

	if showExtendedReport {
		PrintIndentedLn(2, "Route: %v\n", test.ResolvedRoute)
		PrintIndentedLn(2, "Status Code: %v\n", test.StatusCode)

		if len(test.TestCase.Config.Headers) > 0 || opts.AlwaysPrintHeaders {
			requestHeadersJson, _ := json.MarshalIndent(test.RequestHeaders, IndentStr(2), " ")
			PrintIndentedLn(2, "Request Headers: %v\n", string(requestHeadersJson))
		}

		if len(test.TestCase.ResponseHeaderMatcher.Config) > 0 || opts.AlwaysPrintHeaders {
			// only print headers long output if the test case is validating any of them
			headerJson, _ := json.MarshalIndent(test.ResponseHeaders, IndentStr(2), " ")
			PrintIndentedLn(2, "Response Headers: %v\n", string(headerJson))
		}

		input := YamlToJson(test.TestCase.Config.Input)
		inputJson, _ := json.MarshalIndent(input, IndentStr(2), " ")
		PrintIndentedLn(2, "Input: %v\n", string(inputJson))

		data, _ := json.MarshalIndent(test.Response, IndentStr(2), " ")
		PrintIndentedLn(2, "Response: %v\n\n", string(data))

		PrintIndentedLn(2, "Extended Output:\n")
		for _, f := range test.Fields {
			if f.ShowExtendedMsg {
				PrintIndentedLn(3, fmt.Sprintf("%v", f.ObjectKeyPath))
				PrintIndentedLn(5, fmt.Sprintf("%v:\n", f.Error))
			}
		}

		fmt.Print(opts.Colors.BrightWhite("---\n"))
	}
}

func PrintReport(opts ReportOptions, passed bool, testingDuration time.Duration, results []MultiSuiteResult) {
	globalFailed := 0
	globalPassed := 0
	var globalTestDuration time.Duration
	fmt.Printf("\n\n")
	for _, r := range results {
		globalFailed += r.TestResults.Failed
		globalPassed += r.TestResults.Passed
		globalTestDuration += r.TestResults.Duration

		PrintIndentedLn(0, "[%v] %v\n", getSuccessString(opts.Colors, r.Passed, ""),
			opts.Colors.Underline(opts.Colors.BrightWhite(r.TestFile)))
		PrintIndentedLn(1, "Suite Duration: %v\n", r.TestResults.Duration)
		PrintIndentedLn(1, "Passed: %v, Failed: %v, Total:%v\n", r.TestResults.Passed,
			r.TestResults.Failed, r.TestResults.Total)

		fmt.Printf("%v\n", separator(opts.Colors))

		for _, test := range r.TestResults.Results {
			if ShouldShowReport(opts, test) {
				PrintSingleTestReport(opts, test)
			}
		}

		if r.Error != nil {
			PrintIndentedLn(1, opts.Colors.BrightRed("One or more tests failed within execution and the test suite could not be completed:\n"))
			PrintIndentedLn(1, "%q\n\n", r.Error)
		}
	}

	fmt.Printf("%v\n", separator(opts.Colors))
	path := opts.TestsPath

	PrintIndentedLn(0, "[%v] %v\n", getSuccessString(opts.Colors, passed, ""), opts.Colors.BrightWhite(path))
	PrintIndentedLn(0, "%-6[2]d:Total Tests\n%-6[3]d:Passed\n%-6[4]d:Failed\n", globalPassed+globalFailed, globalPassed, globalFailed)
	PrintIndentedLn(0, "\nTotal Execution Time: %v (CPU Time: %v)\n", testingDuration, globalTestDuration)
	fmt.Printf("%v\n", separator(opts.Colors))

}
