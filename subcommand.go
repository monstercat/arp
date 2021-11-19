package arp

import (
	"fmt"
	"os/exec"
	"strings"
)

const (
	CMD_PREFIX = "$("
	CMD_SUFFIX = ")"
)

func executeCommandStr(input string) (string, error) {
	var tokens []string
	inQuote := false
	argStartPos := 0
	escaped := false

	realCmd := input[len(CMD_PREFIX) : len(input)-len(CMD_SUFFIX)]
	for i := 0; i < len(realCmd) && argStartPos < len(realCmd); i++ {
		char := realCmd[i]

		if char == '\\' {
			escaped = true
			// skip our escape character
			continue
		} else if escaped {
			escaped = false
			// this is the character we're escaping, so skip checking it
			continue
		}

		// regular arguments can be separated by spaces if not quoted
		if !inQuote && char == ' ' && argStartPos != i {
			// no +1 on token end to exclude the space delimiter
			tokens = append(tokens, realCmd[argStartPos:i])
			argStartPos = i + 1
		} else if inQuote && char == ' ' {
			// if we're in a quote and hit a space, we can ignore it
			continue
		} else if !inQuote && char == '"' {
			//if we aren't in a quoted string and we hit a quote, then we can continue
			inQuote = true
		} else if inQuote && char == '"' {
			// if we are in a quote and we hit another quote, we'll treat that as the closing one
			// +1 to include the end quote for our token
			tokens = append(tokens, realCmd[argStartPos:i+1])
			inQuote = false
			// make our next argument starting position skip this closing quote
			argStartPos = i + 1
		}
	}

	if argStartPos < len(realCmd) {
		tokens = append(tokens, realCmd[argStartPos:])
	}

	// Promote all nested quotes 'up' one level
	var args []string
	for _, s := range tokens {
		newToken := ""
		// if the whole token is quoted, then we can remove them and promote the nested quotes
		if s[0] == '"' && s[len(s)-1] == '"' {
			s = s[1 : len(s)-1]
		} else {
			// otherwise, leave the string alone.
			args = append(args, s)
			continue
		}

		for i := 0; i < len(s); i++ {
			if s[i] == '\\' {
				escapeEnd := i + 1
				for ; escapeEnd < len(s); escapeEnd++ {
					if s[escapeEnd] != '\\' {
						break
					}
				}
				escapeCount := escapeEnd - i

				// If we have just one escape character, then it no longer becomes escaped. This is assuming that
				// the first escape character is due to the escaping required on the yaml config format of '$(PROGRAM \"ARGS\")'
				// where $() requires to be quoted and the first grouping of args is quoted to prevent incorrect splitting.
				if escapeCount == 1 {
					i = escapeEnd - 1
					continue
				}

				if escapeCount%2 == 0 {
					// if we have an even amount, remove one to make it odd.
					for c := 0; c < escapeCount-1; c++ {
						newToken += string('\\')
					}
				} else {
					// otherwise, remove 2 if there's an odd amount. This will keep it odd, but 'promote' the escape
					// sequence up a level in the string.
					for c := 0; c < escapeCount-2; c++ {
						newToken += string('\\')
					}
				}
				i = escapeEnd - 1
				continue
			}

			newToken += string(s[i])

		}
		args = append(args, newToken)
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
