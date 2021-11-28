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

type TokenQuoteState struct {
	InDoubleQuote   bool
	InSingleQuote   bool
	InBacktickQuote bool
}

func (ts *TokenQuoteState) InQuote() bool {
	return ts.InSingleQuote || ts.InDoubleQuote || ts.InBacktickQuote
}

func (ts *TokenQuoteState) IsQuote(char uint8) bool {
	return char == '"' || char == '\'' || char == '`'
}

func (ts *TokenQuoteState) SetQuote(char uint8) {
	switch char {
	case '"':
		ts.InDoubleQuote = true
	case '\'':
		ts.InSingleQuote = true
	case '`':
		ts.InBacktickQuote = true
	}
}

func (ts *TokenQuoteState) UnsetQuote(char uint8) {
	switch char {
	case '"':
		ts.InDoubleQuote = false
	case '\'':
		ts.InSingleQuote = false
	case '`':
		ts.InBacktickQuote = false
	}
}

func splitStringTokens(input string) []string {

	quoteState := TokenQuoteState{}
	tokenStartPos := 0
	escaped := false

	// First split string by words or quoted blocks
	var tokens []string
	sanitizedInput := strings.TrimSpace(input)

	for i := 0; i < len(sanitizedInput) && tokenStartPos < len(sanitizedInput); i++ {
		char := input[i]

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
		if !quoteState.InQuote() && char == ' ' && tokenStartPos != i {
			// no +1 on token end to exclude the space delimiter
			t := strings.TrimSpace(sanitizedInput[tokenStartPos:i])
			if t != "" {
				tokens = append(tokens, t)
			}
			tokenStartPos = i
		} else if quoteState.InQuote() && char == ' ' {
			// if we're in a quote and hit a space, we can ignore it
			continue
		} else if !quoteState.InQuote() && quoteState.IsQuote(char) {
			//if we aren't in a quoted string and we hit a quote, then we can continue
			quoteState.SetQuote(char)
		} else if quoteState.InQuote() && quoteState.IsQuote(char) {
			quoteState.UnsetQuote(char)
			// make sure we're not in nested quotes of mixed types
			if quoteState.InQuote() {
				continue
			}

			t := strings.TrimSpace(sanitizedInput[tokenStartPos : i+1])
			if t != "" {
				tokens = append(tokens, t)
			}

			// make our next argument starting position skip this closing quote
			tokenStartPos = i + 1
		}
	}

	if tokenStartPos < len(sanitizedInput) {
		t := strings.TrimSpace(sanitizedInput[tokenStartPos:])
		if t != "" {
			tokens = append(tokens, t)
		}
	}

	// Promote all nested quotes 'up' one level
	var promotedStrings []string
	for _, s := range tokens {
		newToken := ""
		// if the whole token is quoted, then we can remove them and promote the nested quotes
		if quoteState.IsQuote(s[0]) && quoteState.IsQuote(s[len(s)-1]) {
			s = s[1 : len(s)-1]
		} else {
			// otherwise, leave the string alone.
			promotedStrings = append(promotedStrings, s)
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
		promotedStrings = append(promotedStrings, newToken)
	}

	return promotedStrings
}

func executeCommandStr(input string) (string, error) {
	realCmd := input[len(CMD_PREFIX) : len(input)-len(CMD_SUFFIX)]
	args := splitStringTokens(realCmd)
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
