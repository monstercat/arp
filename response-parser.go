package arp

import (
	"errors"
	"fmt"
	"net/http"
)

var (
	InvalidContentType = errors.New("Invalid Content Type, falling back to binary")
)

type ResponseParser interface {
	Parse(response *http.Response) (map[string]interface{}, interface{}, error)
}

type ResponseParserHandler map[string]ResponseParser

func (rh *ResponseParserHandler) Register(responseType string, handler ResponseParser) {
	(*rh)[responseType] = handler
}

func (rh *ResponseParserHandler) LoadDefaults() {
	(*rh) = make(map[string]ResponseParser)

	rh.Register("json", &JSONParser{})
	rh.Register("binary", &BinaryParser{})
}

func (rh *ResponseParserHandler) Handle(test *TestCase, response *http.Response) (map[string]interface{}, interface{}, error) {
	responseType := test.Config.Response.Type

	parser, exists := (*rh)[responseType]
	if !exists {
		return nil, nil, fmt.Errorf("No response parser defined for type \"%v\"", responseType)
	}

	js, raw, err := parser.Parse(response)
	if err == InvalidContentType {
		// binary parser should always be available as a fallback option for unsupported/unexpected
		// data types
		fallbackParser := BinaryParser{
			Fallback: true,
			SavePath: test.Config.Response.FilePath,
		}

		return fallbackParser.Parse(response)
	}
	return js, raw, err
}
