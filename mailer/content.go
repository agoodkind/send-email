package mailer

import (
	"fmt"
	"html/template"
	"strings"
)

// ContentKind selects the rendering of a caller-ordered body block.
type ContentKind string

const (
	// ContentText renders escaped text in HTML and unescaped text in plain mail.
	ContentText ContentKind = "text"
	// ContentHTML renders trusted markup with an explicit plain-text alternative.
	ContentHTML ContentKind = "html"
	// ContentTable renders an escaped table and its plain-text representation.
	ContentTable ContentKind = "table"
)

// ContentBlock is one body block. HTML is trusted caller-owned markup; Text
// supplies its required plain-text alternative. Table cells and text are escaped.
type ContentBlock struct {
	Kind  ContentKind
	Text  string
	HTML  string
	Table *Table
}

type htmlContentBlock struct {
	Lines []string
	HTML  template.HTML
	Table *Table
}

func validateContent(msg Message) error {
	if msg.Content == nil {
		return nil
	}
	if msg.Body != "" || msg.HTML != "" || len(msg.Tables) != 0 {
		return fmt.Errorf("ordered content cannot be combined with Body, HTML, or Tables")
	}
	for index, block := range msg.Content {
		switch block.Kind {
		case ContentText:
			if block.HTML != "" || block.Table != nil {
				return fmt.Errorf("content block %d: text cannot include HTML or Table", index)
			}
		case ContentHTML:
			if strings.TrimSpace(block.Text) == "" || strings.TrimSpace(block.HTML) == "" || block.Table != nil {
				return fmt.Errorf("content block %d: HTML requires markup and a text alternative without Table", index)
			}
		case ContentTable:
			if block.Table == nil || block.Text != "" || block.HTML != "" {
				return fmt.Errorf("content block %d: table requires Table without Text or HTML", index)
			}
		default:
			return fmt.Errorf("content block %d: unknown kind %q", index, block.Kind)
		}
	}
	return nil
}

func renderContentText(content []ContentBlock) string {
	parts := make([]string, 0, len(content))
	for _, block := range content {
		if block.Kind == ContentTable {
			parts = append(parts, strings.TrimPrefix(formatTextTables("", []Table{*block.Table}), "\n"))
		} else {
			parts = append(parts, RenderPlain(block.Text))
		}
	}
	return strings.Join(parts, "\n\n")
}

func contentHTMLBlocks(content []ContentBlock) []htmlContentBlock {
	blocks := make([]htmlContentBlock, 0, len(content))
	for _, block := range content {
		converted := htmlContentBlock{Lines: nil, HTML: "", Table: nil}
		switch block.Kind {
		case ContentText:
			converted.Lines = strings.Split(RenderPlain(block.Text), "\n")
		case ContentHTML:
			// #nosec G203 -- ContentHTML is documented as trusted caller markup.
			converted.HTML = template.HTML(block.HTML)
		case ContentTable:
			converted.Table = block.Table
		}
		blocks = append(blocks, converted)
	}
	return blocks
}
