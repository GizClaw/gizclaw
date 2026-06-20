package transformers

import (
	"regexp"
	"strings"
)

var (
	ttsXMLTagPattern       = regexp.MustCompile(`</?[A-Za-z][A-Za-z0-9:_-]*(?:\s+[^<>]*)?/?>`)
	ttsXMLTagPartsPattern  = regexp.MustCompile(`(?is)^<\s*(/?)\s*([A-Za-z][A-Za-z0-9:_-]*)([^<>]*?)(/?)\s*>$`)
	ttsBracketURLPattern   = regexp.MustCompile(`(?is)[（(][^()（）]{0,300}(?:https?://|www\.)[^()（）]{0,300}[）)]`)
	ttsBareURLPattern      = regexp.MustCompile(`(?i)\b(?:https?://|www\.)[^\s<>"'，。！？；、）)\]}]+`)
	ttsWhitespacePattern   = regexp.MustCompile(`[ \t\r\n]+`)
	ttsPunctuationReplacer = strings.NewReplacer(
		" ，", "，",
		" 。", "。",
		" ？", "？",
		" ！", "！",
		" ：", "：",
		" ；", "；",
		" 、", "、",
		"( )", "",
		"（ ）", "",
	)
)

func normalizeTTSSpokenText(text string) string {
	if text == "" {
		return ""
	}
	text = removeTTSXMLControlBlocks(text)
	text = ttsBracketURLPattern.ReplaceAllString(text, "")
	text = ttsXMLTagPattern.ReplaceAllString(text, "")
	text = ttsBareURLPattern.ReplaceAllString(text, "")
	text = ttsWhitespacePattern.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)
	text = ttsPunctuationReplacer.Replace(text)
	return strings.TrimSpace(text)
}

type ttsXMLNode struct {
	text    string
	element *ttsXMLElement
}

type ttsXMLElement struct {
	name     string
	children []ttsXMLNode
}

func removeTTSXMLControlBlocks(text string) string {
	root := &ttsXMLElement{}
	stack := []*ttsXMLElement{root}
	last := 0
	for _, loc := range ttsXMLTagPattern.FindAllStringIndex(text, -1) {
		if loc[0] > last {
			stack[len(stack)-1].children = append(stack[len(stack)-1].children, ttsXMLNode{text: text[last:loc[0]]})
		}
		tag := text[loc[0]:loc[1]]
		parts := ttsXMLTagPartsPattern.FindStringSubmatch(tag)
		if len(parts) != 5 {
			last = loc[1]
			continue
		}
		closing := parts[1] == "/"
		name := strings.ToLower(parts[2])
		selfClosing := strings.TrimSpace(parts[4]) == "/" || strings.HasSuffix(strings.TrimSpace(parts[3]), "/")
		switch {
		case selfClosing:
		case closing:
			if len(stack) > 1 && stack[len(stack)-1].name == name {
				stack = stack[:len(stack)-1]
			}
		default:
			element := &ttsXMLElement{name: name}
			stack[len(stack)-1].children = append(stack[len(stack)-1].children, ttsXMLNode{element: element})
			stack = append(stack, element)
		}
		last = loc[1]
	}
	if last < len(text) {
		stack[len(stack)-1].children = append(stack[len(stack)-1].children, ttsXMLNode{text: text[last:]})
	}
	return renderTTSXMLNodes(root.children)
}

func renderTTSXMLNodes(nodes []ttsXMLNode) string {
	var out strings.Builder
	for _, node := range nodes {
		if node.element == nil {
			out.WriteString(node.text)
		}
	}
	return out.String()
}
