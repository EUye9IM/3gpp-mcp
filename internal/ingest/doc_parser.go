package ingest

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"

	"github.com/3gpp-mcp/3gpp-mcp/internal/model"
)

// DocParser extracts structured sections from a 3GPP .docx file.
type DocParser struct{}

// NewDocParser creates a new DocParser.
func NewDocParser() *DocParser {
	return &DocParser{}
}

// ParseResult holds the output of parsing a .docx file.
type ParseResult struct {
	SpecID   string
	Release  string
	Version  string
	Sections []model.Section
}

// Parse parses a .docx file at the given path.
func (p *DocParser) Parse(docxPath, specID, release string) (*ParseResult, error) {
	r, err := zip.OpenReader(docxPath)
	if err != nil {
		return nil, fmt.Errorf("open docx: %w", err)
	}
	defer r.Close()

	styles := p.readStyles(&r.Reader)

	allParas, err := p.parseDocumentXML(&r.Reader)
	if err != nil {
		return nil, fmt.Errorf("parse document: %w", err)
	}

	paras := p.resolveParagraphs(allParas, styles)

	sections := p.buildSections(paras, specID, release)

	return &ParseResult{
		SpecID:   specID,
		Release:  release,
		Sections: sections,
	}, nil
}

type rawPara struct {
	styleID string
	texts   []string // individual run texts
	text    string   // joined text
}

type resolvedPara struct {
	text  string
	level int // 1-8 for headings, 0 for body
	style string
	runs  []string // individual run texts for heading number extraction
}

func (p *DocParser) readStyles(r *zip.Reader) map[string]string {
	styleMap := make(map[string]string)

	f, err := r.Open("word/styles.xml")
	if err != nil {
		return styleMap
	}
	defer f.Close()

	type styleNameVal struct {
		Val string `xml:"val,attr"`
	}
	type styleDef struct {
		ID   string       `xml:"styleId,attr"`
		Name styleNameVal `xml:"name"`
	}

	var doc struct {
		Styles []styleDef `xml:"style"`
	}

	if err := xml.NewDecoder(f).Decode(&doc); err != nil {
		return styleMap
	}

	for _, s := range doc.Styles {
		if s.Name.Val != "" {
			styleMap[s.ID] = s.Name.Val
		}
	}

	return styleMap
}

func (p *DocParser) parseDocumentXML(r *zip.Reader) ([]rawPara, error) {
	f, err := r.Open("word/document.xml")
	if err != nil {
		return nil, fmt.Errorf("open document.xml: %w", err)
	}
	defer f.Close()

	decoder := xml.NewDecoder(f)

	var paragraphs []rawPara
	var current rawPara
	var runBuf strings.Builder
	var inP, inR, inT bool

	flushRun := func() {
		if runBuf.Len() > 0 {
			current.texts = append(current.texts, runBuf.String())
			runBuf.Reset()
		}
	}

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				inP = true
				current = rawPara{}
			case "pStyle":
				if inP {
					for _, attr := range t.Attr {
						if attr.Name.Local == "val" {
							current.styleID = attr.Value
						}
					}
				}
			case "r":
				inR = true
			case "t":
				inT = true
			}

		case xml.CharData:
			if inP && inR && inT {
				runBuf.WriteString(string(t))
			}

		case xml.EndElement:
			switch t.Name.Local {
			case "p":
				if inP {
					flushRun()
					if current.texts != nil {
						current.text = strings.Join(current.texts, "")
					}
					paragraphs = append(paragraphs, current)
					inP = false
				}
			case "r":
				flushRun()
				inR = false
			case "t":
				inT = false
			}
		}
	}

	return paragraphs, nil
}

func (p *DocParser) resolveParagraphs(raw []rawPara, styles map[string]string) []resolvedPara {
	var paras []resolvedPara
	for _, rp := range raw {
		rp.text = strings.TrimSpace(rp.text)
		if rp.text == "" {
			continue
		}
		styleName := styles[rp.styleID]
		level := headingLevel(styleName)
		paras = append(paras, resolvedPara{
			text:  rp.text,
			level: level,
			style: styleName,
			runs:  rp.texts,
		})
	}
	return paras
}

