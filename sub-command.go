package arp

import (
	"fmt"
	"os/exec"
	"strings"
)

const (
	CMD_PREFIX    = "$("
	CMD_SUFFIX    = ")"
	CMD_DELIMITER = " "
)

func executeCommandStr(input string) (string, error) {
	sanitized := []rune(input)
	sanitized = sanitized[len(CMD_PREFIX) : len(sanitized)-len(CMD_SUFFIX)]
	args := PromoteTokenQuotes(SplitStringTokens(string(sanitized), CMD_DELIMITER))
	if len(args) == 0 {
		return "", nil
	}

	cmd := exec.Command(args[0], args[1:]...)
	val, err := cmd.CombinedOutput()
	return strings.TrimSuffix(string(val), "\n"), err
}

func isCmd(input string) bool {
	return strings.HasPrefix(input, CMD_PREFIX) && strings.HasSuffix(input, CMD_SUFFIX)
}

func ExecuteCommand(input string) (interface{}, error) {
	var outputString = input
	commands := TokenStack{}
	commands.Parse(input, CMD_PREFIX, CMD_SUFFIX)

	if len(commands.Frames) == 0 {
		return input, nil
	}

	type ExtendedStackFrame struct {
		TokenStackFrame
		ExecuteCommandResult string
	}

	toExecute := []ExtendedStackFrame{}
	for _, v := range commands.Frames {
		toExecute = append(toExecute, ExtendedStackFrame{
			TokenStackFrame:      v,
			ExecuteCommandResult: v.Token,
		})
	}

	for i, v := range toExecute {
		var commandOutput string
		// make sure we are executing commands and not the results of commands that were already executed
		if isCmd(v.ExecuteCommandResult) {
			var err error
			commandOutput, err = executeCommandStr(v.ExecuteCommandResult)
			if err != nil {
				errMsg := fmt.Sprintf("Execution error: %v: %q", err, commandOutput)
				return errMsg, fmt.Errorf(errMsg)
			}
		}

		if v.Nested == 0 {
			outputString = strings.ReplaceAll(outputString, v.Token, commandOutput)
		}
		// once a command is executed, we want to populate the parent command stack frames with the text result in place
		// of this nested command.
		for offset := i + 1; offset < len(toExecute); offset++ {
			frame := toExecute[offset]
			if !strings.Contains(frame.ExecuteCommandResult, v.Token) {
				continue
			}
			frame.ExecuteCommandResult = strings.ReplaceAll(frame.ExecuteCommandResult, v.Token, commandOutput)
			toExecute[offset] = frame
		}

	}
	return outputString, nil
}

// Iterate through an object and execute any command strings that are located.
// Returns the input object with the command strings expanded to their results
func RecursiveExecuteCommand(input interface{}) (interface{}, error) {
	if input == nil {
		return nil, nil
	}

	switch n := input.(type) {
	case map[interface{}]interface{}:
		for k := range n {
			if node, err := RecursiveExecuteCommand(n[k]); err != nil {
				return nil, err
			} else {
				n[k] = node
			}
		}
		return n, nil
	case map[string]interface{}:
		for k := range n {
			if node, err := RecursiveExecuteCommand(n[k]); err != nil {
				return nil, err
			} else {
				n[k] = node
			}
		}
		return n, nil
	case []interface{}:
		for i, e := range n {
			if node, err := RecursiveExecuteCommand(e); err != nil {
				return nil, err
			} else {
				n[i] = node
			}
		}
		return n, nil
	case []string:
		var newElements []interface{}
		for _, e := range n {
			res, err := ExecuteCommand(e)
			if err != nil {
				return nil, err
			}
			newElements = append(newElements, res)
		}
		return newElements, nil
	case string:
		res, err := ExecuteCommand(n)
		if res == nil {
			return input, nil
		}
		return res, err
	}

	return input, nil
}
