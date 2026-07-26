package ingest

import (
	"strings"
	"testing"
)

func TestContentQualityCheck(t *testing.T) {
	docxPath := "/tmp/3gpp_validate/29508-k00.docx"
	p := NewDocParser()
	result, err := p.Parse(docxPath, "29.508", "Rel-20")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	totalChars := 0
	contentCount := 0
	for _, s := range result.Sections {
		if len(s.Content) > 0 {
			totalChars += len(s.Content)
			contentCount++
		}
	}

	t.Logf("=== Content Statistics ===")
	t.Logf("Sections: %d total, %d with content, %d empty (parent/container)",
		len(result.Sections), contentCount, len(result.Sections)-contentCount)
	t.Logf("Total content chars: %d", totalChars)

	// Short content details
	t.Logf("\n=== Sections with short content (<100 chars) ===")
	shortCount := 0
	for _, s := range result.Sections {
		if len(s.Content) > 0 && len(s.Content) < 100 {
			t.Logf("  %-16s %3d chars  %s", s.SectionNumber, len(s.Content), s.Title)
			if len(s.Content) > 0 {
				t.Logf("    content: %q", strings.TrimSpace(s.Content))
			}
			shortCount++
		}
	}
	if shortCount == 0 {
		t.Logf("  (none)")
	}

	// Check for truncation: sections ending without natural punctuation
	t.Logf("\n=== Sections without terminal punctuation ===")
	punctRunes := []rune{'.', '!', '?', ':', ')', '}', ']', '"', '\u201d', '\u2019'}
	noPunctCount := 0
	for _, s := range result.Sections {
		if len(s.Content) > 0 {
			trimmed := strings.TrimSpace(s.Content)
			if len(trimmed) == 0 {
				continue
			}
			hasPunct := false
			runes := []rune(trimmed)
			lastR := runes[len(runes)-1]
			for _, p := range punctRunes {
				if lastR == p {
					hasPunct = true
					break
				}
			}
			if !hasPunct {
				tail := trimmed
				if len(tail) > 120 {
					tail = "..." + tail[len(tail)-120:]
				}
				t.Logf("  %-16s %5d chars  %q", s.SectionNumber, len(s.Content), tail)
				noPunctCount++
			}
		}
	}
	if noPunctCount == 0 {
		t.Logf("  (none)")
	}

	// Formatting artifacts
	t.Logf("\n=== Formatting artifact check ===")
	xmlCount := 0
	nbSpaceCount := 0
	for _, s := range result.Sections {
		if strings.Contains(s.Content, "<w:") || strings.Contains(s.Content, "</w:") {
			t.Errorf("XML artifact in section %s", s.SectionNumber)
			xmlCount++
		}
		nbSpaceCount += strings.Count(s.Content, "\u00a0")
	}
	if xmlCount == 0 {
		t.Logf("XML artifacts: 0")
	}
	t.Logf("Non-breaking spaces (\\u00a0): %d (normal in 3GPP for spec references like '3GPP\u00a0TS\u00a023.502')", nbSpaceCount)

	// Verify key sections have substantial content
	t.Run("content_depth", func(t *testing.T) {
		tests := []struct {
			section string
			minLen  int
		}{
			{"1", 300},
			{"4.1.1", 500},
			{"4.2.2.2", 5000},
			{"5.6.2.5", 5000},
			{"A.2", 5000},
		}
		for _, tt := range tests {
			var found string
			for _, s := range result.Sections {
				if s.SectionNumber == tt.section {
					found = s.Content
					break
				}
			}
			if found == "" {
				t.Errorf("%s: not found", tt.section)
			} else if len(found) < tt.minLen {
				t.Errorf("%s: %d chars < expected min %d", tt.section, len(found), tt.minLen)
			} else {
				t.Logf("%s: %d chars >= %d OK", tt.section, len(found), tt.minLen)
			}
		}
	})

	// Check for consecutive empty newlines (formatting issue)
	t.Run("no_double_empty_lines", func(t *testing.T) {
		for _, s := range result.Sections {
			if strings.Contains(s.Content, "\n\n\n\n") {
				t.Logf("  %s: has 4+ consecutive newlines", s.SectionNumber)
			}
		}
	})
}