// splitSectionNumber extracts the section number and title from heading text.
func splitSectionNumber(text string, runs []string) (number, title string) {
	// Regex-first extraction on the full text handles cases where runs are
	// merged (e.g. "5.6.2.1"+"7" makes "5.6.2.17" but runs can't disambiguate).
	if m := sectionNumRx.FindString(text); len(m) > 0 {
		number = normalizeSectionNum(m)
		title = strings.TrimSpace(text[len(m):])
		if title == "" {
			title = number
		}
		return
	}

	var allRuns []string
	for _, r := range runs {
		t := strings.TrimSpace(r)
		if t != "" {
			allRuns = append(allRuns, t)
		}
	}

	if len(allRuns) == 0 {
		return text, text
	}

	// Collect leading runs that look like section-number fragments.
	// A fragment is: digit(s) optionally followed by a letter, optionally
	// with trailing dot(s); or a single letter for annex numbering.
	var numParts []string
	idx := 0
	for idx < len(allRuns) {
		s := allRuns[idx]
		// Accept fragments like "4", "4.", "1.1", "1.3.1", "A"
		if isNumberFragment(s) {
			numParts = append(numParts, s)
			idx++
		} else {
			break
		}
	}

	if len(numParts) > 0 {
		// Group numParts: consecutive bare-digit fragments (no dots) are concatenated
		// into a single level component. e.g. ["5.6.2.", "1", "5"] → "5.6.2.15"
		var groups []string
		var bareBuf strings.Builder
		for _, p := range numParts {
			t := strings.TrimRight(p, ".")
			t = strings.TrimLeft(t, ".")
			if t == "" {
				continue
			}
			hadDot := strings.Contains(p, ".")
			if hadDot {
				if bareBuf.Len() > 0 {
					groups = append(groups, bareBuf.String())
					bareBuf.Reset()
				}
				groups = append(groups, t)
			} else {
				bareBuf.WriteString(t)
			}
		}
		if bareBuf.Len() > 0 {
			groups = append(groups, bareBuf.String())
		}

		var n strings.Builder
		for i, g := range groups {
			if i > 0 {
				n.WriteByte('.')
			}
			n.WriteString(g)
		}
		number = n.String()
		title = strings.TrimSpace(strings.Join(allRuns[idx:], " "))
		if title == "" {
			title = number
		}
		return
	}

	// Fallback: token-based extraction from the raw text.
	// Collect leading space-separated tokens that look like number fragments,
	// normalize them into a section number, and use the remainder as the title.
	parts := strings.Fields(text)
	numEnd := 0
	for numEnd < len(parts) && isNumberFragment(parts[numEnd]) {
		numEnd++
	}
	if numEnd > 0 {
		joined := strings.Join(parts[:numEnd], " ")
		number = normalizeSectionNum(joined)
		title = strings.TrimSpace(strings.Join(parts[numEnd:], " "))
		if title == "" {
			title = number
		}
		return
	}

	number = normalizeSectionNum(text)
	title = text
	return
}

func isNumberFragment(s string) bool {
	// Strip trailing dots (common in spaced numbering like "4. ")
	s = strings.TrimRight(s, ".")
	if s == "" {
		return false
	}
	// Single letter (Annex A, B, etc.)
	if len(s) == 1 && s[0] >= 'A' && s[0] <= 'Z' {
		return true
	}
	// Annex subsection: "A.1", "A.1.2" – letter then dot then digits+subdots
	if len(s) >= 3 && s[0] >= 'A' && s[0] <= 'Z' && s[1] == '.' {
		if numFragmentRE2.MatchString(s[2:]) {
			return true
		}
		return false
	}
	// Fraction continuation: ".1", ".1.2"
	if strings.HasPrefix(s, ".") {
		s = s[1:]
	}
	// Digit(s) optionally with embedded dots: "1", "1.1", "1.1.1"
	if numFragmentRE2.MatchString(s) {
		return true
	}
	return false
}

var numFragmentRE2 = regexp.MustCompile(`^\d+[A-Za-z]?(?:\.\d+[A-Za-z]?)*\.?$`)

var sectionNumRE = regexp.MustCompile(`^[A-Z]?\d+[A-Za-z]?`)

// sectionNumRx extracts a full section number from the beginning of text.
// Allows optional whitespace after dots (for "4. 1.3" → "4.1.3").
// Handles: "5.6.2.17Type:", "A.1General", "4.1.1Overview", "4. 1.3 Network Functions".
var sectionNumRx = regexp.MustCompile(`^(?:[A-Z]\s*\.\s*)?(?:\d+\s*\.\s*)*\d+`)

