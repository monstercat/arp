package arp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"golang.org/x/net/html"
)

type BinResponseJson struct {
	Saved     string   `json:"saved"`
	Notice    []string `json:"NOTICE,omitempty"`
	Size      uint64   `json:"size"`
	SHA256Sum string   `json:"sha256sum"`
}

func (bj *BinResponseJson) GenericJSON() map[string]interface{} {
	genericJson := make(map[string]interface{})
	b, _ := json.Marshal(bj)
	json.Unmarshal(b, &genericJson)
	return genericJson
}

type ByteCountWriter struct {
	ByteCount uint64
}

type HtmlResponseJson struct {
	Tag        string              `json:"tag"`
	Content    string              `json:"content"`
	Attributes map[string]string   `json:"attributes"`
	Children   []*HtmlResponseJson `json:"children"`
	Siblings   []*HtmlResponseJson `json:"siblings"`
}

func (hj *HtmlResponseJson) GenericJSON() map[string]interface{} {
	genericJson := make(map[string]interface{})
	b, _ := json.Marshal(hj)
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

// Convert an HTML Node response into a nicer JSON representation
func getHtmlJson(node *html.Node) (map[string]interface{}, error) {
	type queuedNode struct {
		Node      *html.Node
		Target    *HtmlResponseJson
		IsSibling bool
		IsChild   bool
	}

	var headNode *HtmlResponseJson
	nodeQueue := []*queuedNode{
		{Node: node, Target: nil},
	}

	for len(nodeQueue) > 0 {
		curNode := nodeQueue[0]
		nodeQueue = nodeQueue[1:]

		curHtmlNode, promote := htmlNodeToJson(curNode.Node)
		var nextTarget *HtmlResponseJson
		if headNode == nil && curNode.Target == nil && curNode.IsChild == false && curNode.IsSibling == false {
			headNode = curHtmlNode
			curNode.Target = headNode
		} else if curNode.IsSibling && !promote {
			curNode.Target.Siblings = append(curNode.Target.Siblings, curHtmlNode)
		} else if curNode.IsChild && !promote {
			curNode.Target.Children = append(curNode.Target.Children, curHtmlNode)
		}

		if promote {
			curNode.Target.Content = curHtmlNode.Content
		}

		if curNode.Node.NextSibling != nil {
			// leave target as our original nodes target since we are appending at the same level of our tree
			nextTarget = curNode.Target
			nodeQueue = append(nodeQueue, &queuedNode{
				Node:      curNode.Node.NextSibling,
				Target:    nextTarget,
				IsSibling: true,
			})
		}

		if curNode.Node.FirstChild != nil {
			// make target the current node since it will be the new parent
			if promote {
				nextTarget = curNode.Target
			} else {
				nextTarget = curHtmlNode
			}

			nodeQueue = append(nodeQueue, &queuedNode{
				Node:    curNode.Node.FirstChild,
				Target:  nextTarget,
				IsChild: true,
			})
		}
	}

	return headNode.GenericJSON(), nil
}

func htmlNodeToJson(curNode *html.Node) (*HtmlResponseJson, bool) {
	if curNode == nil {
		return nil, false
	}

	promote := false

	htmlNode := HtmlResponseJson{}
	t := curNode.DataAtom.String()
	if t == "" {
		htmlNode.Tag = curNode.Data
	} else {
		htmlNode.Tag = t
	}

	// try to clean up the output a bit by removing redundant child nodes
	if curNode.Type == html.TextNode {
		promote = true
	}

	htmlNode.Attributes = make(map[string]string)

	for _, a := range curNode.Attr {
		htmlNode.Attributes[a.Key] = a.Val
	}

	return &htmlNode, promote
}
