package arp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Test Config keys
	CFG_SKIP          = "skip"
	CFG_TAGS          = "tags"
	CFG_RESPONSE_CODE = "code"

	CFG_RESPONSE_TYPE_BIN  = "binary"
	CFG_RESPONSE_TYPE_JSON = "json"
	CFG_RESPONSE_TYPE_HTML = "html"

	// Mime types
	MIME_JSON = "application/json"
	MIME_TEXT = "text/plain"

	//Headers
	HEADER_CONTENT_TYPE = "Content-Type"

	// MISC
	RESPONSE_PATH_FMT = "binary-response-*"

	//DataStore Vars
	DS_WS_CLIENT = "ws"
)

type TestCaseRpcCfg struct {
	Protocol  string `yaml:"protocol"`
	Address   string `yaml:"address"`
	Procedure string `yaml:"procedure"`
}

type TestCaseResponseCfg struct {
	// status code could end up being either a number or an object defining a validation definition
	StatusCode interface{}                 `yaml:"code"`
	Type       string                      `yaml:"type"`
	FilePath   string                      `yaml:"filePath"`
	Payload    interface{}                 `yaml:"payload"`
	Headers    map[interface{}]interface{} `yaml:"headers"`
}

type TestCaseCfg struct {
	Name        string                      `yaml:"name"`
	Description string                      `yaml:"description"`
	ExitOnRun   bool                        `yaml:"exit"`
	Skip        bool                        `yaml:"skip"`
	Input       map[interface{}]interface{} `yaml:"input"`
	FormInput   bool                        `yaml:"formInput"`
	Tags        []string                    `yaml:"tags"`
	Headers     map[interface{}]interface{} `yaml:"headers"`
	Route       string                      `yaml:"route"`
	Method      string                      `yaml:"method"`
	RPC         TestCaseRpcCfg              `yaml:"rpc"`
	Websocket   bool                        `yaml:"websocket"`
	Response    TestCaseResponseCfg         `yaml:"response"`
}

type TestCase struct {
	Config                TestCaseCfg
	IsRPC                 bool
	ResponseHeaderMatcher ResponseMatcher
	StatusCodeMatcher     ResponseMatcher
	ResponseMatcher       ResponseMatcher
	GlobalDataStore       *DataStore
	Tags                  map[string]bool
}

type TestResult struct {
	TestCase        TestCase
	Fields          []*FieldMatcherResult
	Passed          bool
	Response        interface{}
	RawResponse     interface{}
	ResponseHeaders map[string]interface{}
	RequestHeaders  http.Header
	ResolvedRoute   string
	StatusCode      int
	StartTime       time.Time
	EndTime         time.Time
}

type InputReader struct {
	FormWriter *multipart.Writer
	BodyReader io.Reader
	ErrorChan  chan error
}

// tag string can contain 1 or more tags separated by ",". This syntax will OR the tags.
func (t *TestCase) HasTag(tagList string) bool {
	hasTag := false
	tags := strings.Split(tagList, ",")
	for _, tt := range tags {
		tagStr := tt
		negated := false

		if strings.HasPrefix(tt, "!") {
			negated = true
			tagStr = strings.TrimPrefix(tagStr, "!")
		}

		_, hasTag = t.Tags[tagStr]
		if !negated && hasTag {
			return true
		} else if negated && !hasTag {
			return true
		}
	}
	return false
}

func (t *TestCase) LoadConfig(test *TestCaseCfg) error {
	t.ResponseMatcher = NewResponseMatcher(t.GlobalDataStore)
	t.ResponseHeaderMatcher = NewResponseMatcher(t.GlobalDataStore)
	t.StatusCodeMatcher = NewResponseMatcher(t.GlobalDataStore)
	t.Config = *test

	switch t.Config.Response.Type {
	case CFG_RESPONSE_TYPE_JSON, CFG_RESPONSE_TYPE_BIN, CFG_RESPONSE_TYPE_HTML:
	case "":
		t.Config.Response.Type = CFG_RESPONSE_TYPE_JSON
	default:
		return fmt.Errorf("Invalid 'response.type' specified for %v: %v", t.Config.Name, t.Config.Response.Type)
	}

	if t.Config.RPC.Address != "" && t.Config.RPC.Procedure != "" && t.Config.RPC.Protocol != "" {
		t.IsRPC = true
		t.Config.Method = "RPC"
		t.Config.Route = fmt.Sprintf("%v://%v#%v", t.Config.RPC.Protocol, t.Config.RPC.Address, t.Config.RPC.Procedure)
	}

	if t.Config.Websocket {
		t.Config.Method = "WS"
	}

	if t.Config.Method == "" || t.Config.Response.Type == CFG_RESPONSE_TYPE_HTML {
		t.Config.Method = "GET"
	}

	// generate a mapping for tags to improve look up times
	t.Tags = make(map[string]bool)
	for _, tag := range t.Config.Tags {
		t.Tags[tag] = true
	}

	// Start loading our matchers
	sc := t.Config.Response.StatusCode
	if sc != nil {
		keyPath := FieldMatcherPath{
			Keys: []FieldMatcherKey{{Name: CFG_RESPONSE_CODE, RealKey: JsonKey{Name: CFG_RESPONSE_CODE}}},
		}

		if statusMatcher, mOk := sc.(map[interface{}]interface{}); mOk {
			if err := t.StatusCodeMatcher.loadField(sc, statusMatcher, keyPath); err != nil {
				return err
			}
		} else {

			if err := t.StatusCodeMatcher.loadSimplifiedField(sc, sc, keyPath); err != nil {
				return err
			}
		}
	}

	payload := t.Config.Response.Payload
	if payload != nil {
		rootPath := FieldMatcherPath{
			Keys: []FieldMatcherKey{
				{
					Ignore: true,
					Name:   "",
					RealKey: JsonKey{
						Name: "",
					},
				},
			},
		}

		if payloadObj, ok := payload.(map[interface{}]interface{}); ok {
			if err := t.ResponseMatcher.loadField(payload, payloadObj, rootPath); err != nil {
				if err := t.ResponseMatcher.loadObjectFields(payload, payloadObj, rootPath); err != nil {
					return err
				}
				//return err
			}
		} else {
			if err := t.ResponseMatcher.loadSimplifiedField(payload, payload, rootPath); err != nil {
				return err
			}
		}
	}

	respHeaders := t.Config.Response.Headers
	if respHeaders != nil {
		if err := t.ResponseHeaderMatcher.
			loadObjectFields(respHeaders, respHeaders, FieldMatcherPath{}); err != nil {
			return err
		}
	}

	return nil
}

