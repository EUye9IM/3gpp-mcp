package ingest

import (
	"strings"
	"testing"

	"github.com/3gpp-mcp/3gpp-mcp/internal/model"
)

func TestDocParser_Parse_29508(t *testing.T) {
	docxPath := "/tmp/3gpp_test/29508-k00.docx"

	p := NewDocParser()
	result, err := p.Parse(docxPath, "29.508", "Rel-20")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if result.SpecID != "29.508" {
		t.Errorf("specID: want 29.508, got %s", result.SpecID)
	}

	if len(result.Sections) < 50 {
		t.Errorf("too few sections: %d", len(result.Sections))
	}

	// Verify section 4.1 exists with correct parent
	var sec41 *model.Section
	for i := range result.Sections {
		s := &result.Sections[i]
		if s.SectionNumber == "4.1" {
			sec41 = s
			break
		}
	}
	if sec41 == nil {
		t.Fatal("section 4.1 not found")
	}
	if sec41.ParentNumber != "4" {
		t.Errorf("4.1 parent: want 4, got %s", sec41.ParentNumber)
	}
	if !strings.Contains(sec41.Title, "Service Description") {
		t.Errorf("4.1 title: want 'Service Description', got %q", sec41.Title)
	}

	// Verify a leaf section has content
	var sec411 *model.Section
	for i := range result.Sections {
		s := &result.Sections[i]
		if s.SectionNumber == "4.1.1" {
			sec411 = s
			break
		}
	}
	if sec411 == nil {
		t.Fatal("section 4.1.1 not found")
	}
	if len(sec411.Content) < 100 {
		t.Errorf("4.1.1 content too short: %d chars", len(sec411.Content))
	}
}

func TestSplitSectionNumber(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		runs      []string
		wantNum   string
		wantTitle string
	}{
		{
			name:      "simple section",
			text:      "5.3.7 RRC Reestablishment",
			runs:      []string{"5.3.7 ", "RRC Reestablishment"},
			wantNum:   "5.3.7",
			wantTitle: "RRC Reestablishment",
		},
		{
			name:      "section without space delimiter",
			text:      "3.1Definitions",
			runs:      []string{"3.1", "Definitions"},
			wantNum:   "3.1",
			wantTitle: "Definitions",
		},
		{
			name:      "foreword (no number)",
			text:      "Foreword",
			runs:      []string{"Foreword"},
			wantNum:   "Foreword",
			wantTitle: "Foreword",
		},
		{
			name:      "spaced number 4. 1.3",
			text:      "4. 1.3 Network Functions",
			runs:      []string{"4.", "1.3", "Network Functions"},
			wantNum:   "4.1.3",
			wantTitle: "Network Functions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNum, gotTitle := splitSectionNumber(tt.text, tt.runs)
			if gotNum != tt.wantNum {
				t.Errorf("number: want %q, got %q", tt.wantNum, gotNum)
			}
			if gotTitle != tt.wantTitle {
				t.Errorf("title: want %q, got %q", tt.wantTitle, gotTitle)
			}
		})
	}
}

func TestNormalizeSectionNum(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"4.1", "4.1"},
		{"4. 1.3", "4.1.3"},
		{"5 .1", "5.1"},
		{"4.", "4"},
		{"5..6", "5.6"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeSectionNum(tt.input)
			if got != tt.want {
				t.Errorf("want %q, got %q", tt.want, got)
			}
		})
	}
}
