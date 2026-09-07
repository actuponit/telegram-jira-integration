package jira

import "strings"

// buildADF renders plain text into a minimal Atlassian Document Format
// document: one paragraph per blank-line-separated block, with single
// newlines inside a block rendered as hardBreak nodes.
func buildADF(text string) map[string]any {
	blocks := strings.Split(text, "\n\n")
	content := make([]map[string]any, 0, len(blocks))

	for _, block := range blocks {
		lines := strings.Split(block, "\n")
		paragraphContent := make([]map[string]any, 0, len(lines)*2-1)
		for i, line := range lines {
			if i > 0 {
				paragraphContent = append(paragraphContent, map[string]any{"type": "hardBreak"})
			}
			if line != "" {
				paragraphContent = append(paragraphContent, map[string]any{"type": "text", "text": line})
			}
		}
		if len(paragraphContent) == 0 {
			// ADF text nodes require non-empty text, but a paragraph still
			// needs >=1 inline child; hardBreak has no text requirement.
			paragraphContent = append(paragraphContent, map[string]any{"type": "hardBreak"})
		}
		content = append(content, map[string]any{
			"type":    "paragraph",
			"content": paragraphContent,
		})
	}

	return map[string]any{
		"type":    "doc",
		"version": 1,
		"content": content,
	}
}
