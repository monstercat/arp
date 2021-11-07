package arp

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"gopkg.in/yaml.v2"
)

func YamlToJson(i interface{}) interface{} {
	switch x := i.(type) {
	case map[interface{}]interface{}:
		m2 := map[string]interface{}{}
		for k, v := range x {
			m2[k.(string)] = YamlToJson(v)
		}
		return m2
	case []interface{}:
		for i, v := range x {
			x[i] = YamlToJson(v)
		}
	}
	return i
}

func PrintYamlObj(object interface{}) (string, error) {
	bytes, err := yaml.Marshal(object)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func ObjectPrintf(message string, obj interface{}) string {
	objStr, _ := PrintYamlObj(obj)
	return fmt.Sprintf("%v:\n---\n%v---\n", message, objStr)
}

func ToJsonObj(obj interface{}) (map[string]interface{}, error) {
	b, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	var r map[string]interface{}
	err = json.Unmarshal(b, &r)
	if err != nil {
		return nil, err
	}

	return r, nil
}

func Base64GzipToByteReader(input string) (io.ReadCloser, error) {

	gzipB, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 encoded string found")
	}

	gzipR, err := gzip.NewReader(bytes.NewReader(gzipB))
	if err != nil {
		return nil, fmt.Errorf(" base64 encoded string was not gzip compressed.")
	}

	return gzipR, nil
}
