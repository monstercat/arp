package arp

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

// Default built-in response handler and validator for JSON rest APIs
type JSONParser struct{}

// Implement ResponseHandler
func (jp *JSONParser) Parse(response *http.Response) (map[string]interface{}, interface{}, error) {
	headers := response.Header
	body := response.Body
	// expecting JSON response, we can assume (hopefully) that the JSON data will fit in memory
	var responseJson map[string]interface{}
	var responseData []byte
	for _, t := range headers.Values(HEADER_CONTENT_TYPE) {
		if strings.Contains(t, MIME_JSON) || strings.Contains(t, MIME_TEXT) {
			var rErr error
			responseData, rErr = ioutil.ReadAll(body)
			if rErr != nil {
				return nil, nil, fmt.Errorf("failed to parse API response: %v", rErr)
			}
			break
		}
	}
	if len(responseData) > 0 {
		if err := json.Unmarshal(responseData, &responseJson); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal JSON response: %v", err)
		}
	} else {
		// a content type header was provided and no json response was provided, fallback to binary
		return nil, nil, InvalidContentType
	}

	return responseJson, nil, nil
}

// Implement ResponseValidator
func (jp *JSONParser) Validate(test *TestCase, result *TestResult) (bool, []*FieldMatcherResult, error) {
	statusCode := result.StatusCode
	response := result.Response
	headers := result.ResponseHeaders

	var newResults = []*FieldMatcherResult{}

	// Validate status code
	sPassed, sResult, sErr := test.StatusCodeMatcher.Match(map[string]interface{}{
		CFG_RESPONSE_CODE: statusCode,
	})
	if sErr != nil {
		return false, sResult, sErr
	}
	for _, sR := range sResult {
		sR.ObjectKeyPath = StatusCodePath
		newResults = append(newResults, sR)
	}

	// Validate Response Data
	var status bool
	var results []*FieldMatcherResult
	var err error
	status, results, err = test.ResponseMatcher.Match(response)

	if err != nil {
		return false, results, err
	}
	newResults = append(newResults, results...)

	// Validate response headers
	headerStatus, headerResults, headerErr := test.ResponseHeaderMatcher.Match(headers)
	if headerErr != nil {
		return false, headerResults, headerErr
	}
	for _, hR := range headerResults {
		hR.ObjectKeyPath = HeadersPath + hR.ObjectKeyPath
		newResults = append(newResults, hR)
	}
	// Wrap things up
	if status && headerStatus && sPassed {
		for k := range test.ResponseMatcher.DS.Store {
			test.GlobalDataStore.Put(k, test.ResponseMatcher.DS.Get(k))
		}
	}
	return status && headerStatus && sPassed, newResults, nil
}
