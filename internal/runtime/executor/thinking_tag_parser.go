package executor

import (
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	thinkingStartTag = "<thinking>"
	thinkingEndTag   = "</thinking>"
	maxTagBufferLen  = 12
)

var thinkingStartPartials = []string{
	"<thinking",
	"<thinkin",
	"<thinki",
	"<think",
	"<thin",
	"<thi",
	"<th",
	"<t",
	"<",
}

var thinkingEndPartials = []string{
	"</thinking",
	"</thinkin",
	"</thinki",
	"</think",
	"</thin",
	"</thi",
	"</th",
	"</t",
	"</",
}

type ThinkingTagParser struct {
	inThinking bool
	tagBuffer  string
	active     bool
}

func NewThinkingTagParser(modelName string) *ThinkingTagParser {
	return &ThinkingTagParser{
		active: strings.Contains(strings.ToLower(modelName), "claude"),
	}
}

type thinkingTextSegment struct {
	text    string
	thought bool
}

func (p *ThinkingTagParser) Process(payload []byte) []byte {
	if !p.active {
		return payload
	}
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return payload
	}

	partsResult := gjson.GetBytes(payload, "response.candidates.0.content.parts")
	if !partsResult.Exists() || !partsResult.IsArray() {
		return payload
	}

	parts := partsResult.Array()
	if len(parts) == 0 {
		return payload
	}

	updatedParts := make([]string, 0, len(parts))
	changed := false

	for _, part := range parts {
		if !part.Get("text").Exists() {
			updatedParts = append(updatedParts, part.Raw)
			continue
		}
		if part.Get("functionCall").Exists() || part.Get("inlineData").Exists() || part.Get("inline_data").Exists() {
			updatedParts = append(updatedParts, part.Raw)
			continue
		}

		originalText := part.Get("text").String()
		text := originalText
		if p.tagBuffer != "" {
			text = p.tagBuffer + text
			p.tagBuffer = ""
		}

		segments := p.splitThinkingText(text)
		if len(segments) == 0 {
			changed = true
			continue
		}

		if len(segments) == 1 {
			updated := part.Raw
			if segments[0].text != originalText {
				updated, _ = sjson.Set(updated, "text", segments[0].text)
			}
			if segments[0].thought && !part.Get("thought").Bool() {
				updated, _ = sjson.Set(updated, "thought", true)
			}
			if updated != part.Raw {
				changed = true
			}
			updatedParts = append(updatedParts, updated)
			continue
		}

		changed = true
		for _, segment := range segments {
			if segment.text == "" {
				continue
			}
			partJSON := `{}`
			partJSON, _ = sjson.Set(partJSON, "text", segment.text)
			if segment.thought {
				partJSON, _ = sjson.Set(partJSON, "thought", true)
			}
			updatedParts = append(updatedParts, partJSON)
		}
	}

	if !changed {
		return payload
	}

	partsJSON := "[" + strings.Join(updatedParts, ",") + "]"
	updated, err := sjson.SetRawBytes(payload, "response.candidates.0.content.parts", []byte(partsJSON))
	if err != nil {
		return payload
	}
	return updated
}

func (p *ThinkingTagParser) splitThinkingText(text string) []thinkingTextSegment {
	if text == "" {
		return []thinkingTextSegment{{text: "", thought: p.inThinking}}
	}

	segments := make([]thinkingTextSegment, 0, 2)
	remaining := text

	for len(remaining) > 0 {
		if p.inThinking {
			endIdx := strings.Index(remaining, thinkingEndTag)
			if endIdx >= 0 {
				if endIdx > 0 {
					segments = append(segments, thinkingTextSegment{text: remaining[:endIdx], thought: true})
				}
				remaining = remaining[endIdx+len(thinkingEndTag):]
				p.inThinking = false
				continue
			}

			trimmed, buffer := splitTrailingPartialTag(remaining, thinkingEndPartials)
			if buffer != "" {
				p.tagBuffer = buffer
			}
			if trimmed != "" {
				segments = append(segments, thinkingTextSegment{text: trimmed, thought: true})
			}
			remaining = ""
			continue
		}

		startIdx := strings.Index(remaining, thinkingStartTag)
		if startIdx >= 0 {
			if startIdx > 0 {
				segments = append(segments, thinkingTextSegment{text: remaining[:startIdx], thought: false})
			}
			remaining = remaining[startIdx+len(thinkingStartTag):]
			p.inThinking = true
			continue
		}

		trimmed, buffer := splitTrailingPartialTag(remaining, thinkingStartPartials)
		if buffer != "" {
			p.tagBuffer = buffer
		}
		if trimmed != "" {
			segments = append(segments, thinkingTextSegment{text: trimmed, thought: false})
		}
		remaining = ""
	}

	return segments
}

func splitTrailingPartialTag(text string, partials []string) (string, string) {
	for _, partial := range partials {
		if strings.HasSuffix(text, partial) {
			if len(partial) > maxTagBufferLen {
				return text, ""
			}
			return text[:len(text)-len(partial)], partial
		}
	}
	return text, ""
}
