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

// Parse Extracts tokens that are wrapped between a predetermined prefix and suffix
// tokens are stored in the order from the inner-most nested out
func (s *TokenStack) Parse(input string, prefix string, suffix string) {
	workStack := TokenStack{}
	var curStackFrame *TokenStackFrame
	for i := 0; i < len(input); i++ {
		char := input[i]
		if strings.HasPrefix(input[i:], prefix) {
			nestLevel := 0
			if curStackFrame != nil {
				workStack.Push(*curStackFrame)
				nestLevel = curStackFrame.Nested + 1
			}
			curStackFrame = &TokenStackFrame{}
			curStackFrame.StartPos = i
			curStackFrame.Nested = nestLevel
		} else if curStackFrame != nil && strings.HasPrefix(input[i:], suffix) {
			curStackFrame.EndPos = i + len(suffix) - 1
			curStackFrame.Token = input[curStackFrame.StartPos : curStackFrame.EndPos+1]
			s.Push(*curStackFrame)
			curStackFrame = workStack.Pop()
		} else if curStackFrame == nil {
			s.Extra += string(char)
		}
	}
}
