package arp

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/rpc"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"golang.org/x/net/html"
)

const (
	WS_ENC_BASE64   = "base64gzip"
	WS_ENC_HEX      = "hex"
	WS_ENC_FILE     = "file"
	WS_ENC_EXTERNAL = "external"
	WS_RESPONSE     = "responses"

	WS_MSG_TEXT = "text"
	WS_MSG_JSON = "json"
	WS_MSG_BIN  = "binary"
)

type WSMessage struct {
	Payload     interface{} `yaml:"payload" json:"payload"`
	Args        []string    `yaml:"args" json:"args"`
	WriteOnly   bool        `yaml:"WriteOnly" json:"WriteOnly"`
	ReadOnly    bool        `yaml:"readOnly" json:"readOnly"`
	Response    string      `yaml:"response" json:"response"`
	MessageType string      `yam:"type" json:"type"`
	Encoding    string      `yaml:"encoding" json:"encoding"`
	FilePath    string      `yaml:"filePath" json:"filePath"`
}

type WSInput struct {
	Requests []WSMessage `yaml:"requests" json:"requests"`
	Close    bool        `yaml:"close" json:"close"`
}

type WsResponseJson struct {
	Responses []map[string]interface{} `json:"responses"`
}

func responseIsBinary(t *TestCase) bool {
	return t.Config.Response.Type == CFG_RESPONSE_TYPE_BIN
}

func responseIsHtml(t *TestCase) bool {
	return t.Config.Response.Type == CFG_RESPONSE_TYPE_HTML
}

func executeRest(test *TestCase, result *TestResult, input interface{}) error {
	client := http.Client{}
	defer client.CloseIdleConnections()

	var request *http.Request
	var response *http.Response
	var route string
	var err error
	var requestInputReader io.Reader = nil
	var requestInput *InputReader = nil

	if strings.ToLower(test.Config.Method) != "get" {
		requestInput, err = test.GetRestInput(input)
		if err != nil {
			return fmt.Errorf("failed to setup test input: %v", err)
		}

		requestInputReader = requestInput.BodyReader
	}

	route, err = test.GetTestRoute()
	if err != nil {
		return fmt.Errorf("failed to determine test route: %v", err)
	}
	result.ResolvedRoute = route

	request, err = http.NewRequest(test.Config.Method, result.ResolvedRoute, requestInputReader)
	if err != nil {
		return fmt.Errorf("failed to initialize http request: %v", err)
	}

	headers, err := test.GetTestHeaders(requestInput)
	if err != nil {
		return fmt.Errorf("failed to resolve test headers parameter: %v", err)
	}

	for k := range headers {
		key := fmt.Sprintf("%v", k)
		val := headers[k].(string)
		request.Header.Set(key, val)
	}

	result.RequestHeaders = request.Header
	response, err = client.Do(request)
	if requestInput != nil && requestInput.ErrorChan != nil {
		if inputErr := <-requestInput.ErrorChan; inputErr != nil {
			return fmt.Errorf("request input failure: %v", inputErr)
		}
	}
	if err != nil {
		return fmt.Errorf("failed to fetch API response: %v", err)
	}
	result.StatusCode = response.StatusCode

	// convert response headers to json for validation
	var responseHeaders map[string]interface{}
	headerData, _ := json.Marshal(&response.Header)
	if err := json.Unmarshal(headerData, &responseHeaders); err != nil {
		return fmt.Errorf("failed to convert response headers: %v\n%v", err, response.Header)
	}
	result.ResponseHeaders = responseHeaders

	var responseJson map[string]interface{}
	fallbackToBinary := false

	if responseIsHtml(test) {
		node, err := html.Parse(response.Body)
		if err != nil {
			return err
		}
		result.HtmlResponse = node
		rj, err := getHtmlJson(node)
		if err != nil {
			return err
		}
		responseJson = rj
	} else if !responseIsBinary(test) && len(response.Header.Values(HEADER_CONTENT_TYPE)) > 0 {
		// expecting JSON response, we can assume (hopefully) that the JSON data will fit in memory
		var responseData []byte
		for _, t := range response.Header.Values(HEADER_CONTENT_TYPE) {
			if strings.Contains(t, MIME_JSON) || strings.Contains(t, MIME_TEXT) {
				var rErr error
				responseData, rErr = ioutil.ReadAll(response.Body)
				if rErr != nil {
					return fmt.Errorf("failed to parse API response: %v", rErr)
				}
				break
			}
		}
		if len(responseData) > 0 {
			if err := json.Unmarshal(responseData, &responseJson); err != nil {
				return fmt.Errorf("failed to unmarshal JSON response: %v", err)
			}
		} else {
			// a content type header was provided and no json response was provided, fallback to binary
			fallbackToBinary = true
		}
	}
	// non-JSON response we'll need to stream from the body
	if responseIsBinary(test) || fallbackToBinary {
		rj, err := getBinaryJson(test.Config.Response.FilePath, !fallbackToBinary, response.Body)
		if err != nil {
			return err
		}
		responseJson = rj
	}

	result.Response = responseJson
	return nil
}