func (t *TestCase) GetTestRoute() (string, error) {
	resolvedRoute, err := t.GlobalDataStore.ExpandVariable(t.Config.Route)
	if err != nil {
		return "", err
	}
	return varToString(resolvedRoute, t.Config.Route), nil
}

func (t *TestCase) GetTestRpcAddr() (string, error) {
	resolvedAddr, err := t.GlobalDataStore.ExpandVariable(t.Config.RPC.Address)
	if err != nil {
		return "", err
	}
	return varToString(resolvedAddr, t.Config.RPC.Address), nil
}

// Returns a new input object with all included variables resolved
func (t *TestCase) GetResolvedTestInput() (interface{}, error) {
	node, err := t.GlobalDataStore.RecursiveResolveVariables(t.Config.Input)
	if err != nil {
		return nil, err
	}

	node, err = RecursiveExecuteCommand(node)
	if err != nil {
		return nil, err
	}

	return node, err
}

func (t *TestCase) GetTestHeaders(inputReader *InputReader) (map[interface{}]interface{}, error) {
	node, err := t.GlobalDataStore.RecursiveResolveVariables(t.Config.Headers)
	if err != nil {
		return nil, err
	}

	node, err = RecursiveExecuteCommand(node)
	if err != nil {
		return nil, err
	}

	headersMap, ok := node.(map[interface{}]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to load headers for test - expected an object")
	}

	if inputReader != nil && t.Config.FormInput {
		headersMap[HEADER_CONTENT_TYPE] = inputReader.FormWriter.FormDataContentType()
	}

	return headersMap, nil
}

func (t *TestCase) StepExecWebsocket(step int, result *TestResult) (passed bool, remaining int, err error) {
	defer func() { result.EndTime = time.Now().UTC() }()
	input, err := t.GetResolvedTestInput()
	if err != nil {
		return false, 0, fmt.Errorf("failed to get test input: %v", err)
	}

	if remaining, err = executeWebSocket(t, result, input, step); err != nil {
		return false, remaining, err
	}
	result.Passed, result.Fields, err = t.ResponseMatcher.Match(result.Response)
	return
}

func (t *TestCase) GetStubbedFailResult(errorMsg string) *TestResult {
	return &TestResult{
		TestCase:  *t,
		StartTime: time.Now().UTC(),
		EndTime:   time.Now().UTC(),
		Fields: []*FieldMatcherResult{
			{
				Error:           errorMsg,
				ObjectKeyPath:   "test.Error",
				Status:          false,
				ShowExtendedMsg: true,
			},
		},
		Passed: false,
	}
}

func (t *TestCase) Execute(testTags []string) (passed bool, result *TestResult, err error) {
	respParser, respValidator := LoadExtensions(nil)

	result = &TestResult{
		TestCase:  *t,
		StartTime: time.Now().UTC(),
	}

	defer func() { result.EndTime = time.Now().UTC() }()

	if t.Config.Skip {
		result.Fields = []*FieldMatcherResult{
			{
				Error:         "Skipping test as configured",
				ObjectKeyPath: fmt.Sprintf("test.%v", CFG_SKIP),
				Status:        true,
			},
		}
		result.Passed = true
		return true, result, nil
	}

	if t.SkipTestOnTags(testTags) {
		result.Fields = []*FieldMatcherResult{
			{
				Error:         fmt.Sprintf("Skipping test - no tags matching the combination of: %v", testTags),
				ObjectKeyPath: fmt.Sprintf("test.%v", CFG_TAGS),
				Status:        true,
			},
		}
		result.Passed = true
		return true, result, nil
	}

	input, err := t.GetResolvedTestInput()
	if err != nil {
		return false, result, fmt.Errorf("failed to get test input: %v", err)
	}

	if t.Config.Websocket {
		if _, err := executeWebSocket(t, result, input, -1); err != nil {
			return false, result, err
		}
	} else if !t.IsRPC {
		if err := executeRest(t, result, respParser, input); err != nil {
			return false, result, err
		}
	} else {
		if err := executeRPC(t, result, input); err != nil {
			return false, result, err
		}
	}

	result.Passed, result.Fields, err = respValidator.Handle(t, result)
	return result.Passed, result, err
}

