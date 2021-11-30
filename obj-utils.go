package arp

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

const (
	JSON_OBJECT_DELIM      = "."
	JSON_INDEX_START_DELIM = "["
	JSON_INDEX_END_DELIM   = "]"
	JSON_INDEX_DELIM       = JSON_INDEX_START_DELIM + JSON_INDEX_END_DELIM
	JSON_RESERVED_CHARS    = JSON_OBJECT_DELIM + JSON_INDEX_DELIM
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

func JsonToYaml(i interface{}) interface{} {
	switch x := i.(type) {
	case map[string]interface{}:
		m2 := map[interface{}]interface{}{}
		for k, v := range x {
			m2[k] = JsonToYaml(v)
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

type JsonKey struct {
	Name           string
	IsArray        bool
	IsArrayElement bool
	IsLast         bool
	IsObject       bool
}

// GetJsonPath Returns a string representation of a series of json keys that make up a path to a value
// in a json object. The maxDepth provides a limit on how deep into the path structure to construct since
// the key object count to json path node count is not always 1-1 (e.g. array[index] is split as 2 keys but
// only counts as a single node in the path)
// The remaining unprocessed keys are returned along with the string representation of the processed keys.
func GetJsonPath(keys []JsonKey, maxDepth int) (string, []JsonKey) {
	pathStr := ""
	nodeCount := 0
	remainingKeys := keys
	for i, k := range keys {
		remainingKeys = keys[i:]
		if nodeCount >= maxDepth {
			return pathStr, remainingKeys
		}

		if k.IsArrayElement {
			pathStr += fmt.Sprintf("[%v]", k.Name)
			nodeCount++
		} else if k.IsObject {
			pathStr += fmt.Sprintf(".%v", k.Name)
		} else {
			pathStr += "." + k.Name
			nodeCount++
		}
	}

	return pathStr, remainingKeys
}

func sanitizeQuotedIndex(key string) string {
	cleaned := key
	for _, c := range "\"`'" {
		cleaned = strings.TrimPrefix(cleaned, string(c))
		cleaned = strings.TrimSuffix(cleaned, string(c))
	}
	return cleaned
}

// SplitJsonPath Splits a string formatted as a JSON accessor into its individual keys
// with metadata. E.g. "data.someArray[1].value" -> "[data, someArray, 1, value]"
func SplitJsonPath(jsonPath string) []JsonKey {
	keys := SplitStringTokens(jsonPath, JSON_OBJECT_DELIM)

	// Extract array indexing from the keys as their own key for iterating the datastore.
	var expandedKeys []JsonKey
	for _, k := range keys {
		foundBrackets := false
		// now that we've split out the tokens, we can remove any quotes surrounding keys
		keyStrs := PromoteTokenQuotes(SplitStringTokens(k, JSON_INDEX_DELIM))
		if len(keyStrs) > 0 {
			foundBrackets = true

			for _, ks := range keyStrs {
				var toAdd JsonKey
				// test if it's a number
				if _, err := strconv.ParseInt(ks, 10, 64); err == nil {
					toAdd = JsonKey{Name: ks, IsArrayElement: true}
					if len(expandedKeys) > 0 {
						// mark the previous key as an array
						expandedKeys[len(expandedKeys)-1].IsArray = true
					}
				} else {
					// otherwise its an object key
					toAdd = JsonKey{Name: sanitizeQuotedIndex(ks)}
				}
				expandedKeys = append(expandedKeys, toAdd)
			}
		}
		// if no brackets were found then we can use the entire key
		if !foundBrackets {
			expandedKeys = append(expandedKeys, JsonKey{Name: k})
		}
	}
	expandedKeys[len(expandedKeys)-1].IsLast = true
	return expandedKeys
}

// PutJsonValue Insert an arbitrary value at a desired jsonPath. If the intermediary
// objects/arrays don't exist, they will be created.
func PutJsonValue(dest map[string]interface{}, jsonPath string, value interface{}) error {
	type Noodle struct {
		Parent      interface{}
		Node        interface{}
		ParentKey   string
		ParentIndex int64
	}

	expandedKeys := SplitJsonPath(jsonPath)
	node := Noodle{
		Node:   dest,
		Parent: dest,
	}

	for _, k := range expandedKeys {
		key := k.Name
		var temp interface{}
		switch v := node.Node.(type) {
		case map[string]interface{}:
			if nextNode, ok := v[key]; !ok {
				if k.IsLast {
					v[key] = value
					return nil
				} else if k.IsArray {
					temp = make([]interface{}, 1)
				} else {
					temp = make(map[string]interface{})
				}
				v[key] = temp
				node = Noodle{
					Node:        temp,
					Parent:      &v,
					ParentKey:   key,
					ParentIndex: -1,
				}
			} else {
				// otherwise overwrite existing ones
				if k.IsLast {
					v[key] = value
					return nil
				}
				node = Noodle{
					Node:        nextNode,
					Parent:      &v,
					ParentKey:   key,
					ParentIndex: -1,
				}
			}
		case []interface{}:
			idx, err := strconv.ParseUint(key, 10, 64)
			if err != nil {
				return fmt.Errorf(BadIndexDSFmt, jsonPath)
			}
			// if the index is out of range, then we'll resize the array just enough to fix the index
			if idx > uint64(len(v)) {
				newArray := v[:]
				delta := (idx - uint64(len(v))) + 1
				for delta > 0 {
					delta--
					newArray = append(newArray, nil)

				}
				if node.ParentKey != "" {
					n := node.Parent.(*map[string]interface{})
					(*n)[node.ParentKey] = newArray
				} else if node.ParentIndex >= 0 {
					n := node.Parent.(*[]interface{})
					(*n)[node.ParentIndex] = newArray
				}
				v = newArray
			}
			if v[idx] == nil {
				if k.IsLast {
					v[idx] = value
					return nil
				} else if k.IsArray {
					temp = make([]interface{}, 1)
				} else {
					temp = make(map[string]interface{})
				}
				v[idx] = temp
			} else {
				if k.IsLast {
					v[idx] = value
					return nil
				}
			}
			node = Noodle{
				Node:        v[idx],
				Parent:      &v,
				ParentIndex: int64(idx),
			}
		}
	}
	return nil
}

func GetJsonValue(src map[string]interface{}, jsonPath string) (interface{}, error) {
	expandedKeys := SplitJsonPath(jsonPath)
	var node interface{}
	node = src
	for _, k := range expandedKeys {
		key := k.Name
		switch v := node.(type) {
		case map[string]interface{}:
			if nextNode, ok := v[key]; !ok {
				return "", fmt.Errorf(MissingDSKeyFmt, jsonPath)
			} else {
				node = nextNode
			}
		case []interface{}:
			idx, err := strconv.ParseUint(key, 10, 64)
			if err != nil {
				// should catch non integer and negative value
				return "", fmt.Errorf(BadIndexDSFmt, jsonPath)
			}
			if idx > uint64(len(v)) {
				return "", fmt.Errorf(IndexExceedsDSFmt, jsonPath)
			}

			node = v[idx]
		default:
			return "", fmt.Errorf(MissingDSKeyFmt, jsonPath)
		}
	}

	return node, nil
}