func executeRPC(test *TestCase, result *TestResult, input interface{}) error {
	var client *rpc.Client
	var err error

	addr, err := test.GetTestRpcAddr()
	if err != nil {
		return fmt.Errorf("failed to determine test route: %v", err)
	}
	result.ResolvedRoute = addr

	switch test.Config.RPC.Protocol {
	case "tcp":
		client, err = rpc.Dial("tcp", addr)
	default:
		client, err = rpc.DialHTTP("tcp", addr)
	}

	defer func() {
		if client != nil {
			client.Close()
		}
	}()

	if err != nil {
		return fmt.Errorf("failed to dial rpc client: %v", err)
	}

	var args []byte
	jsonNode := YamlToJson(input)
	b, err := json.Marshal(jsonNode)
	if err != nil {
		return fmt.Errorf("failed to read input for test call: %v", err)
	}
	args = b

	var reply []byte
	err = client.Call(test.Config.RPC.Procedure, args, &reply)
	if err != nil {
		return fmt.Errorf("rpc call failed: %v", err)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(reply, &response); err != nil {
		return fmt.Errorf("failed to unmarshal rcp response: %v", err)
	}

	result.Response = response
	return nil
}

func executeWebSocket(test *TestCase, result *TestResult, input interface{}, step int) (int, error) {
	client, route, err := test.GetWebsocketClient()
	if err != nil {
		return 0, err
	}
	result.ResolvedRoute = route

	inputs, err := test.GetWebsocketInput(input)
	if err != nil {
		return 0, err
	}

	if inputs.Close {
		defer func() {
			test.CloseWebsocket()
		}()
	}

	if result.Response == nil {
		result.Response = make(map[string]interface{})
		result.Response[WS_RESPONSE] = make([]interface{}, 0)
	}

	if step >= 0 && step < len(inputs.Requests) {
		return len(inputs.Requests) - 1 - step, executeWebsoecktRequest(client, &inputs.Requests[step], result)
	}

	for _, ti := range inputs.Requests {
		err := executeWebsoecktRequest(client, &ti, result)
		if err != nil {
			return 0, err
		}
	}

	return 0, nil
}

func executeWebsoecktRequest(client *websocket.Conn, testInput *WSMessage, result *TestResult) error {
	if !testInput.ReadOnly {
		err := writeWebsocketPayload(client, testInput)
		if err != nil {
			//result.Passed = false
			//result.RunError = err
			return err
		}
	}

	if !testInput.WriteOnly {
		var subRespJson map[string]interface{}
		if testInput.Response == "binary" {
			_, responseReader, err := client.NextReader()
			if err != nil {
				return fmt.Errorf("failed to initialze websocket response reader: %v", err)
			}
			subRespJson, _ = getBinaryJson(testInput.FilePath, true, responseReader)
		} else {
			_, responseData, err := client.ReadMessage()
			if err != nil {
				return fmt.Errorf("failed to read websocket response: %v", err)
			}

			if testInput.Response == "json" || testInput.Response == "" {
				if err := json.Unmarshal(responseData, &subRespJson); err != nil {
					subRespJson, _ = getBinaryJson("", false, bytes.NewReader(responseData))
				}
			} else if testInput.Response == "text" {
				subRespJson = make(map[string]interface{})
				subRespJson["payload"] = string(responseData)
			}
		}

		result.Response[WS_RESPONSE] = append(result.Response[WS_RESPONSE].([]interface{}), subRespJson)
	}
	return nil
}

func writeWebsocketPayload(client *websocket.Conn, input *WSMessage) error {
	msType := websocket.TextMessage
	switch input.MessageType {
	case WS_MSG_TEXT:
		fallthrough
	case WS_MSG_JSON:
		msType = websocket.TextMessage
	case WS_MSG_BIN:
		msType = websocket.BinaryMessage
	}

	if msType == websocket.TextMessage {
		var b []byte
		if s, ok := input.Payload.(string); ok {
			b = []byte(s)
		} else {
			marshalled, err := json.Marshal(input.Payload)
			if err != nil {
				return fmt.Errorf("failed to marshal websocket input: %v -> %v", input.Payload, err)
			}
			b = marshalled
		}
		client.WriteMessage(msType, b)
		return nil
	}

	if input.Encoding == "" {
		input.Encoding = WS_ENC_BASE64
	}

	socketWriter, err := client.NextWriter(msType)
	if err != nil {
		return fmt.Errorf("failed to initiate websocket writer: %v", err)
	}

	defer socketWriter.Close()
	var inputReader io.Reader

	var cmd *exec.Cmd
	var wg *sync.WaitGroup
	var cmdErr error
	var cmdStdErr string

	switch input.Encoding {
	case WS_ENC_BASE64:
		base64gz, ok := input.Payload.(string)
		if !ok {
			return fmt.Errorf("websocket payload expected to be base64 gzip - found non-string value instead")
		}

		b64R, err := Base64GzipToByteReader(base64gz)
		if err != nil {
			return fmt.Errorf("websocket payload expected to be base64 encoded gzip - %v", err)
		}
		inputReader = b64R
		defer b64R.Close()
	case WS_ENC_HEX:
		hexInput, ok := input.Payload.(string)
		if !ok {
			return fmt.Errorf("websocket payload expected to be a hex string - found non-string value instead")
		}

		hexReader := hex.NewDecoder(bytes.NewReader([]byte(hexInput)))
		inputReader = hexReader
	case WS_ENC_FILE:
		// stream the file contents through the websocket message
		filepath, ok := input.Payload.(string)
		if !ok {
			return fmt.Errorf("websocket payload expected to be a file path - found non-string value instead")
		}

		fileReader, err := os.Open(filepath)
		if err != nil {
			return fmt.Errorf("failed to open file %v to send via websocket: %v", filepath, err)
		}
		defer fileReader.Close()
		inputReader = fileReader
	case WS_ENC_EXTERNAL:
		cmd = exec.Command(fmt.Sprintf("%v", input.Payload), input.Args...)

		inputReader, err = cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("failed to initialize stdout pipe for extern input: %v: %v", input.Payload, err)
		}
		errPipe, err := cmd.StderrPipe()
		if err != nil {
			return fmt.Errorf("failed to initialize stderr pipe for extern input: %v: %v", input.Payload, err)
		}

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("failed to start external input: %v: %v", input.Payload, err)
		}

		wg = &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, err := ioutil.ReadAll(errPipe)
			if err != nil {
				cmdErr = err
			} else {
				cmdStdErr = string(data)
			}
		}()
	}

	io.Copy(socketWriter, inputReader)
	socketWriter.Close()

	if cmd != nil && wg != nil {
		wg.Wait()
		if err := cmd.Wait(); err != nil {
			return fmt.Errorf("external input failed to execute: %v: %v", err, cmdStdErr)
		}

		if cmdErr != nil {
			return fmt.Errorf("encountered an error while reading stderr for external input: %v", err)
		}
	}

	return nil
}