func (t *TestCase) CloseWebsocket() {
	if wsc, ok := t.GlobalDataStore.Store[DS_WS_CLIENT]; ok {
		c := wsc.(*websocket.Conn)
		c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		c.Close()

		delete(t.GlobalDataStore.Store, DS_WS_CLIENT)
	}
}

func (t *TestCase) GetWebsocketClient() (*websocket.Conn, string, error) {
	route, err := t.GetTestRoute()
	if err != nil {
		return nil, "", fmt.Errorf("failed to determine test route: %v", err)
	}

	// Get the client. If a client was already initialized and connected in this test suite, then re-use that one
	// so that the test suite can preserve its session across multiple test cases. Maybe in the future (if there's demand)
	// it a new flag can be added to the test case as to whether or not the connection should be closed forcing the next
	// test to create a new connection.
	// Otherwise, if no client exists already, we'll create a new one and connect it.
	var client *websocket.Conn
	if prevClient, ok := t.GlobalDataStore.Store[DS_WS_CLIENT]; !ok {
		inputHeaders := http.Header{}

		headers, err := t.GetTestHeaders(nil)
		if err != nil {
			return nil, route, fmt.Errorf("failed to resolve test headers parameter: %v", err)
		}
		for k := range headers {
			key := fmt.Sprintf("%v", k)
			val := headers[k].(string)
			inputHeaders.Set(key, val)
		}

		client, _, err = websocket.DefaultDialer.Dial(route, inputHeaders)
		if err != nil {
			return nil, route, fmt.Errorf("failed to start websocket client: %v", err)
		}
		t.GlobalDataStore.Put(DS_WS_CLIENT, client)
	} else {
		client = prevClient.(*websocket.Conn)
	}

	return client, route, nil
}

func (t *TestCase) GetWebsocketInput(input interface{}) (*WSInput, error) {
	jNode := YamlToJson(input)
	b, err := json.Marshal(&jNode)
	if err != nil {
		return nil, err
	}

	var inputs WSInput
	json.Unmarshal(b, &inputs)
	return &inputs, nil
}

func (t *TestCase) GetRestInput(input interface{}) (*InputReader, error) {

	// if we aren't passing in form input, just provide the input object as JSON
	if !t.Config.FormInput || t.Config.Websocket {
		jsonNode := YamlToJson(input)
		b, err := json.Marshal(jsonNode)
		if err != nil {
			return nil, err
		}
		return &InputReader{BodyReader: bytes.NewReader(b)}, nil
	}

	// Otherwise, take the fields from the input objet and write them as form fields.
	// Files can be identified as arrays of strings to allow for multi-file uploading
	mappedNode, mOk := input.(map[interface{}]interface{})
	if !mOk {
		return nil, fmt.Errorf("failed to read test input - expected test input to be an object")
	}

	// setup our pipe so we can stream our input files from disk rather than loading to memory
	outputReader, outputWriter, _ := os.Pipe()

	inputReader := &InputReader{
		BodyReader: outputReader,
		FormWriter: multipart.NewWriter(outputWriter),
		ErrorChan:  make(chan error),
	}

	// Start our form provider to pipe in form data as it is read
	go func() {
		for k := range mappedNode {
			key := fmt.Sprintf("%v", k)
			switch v := mappedNode[k].(type) {
			default:
				inputReader.FormWriter.WriteField(key, fmt.Sprintf("%v", v))
			case []interface{}:
				for _, f := range v {
					path := f.(string)
					input, err := os.Open(path)
					if err != nil {
						inputReader.ErrorChan <- fmt.Errorf("failed to open file for form input: %v: %v", f, err)
					}

					w, err := inputReader.FormWriter.CreateFormFile(key, filepath.Base(path))
					if err != nil {
						inputReader.ErrorChan <- fmt.Errorf("failed reading file for form input: %v: %v", f, err)
					}

					io.Copy(w, input)
					input.Close()
				}
			}
		}

		outputWriter.Close()
		inputReader.FormWriter.Close()
		inputReader.ErrorChan <- nil
	}()

	return inputReader, nil
}

func (t *TestCase) SkipTestOnTags(testTags []string) bool {
	for _, inTag := range testTags {
		if !t.HasTag(inTag) {
			return true
		}
	}
	return false
}