// normalizeSectionNum collapses spaces and stray dots within a section number.
func normalizeSectionNum(s string) string {
	s = strings.TrimSpace(s)
	// Replace " ." and ". " with "." to collapse spaced-out numbers
	s = strings.ReplaceAll(s, ". ", ".")
	s = strings.ReplaceAll(s, " .", ".")
	// Collapse adjacent dots
	for strings.Contains(s, "..") {
		s = strings.ReplaceAll(s, "..", ".")
	}
	// Collapse remaining whitespace-separated digit groups
	parts := strings.Fields(s)
	if len(parts) <= 1 {
		return strings.TrimSuffix(s, ".")
	}

	allFrags := true
	for _, p := range parts {
		if !isNumberFragment(strings.TrimRight(p, ".")) {
			allFrags = false
			break
		}
	}

	if allFrags {
		var result strings.Builder
		partsJoined := make([]string, 0, len(parts))
		bareBuf := strings.Builder{}
		for _, p := range parts {
			trimmed := strings.TrimRight(p, ".")
			if trimmed == "" {
				continue
			}
			isBare := !strings.ContainsAny(trimmed, ".")
			if isBare {
				bareBuf.WriteString(trimmed)
			} else {
				if bareBuf.Len() > 0 {
					partsJoined = append(partsJoined, bareBuf.String())
					bareBuf.Reset()
				}
				partsJoined = append(partsJoined, trimmed)
			}
		}
		if bareBuf.Len() > 0 {
			partsJoined = append(partsJoined, bareBuf.String())
			bareBuf.Reset()
		}
		for i, p := range partsJoined {
			if i > 0 {
				result.WriteByte('.')
			}
			result.WriteString(p)
		}
		return result.String()
	}

	return strings.TrimSuffix(s, ".")
}

// parentSectionNumber extracts the parent from "5.3.7" → "5.3".
func parentSectionNumber(number string) string {
	idx := strings.LastIndexByte(number, '.')
	if idx < 0 {
		// Could be a letter-based section like "A" under an Annex
		return ""
	}
	return number[:idx]
}

var annexNumRE = regexp.MustCompile(`^Annex\s+([A-Z])\s`)

// buildSections converts resolved paragraphs into a flat section list.
func (p *DocParser) buildSections(paras []resolvedPara, specID, release string) []model.Section {
	var sections []model.Section
	var currentSection *model.Section
	var contentBuf strings.Builder
	var nestingStack []int

	flushContent := func() {
		if currentSection != nil {
			currentSection.Content = strings.TrimSpace(contentBuf.String())
			contentBuf.Reset()
		}
	}

	for _, para := range paras {
		if para.level > 0 {
			flushContent()

			num, title := splitSectionNumber(para.text, para.runs)
			if num == "" || num == para.text {
				// No structured section number found (e.g., "Foreword")
				num = para.text
				title = para.text
			}

			parentNum := parentSectionNumber(num)

			// Handle Annex: "Annex A (normative):OpenAPI specification" → num="A"
			if annexNumRE.MatchString(para.text) {
				m := annexNumRE.FindStringSubmatch(para.text)
				if m != nil {
					num = m[1]
					title = strings.TrimSpace(para.text[len(m[0]):])
					if title == "" {
						title = para.text
					}
				}
			}

			// Pop nesting stack to find correct parent
			for len(nestingStack) > 0 && para.level <= nestingStack[len(nestingStack)-1] {
				nestingStack = nestingStack[:len(nestingStack)-1]
			}

			section := model.Section{
				SpecID:        specID,
				Release:       release,
				SectionNumber: num,
				ParentNumber:  parentNum,
				Title:         title,
			}
			sections = append(sections, section)
			currentSection = &sections[len(sections)-1]
			nestingStack = append(nestingStack, para.level)
		} else if currentSection != nil {
			contentBuf.WriteString(para.text)
			contentBuf.WriteString("\n\n")
		}
	}

	flushContent()
	return sections
}

func headingLevel(styleName string) int {
	lower := strings.ToLower(styleName)
	if !strings.HasPrefix(lower, "heading ") {
		return 0
	}
	n := lower[len("heading "):]
	switch n {
	case "1":
		return 1
	case "2":
		return 2
	case "3":
		return 3
	case "4":
		return 4
	case "5":
		return 5
	case "6":
		return 6
	case "7":
		return 7
	case "8":
		return 8
	}
	return 0
}
