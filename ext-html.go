package arp

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

type HtmlExt struct{}

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

// Implement ResponseHandler
func (hp *HtmlExt) Parse(response *http.Response) (interface{}, interface{}, error) {
	node, err := html.Parse(response.Body)
	if err != nil {
		return nil, nil, InvalidContentType
	}
	rj, err := getHtmlJson(node)
	if err != nil {
		return nil, nil, err
	}

	return rj, node, err
}

// Implement ResponseValidator
func (hp *HtmlExt) Validate(test *TestCase, result *TestResult) (bool, []*FieldMatcherResult, error) {
	response := result.RawResponse
	rMatcher := test.ResponseMatcher

	var docReader *goquery.Document
	if v, ok := response.(*html.Node); ok {
		docReader = goquery.NewDocumentFromNode(v)
	}

	// HTML processor uses goquery to use jquery like selectors for locating nodes.
	// Each nested query selector will be applied to the results of the previous selector.
	processor := func(matcher *FieldMatcherConfig, response interface{}) ResponseMatcherResults {
		var curSelection *goquery.Selection
		return rMatcher.MatchConfig(matcher, response, func(p FieldMatcherKey) interface{} {
			var resultNode interface{}
			key := p.RealKey
			if strings.HasPrefix(key.Name, "<") && strings.HasSuffix(key.Name, ">") {
				newKey := strings.TrimPrefix(key.Name, "<")
				newKey = strings.TrimSuffix(newKey, ">")
				newKey = strings.TrimSpace(newKey)
				if curSelection == nil {
					curSelection = docReader.Find(newKey)
				} else {
					curSelection = curSelection.Find(newKey)
				}

				if len(curSelection.Nodes) == 1 {
					htmlNode, _ := getHtmlJson(curSelection.Nodes[0])
					resultNode = htmlNode
				} else if len(curSelection.Nodes) > 1 {
					selectionRoot := html.Node{}
					curSelectNode := &selectionRoot
					for _, cNode := range curSelection.Nodes {
						(*curSelectNode).NextSibling = cNode
						curSelectNode = cNode
					}
					resultNode, _ = getHtmlJson(&selectionRoot)
				}
			}
			return resultNode
		})
	}

	return rMatcher.MatchBase(response, processor)
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
