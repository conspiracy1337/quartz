package toast

import (
	"strconv"
	"strings"
)

// xmlBuilder writes the toast XML the Windows Runtime consumes, with the
// elements and attributes of upstream's text/template (tmpl/xml.go.tmpl),
// written directly so text/template stays out of the binary. Unlike the
// template it escapes attribute values and splits "]]>" inside CDATA, so no
// field can change the document's structure.
//
// For more information about the xml schema:
// https://learn.microsoft.com/en-us/uwp/schemas/tiles/toastschema/schema-root
type xmlBuilder struct{ strings.Builder }

var attrEscaper = strings.NewReplacer(`&`, "&amp;", `<`, "&lt;", `>`, "&gt;", `"`, "&quot;", `'`, "&apos;")

// open starts an element with its attributes (name, value pairs).
func (b *xmlBuilder) open(name string, attrs ...string) {
	b.WriteByte('<')
	b.WriteString(name)
	for i := 0; i+1 < len(attrs); i += 2 {
		b.WriteByte(' ')
		b.WriteString(attrs[i])
		b.WriteString(`="`)
		_, _ = attrEscaper.WriteString(b, attrs[i+1])
		b.WriteByte('"')
	}
	b.WriteByte('>')
}

// empty writes an element without content.
func (b *xmlBuilder) empty(name string, attrs ...string) {
	b.open(name, attrs...)
	b.close(name)
}

func (b *xmlBuilder) close(name string) {
	b.WriteString("</")
	b.WriteString(name)
	b.WriteByte('>')
}

// cdata writes s as character data that may hold any text.
func (b *xmlBuilder) cdata(s string) {
	b.WriteString("<![CDATA[")
	b.WriteString(strings.ReplaceAll(s, "]]>", "]]]]><![CDATA[>"))
	b.WriteString("]]>")
}

func (n *Notification) buildXML() (string, error) {
	var b xmlBuilder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.open("toast", "activationType", n.ActivationType, "launch", n.ActivationArguments, "duration", n.Duration)
	b.open("visual")
	b.open("binding", "template", "ToastGeneric")
	if n.HeroIcon != "" {
		b.empty("image", "placement", "hero", "src", n.HeroIcon)
	}
	if n.Icon != "" {
		attrs := []string{"placement", "appLogoOverride", "src", n.Icon}
		if n.IconCrop != "" {
			attrs = append(attrs, "hint-crop", n.IconCrop)
		}
		b.empty("image", attrs...)
	}
	if n.Title != "" {
		b.open("text", "hint-maxLines", "1")
		b.cdata(n.Title)
		b.close("text")
	}
	if n.Body != "" {
		b.open("text")
		b.cdata(n.Body)
		b.close("text")
	}
	b.close("binding")
	b.close("visual")
	if n.Audio != Silent {
		b.empty("audio", "src", n.Audio, "loop", strconv.FormatBool(n.Loop))
	} else {
		b.empty("audio", "silent", "true")
	}
	if len(n.Actions) > 0 {
		b.open("actions")
		for _, in := range n.Inputs {
			kind := "text"
			if len(in.Selections) > 0 {
				kind = "selection"
			}
			b.open("input", "id", in.ID, "title", in.Title, "placeHolderContent", in.Placeholder, "type", kind)
			for _, s := range in.Selections {
				b.empty("selection", "id", s.ID, "content", s.Content)
			}
			b.close("input")
		}
		for _, a := range n.Actions {
			attrs := []string{"activationType", a.Type, "content", a.Content, "arguments", a.Arguments}
			if a.InputID != "" {
				attrs = append(attrs, "hint-inputId", a.InputID)
			}
			b.empty("action", attrs...)
		}
		b.close("actions")
	}
	b.close("toast")
	return b.String(), nil
}
