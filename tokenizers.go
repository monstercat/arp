package arp

import (
	"strings"
)

type TokenStackFrame struct {
	StartPos int
	EndPos   int
	Token    string
	Nested   int
}

type TokenStack struct {
	Frames []TokenStackFrame
	Extra  string
}

func (s *TokenStack) Push(f TokenStackFrame) {
	s.Frames = append(s.Frames, f)
}

func (s *TokenStack) Pop() *TokenStackFrame {
	if len(s.Frames) == 0 {
		return nil
	}
	result := s.Frames[len(s.Frames)-1]
	s.Frames = s.Frames[:len(s.Frames)-1]
	return &result
}

func (s *TokenStack) Size() int {
	return len(s.Frames)
}

// Parse Extracts tokens that are wrapped between a predetermined prefix and suffix
// tokens are stored in the order from the inner-most nested out
func (s *TokenStack) Parse(input string, prefix string, suffix string) {
	workStack := TokenStack{}
	var curStackFrame *TokenStackFrame

	runes := []rune(input)

	escapeNextChar := false
	for i := 0; i < len(runes); i++ {
		char := runes[i]
		if char == '\\' {
			escapeNextChar = true
			continue
		}

		if escapeNextChar {
			escapeNextChar = false
			continue
		}

		if strings.HasPrefix(string(runes[i:]), prefix) {
			nestLevel := 0
			if curStackFrame != nil {
				workStack.Push(*curStackFrame)
				nestLevel = curStackFrame.Nested + 1
			}
			curStackFrame = &TokenStackFrame{}
			curStackFrame.StartPos = i
			curStackFrame.Nested = nestLevel
		} else if curStackFrame != nil && strings.HasPrefix(string(runes[i:]), suffix) {
			curStackFrame.EndPos = i + len(suffix) - 1
			curStackFrame.Token = string(runes[curStackFrame.StartPos : curStackFrame.EndPos+1])
			s.Push(*curStackFrame)
			curStackFrame = workStack.Pop()
		} else if curStackFrame == nil {
			s.Extra += string(char)
		}
	}
}

type TokenQuoteState struct {
	InDoubleQuote   bool
	InSingleQuote   bool
	InBacktickQuote bool
}

func (ts *TokenQuoteState) InQuote() bool {
	return ts.InSingleQuote || ts.InDoubleQuote || ts.InBacktickQuote
}

func (ts *TokenQuoteState) IsQuote(char rune) bool {
	return char == '"' || char == '\'' || char == '`'
}

func (ts *TokenQuoteState) SetQuote(char rune) {
	switch char {
	case '"':
		ts.InDoubleQuote = true
	case '\'':
		ts.InSingleQuote = true
	case '`':
		ts.InBacktickQuote = true
	}
}

func (ts *TokenQuoteState) UnsetQuote(char rune) {
	switch char {
	case '"':
		ts.InDoubleQuote = false
	case '\'':
		ts.InSingleQuote = false
	case '`':
		ts.InBacktickQuote = false
	}
}

func isDelimiter(delimiters string, character rune) bool {
	return strings.ContainsRune(delimiters, character)
}

// SplitStringTokens will split an input on any one of the delimiters. However, it will ignore delimiters that
// are within quotes (single, double, or backticks) or delimiters that are escaped with a preceding backslash '\'.
func SplitStringTokens(input string, delimiters string) []string {
	quoteState := TokenQuoteState{}
	tokenStartPos := 0
	escaped := false

	// First split string by words or quoted blocks
	var tokens []string
	sanitizedInput := []rune(strings.TrimSpace(input))

	for i := 0; i < len(sanitizedInput) && tokenStartPos < len(sanitizedInput); i++ {
		char := sanitizedInput[i]

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
		if !quoteState.InQuote() && isDelimiter(delimiters, char) {
			// no +1 on token end to exclude the space delimiter
			t := strings.TrimSpace(string(sanitizedInput[tokenStartPos:i]))
			if t != "" {
				tokens = append(tokens, t)
			}
			// + 1 to exclude the current delimiter
			tokenStartPos = i + 1
		} else if quoteState.InQuote() && isDelimiter(delimiters, char) {
			// if we're in a quote and hit a space, we can ignore it
			continue
		} else if !quoteState.InQuote() && quoteState.IsQuote(char) {
			//if we aren't in a quoted string and we hit a quote, then we can continue
			quoteState.SetQuote(char)
		} else if quoteState.InQuote() && quoteState.IsQuote(char) {
			quoteState.UnsetQuote(char)
		}
	}

	if tokenStartPos < len(sanitizedInput) {
		t := strings.TrimSpace(string(sanitizedInput[tokenStartPos:]))
		if t != "" {
			tokens = append(tokens, t)
		}
	}

	return tokens
}

// PromoteTokenQuotes will 'promote' nested quotes up one level such that the outermost wrapped quotes
// will be removed, and all the nested escaped quotes will have their corresponding escape characters
// removed one nested level.
func PromoteTokenQuotes(tokens []string) []string {
	quoteState := TokenQuoteState{}
	var promotedStrings []string
	for _, s := range tokens {
		newToken := ""
		rs := []rune(s)

		// if the whole token is quoted, then we can remove them and promote the nested quotes
		if quoteState.IsQuote(rs[0]) && quoteState.IsQuote(rs[len(s)-1]) {
			rs = rs[1 : len(rs)-1]
		} else {
			// otherwise, leave the string alone.
			promotedStrings = append(promotedStrings, s)
			continue
		}

		for i := 0; i < len(rs); i++ {
			if rs[i] == '\\' {
				escapeEnd := i + 1
				for ; escapeEnd < len(rs); escapeEnd++ {
					if rs[escapeEnd] != '\\' {
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
			newToken += string(rs[i])
		}
		promotedStrings = append(promotedStrings, newToken)
	}
	return promotedStrings
}
