package arp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// Default built-in handler and validator for responses containing binary data.
type BinaryParser struct {
	Fallback bool
	SavePath string
}

type ByteCountWriter struct {
	ByteCount uint64
}

type BinResponseJson struct {
	Saved     string   `json:"saved"`
	Notice    []string `json:"NOTICE,omitempty"`
	Size      uint64   `json:"size"`
	SHA256Sum string   `json:"sha256sum"`
}

// Implement ResponseHandler
func (bp *BinaryParser) Parse(response *http.Response) (interface{}, interface{}, error) {
	rj, err := getBinaryJson(bp.SavePath, !bp.Fallback, response.Body)
	if err != nil {
		return nil, nil, err
	}
	return rj, nil, nil
}

// Implement ResponseValidator
func (bp *BinaryParser) Validate(test *TestCase, result *TestResult) (bool, []*FieldMatcherResult, error) {
	return test.ResponseMatcher.Match(result.Response)
}

func (bj *BinResponseJson) GenericJSON() map[string]interface{} {
	genericJson := make(map[string]interface{})
	b, _ := json.Marshal(bj)
	json.Unmarshal(b, &genericJson)
	return genericJson
}

func (w *ByteCountWriter) Write(b []byte) (int, error) {
	bytesToWrite := len(b)
	w.ByteCount += uint64(bytesToWrite)
	return bytesToWrite, nil
}

// Convert a binary response into a JSON object that can be used to identify or compare the contents of (at a high level)
func getBinaryJson(savePath string, isExpected bool, response io.Reader) (map[string]interface{}, error) {
	// if we're expecting a binary response, generate a json representation of the data to use with our
	// validation logic
	hasher := sha256.New()
	sizeCounter := &ByteCountWriter{}

	// we want to track how many bytes we're reading from the body
	sizeReader := io.TeeReader(response, sizeCounter)
	// and we want to pipe the output into the hasher as well
	hashReader := io.TeeReader(sizeReader, hasher)
	responseJson := &BinResponseJson{}

	targetPath := savePath
	var file *os.File
	if !isExpected && targetPath == "" {
		f, err := os.CreateTemp("", RESPONSE_PATH_FMT)
		if err != nil {
			return nil, fmt.Errorf("failed to create temporary file: %v", err)
		}
		file = f
	}

	if targetPath != "" {
		f, fErr := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR, 0700)
		if fErr != nil {
			return nil, fmt.Errorf("failed to open file %v while writing response: %v", savePath, fErr)
		}
		file = f
	}

	if file != nil {
		io.Copy(file, hashReader)
		responseJson.Saved = file.Name()
	} else {
		io.ReadAll(hashReader)
	}

	if !isExpected {
		responseJson.Notice = []string{
			"Unexpected non-JSON response was returned from this call triggering a fallback to its binary representation.",
			"Response data has been written to the path in the 'saved' field of this object."}
	}

	responseJson.SHA256Sum = string(hex.EncodeToString(hasher.Sum(nil)))
	responseJson.Size = sizeCounter.ByteCount

	return responseJson.GenericJSON(), nil
}
