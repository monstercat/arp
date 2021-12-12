package arp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
)

type ExecutableMatcher struct {
	ReturnCode *int
	Cmd        string
	BinPath    string
	PrgmArgs   []string
	ErrorStr   string
	Exists     bool
	DSName     string
	Priority   int
}

func (m *ExecutableMatcher) Parse(parentNode interface{}, node map[interface{}]interface{}) error {
	// expected return value of the programs execution
	if code, ok := node[TEST_EXEC_KEY_RETURN_CODE]; ok {
		if codeInt, cOk := code.(int); cOk {
			m.ReturnCode = &codeInt
		} else {
			return errors.New(ObjectPrintf(fmt.Sprintf(MalformedDefinitionFmt, TEST_EXEC_KEY_RETURN_CODE, TYPE_INT), parentNode))
		}
	}

	// One-liner command (same as with dynamic inputs)
	if cmdStr, ok := node[TEST_EXEC_KEY_CMD]; ok {
		if s, sOk := cmdStr.(string); sOk {
			m.Cmd = s
			fmt.Printf("Got command: %v\n", m.Cmd)
		} else {
			return errors.New(ObjectPrintf(fmt.Sprintf(MalformedDefinitionFmt, TEST_EXEC_KEY_CMD, TYPE_STR), parentNode))
		}
	} else {
		//Otherwise, if no cmd string is provided, fall back to the split binary argument syntax
		// path of the program to execute
		if binPath, ok := node[TEST_EXEC_KEY_BIN_PATH]; ok {
			if p, pOk := binPath.(string); pOk {
				m.BinPath = p
			} else {
				return errors.New(ObjectPrintf(fmt.Sprintf(MalformedDefinitionFmt, TEST_EXEC_KEY_BIN_PATH, TYPE_STR), parentNode))
			}
		}

		// collect the arguments to run
		if prgmArgs, ok := node[TEST_EXEC_KEY_ARGS]; ok {
			if args, aOk := prgmArgs.([]interface{}); aOk {
				for _, a := range args {
					if curArg, cAOk := a.(string); cAOk {
						m.PrgmArgs = append(m.PrgmArgs, curArg)
					} else {
						return errors.New(ObjectPrintf(fmt.Sprintf(MalformedDefinitionFmt, TEST_EXEC_KEY_ARGS, TYPE_STR), parentNode))
					}
				}
			} else {
				return errors.New(ObjectPrintf(fmt.Sprintf(MalformedDefinitionFmt, TEST_EXEC_KEY_ARGS, TYPE_ARRAY), parentNode))
			}
		}
	}

	m.DSName = getDataStoreName(node)
	m.Priority = getMatcherPriority(node)

	var err error
	m.Exists, err = getExistsFlag(node)
	return err
}

func (m *ExecutableMatcher) Match(responseValue interface{}, datastore *DataStore) (bool, DataStore, error) {
	store := NewDataStore()
	m.ErrorStr = ""
	if status, passthrough, message := handleExistence(responseValue, m.Exists, false); !passthrough {
		m.ErrorStr = message
		return status, store, nil
	}
	// expect all inputs to be formatted as a string to pass as an input to the program
	typedResponseValue := fmt.Sprintf("%v", responseValue)

	// immediately store value into datastore so it can be resolved as a variable for program inputs
	if m.DSName != "" {
		if err := (*datastore).PutVariable(m.DSName, typedResponseValue); err != nil {
			return false, store, err
		}
	}

	var status bool

	if m.Cmd == "" {
		resolvedBinPath, err := datastore.ExpandVariable(m.BinPath)
		if err != nil {
			return false, store, fmt.Errorf(BadVarMatcherFmt, m.BinPath)
		}

		// resolve variables in the program
		resolvedArgs, argErr := datastore.RecursiveResolveVariables(m.PrgmArgs)
		if argErr != nil {
			return false, store, fmt.Errorf(BadVarMatcherFmt, m.PrgmArgs)
		}

		argArray, aOk := resolvedArgs.([]interface{})
		if !aOk {
			m.ErrorStr = fmt.Sprintf(MismatchedMatcher, TYPE_ARRAY, reflect.TypeOf(resolvedArgs))
			return false, store, nil
		}

		var argStrings []string
		for _, aA := range argArray {
			if s, isStr := aA.(string); isStr {
				argStrings = append(argStrings, s)
			} else {
				b, _ := json.Marshal(aA)
				argStrings = append(argStrings, string(b))
			}
		}

		status := true
		cmd := exec.Command(resolvedBinPath.(string), argStrings...)

		result, err := cmd.CombinedOutput()
		sanitizedResult := string(result)

		if m.ReturnCode != nil {
			status = *m.ReturnCode == cmd.ProcessState.ExitCode()
		}

		if !status && err != nil {
			m.ErrorStr = fmt.Sprintf("[%v]\n %v", err.Error(), sanitizedResult)
			status = false
		} else {
			m.ErrorStr = sanitizedResult
		}

	} else {
		resolvedCmd, err := datastore.ExpandVariable(m.Cmd)
		if err != nil {
			return false, store, fmt.Errorf(BadVarMatcherFmt, m.Cmd)
		}
		status = true
		result, err := ExecuteCommand(resolvedCmd.(string))
		if err != nil {
			status = false
			m.ErrorStr = fmt.Sprintf("[%v]\n %v", err, result)
		} else {
			m.ErrorStr = strings.TrimSpace(result.(string))
			if m.ErrorStr == "" {
				m.ErrorStr = "[status 0]"
			}
		}
	}

	return status, store, nil
}

func (m *ExecutableMatcher) Error() string {
	return m.ErrorStr
}

func (m *ExecutableMatcher) GetPriority() int {
	return m.Priority
}

func (m *ExecutableMatcher) SetError(error string) {
	m.ErrorStr = error
}
