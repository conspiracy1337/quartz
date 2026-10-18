package toast

import (
	"encoding/xml"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"text/template"
)

// upstreamXML is go-toast v2.0.3's tmpl/xml.go.tmpl, which buildXML
// replaces. It lives in the test only, so text/template never reaches a
// program that uses this package.
const upstreamXML = `<?xml version="1.0" encoding="UTF-8"?>
<toast activationType="{{.ActivationType}}" launch="{{.ActivationArguments}}" duration="{{.Duration}}">
    <visual>
        <binding template="ToastGeneric">
            {{if .HeroIcon}}
            <image placement="hero" src="{{.HeroIcon}}" />
            {{end}}
            {{if .Icon}}
            <image placement="appLogoOverride" src="{{.Icon}}" {{if .IconCrop}} hint-crop="{{.IconCrop}}" {{end}} />
            {{end}}
            {{if .Title}}
            <text hint-maxLines="1"><![CDATA[{{.Title}}]]></text>
            {{end}}
            {{if .Body}}
            <text><![CDATA[{{.Body}}]]></text>
            {{end}}
        </binding>
    </visual>
    {{if ne .Audio "silent"}}
	<audio src="{{.Audio}}" loop="{{.Loop}}" />
	{{else}}
	<audio silent="true" />
	{{end}}
    {{if .Actions}}
    <actions>
        {{range .Inputs}}
        <input id="{{.ID}}" title="{{.Title}}" placeHolderContent="{{.Placeholder}}" {{if .Selections}} type="selection" {{else}} type="text" {{end}}>
            {{range .Selections}}
            <selection id="{{.ID}}" content="{{.Content}}" />
            {{end}}
        </input>
        {{end}}
        {{range .Actions}}
        <action activationType="{{.Type}}" content="{{.Content}}" arguments="{{.Arguments}}" {{if .InputID}} hint-inputId="{{.InputID}}" {{end}} />
        {{end}}
    </actions>
    {{end}}
</toast>`

// tokens reads an XML document as its elements, attributes and non-blank
// text: what the Windows Runtime sees, without the template's indentation.
func tokens(t *testing.T, doc string) []string {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(doc))
	var out []string
	text := ""
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("%v in\n%s", err, doc)
		}
		if v, ok := tok.(xml.CharData); ok {
			text += string(v) // a split CDATA section is one text
			continue
		}
		if s := strings.TrimSpace(text); s != "" {
			out = append(out, "text:"+s)
		}
		text = ""
		switch v := tok.(type) {
		case xml.StartElement:
			s := "<" + v.Name.Local
			for _, a := range v.Attr {
				s += " " + a.Name.Local + "=" + a.Value
			}
			out = append(out, s)
		case xml.EndElement:
			out = append(out, "</"+v.Name.Local)
		case xml.ProcInst:
			out = append(out, "?"+v.Target+" "+string(v.Inst))
		}
	}
}

var samples = []Notification{
	{},
	{AppID: "Quartz", Title: "Hello", Body: "Line one\nLine two"},
	{Title: "Only title", Audio: Silent, Duration: Long, ActivationArguments: "open"},
	{Body: "Only body", Icon: `C:\icons\a.png`, IconCrop: CropStyleCircle, HeroIcon: `C:\hero.png`, Loop: true, Audio: LoopingAlarm},
	{Icon: `C:\icons\b.png`, Title: "Inputs", Inputs: []Input{
		{ID: "reply", Title: "Reply", Placeholder: "Type"},
		{ID: "pick", Title: "Pick", Selections: []InputSelection{{ID: "a", Content: "A"}, {ID: "b", Content: "B"}}},
	}, Actions: []Action{
		{Type: Foreground, Content: "Send", Arguments: "send", InputID: "reply"},
		{Type: Protocol, Content: "Open", Arguments: "https://example.com/"},
	}},
	// Inputs without actions are not written (as upstream).
	{Title: "No actions", Inputs: []Input{{ID: "x"}}},
}

func TestBuildXMLMatchesUpstream(t *testing.T) {
	tpl := template.Must(template.New("toast-xml").Parse(upstreamXML))
	for i, n := range samples {
		n.applyDefaults()
		var want strings.Builder
		if err := tpl.Execute(&want, &n); err != nil {
			t.Fatal(err)
		}
		got, err := n.buildXML()
		if err != nil {
			t.Fatal(err)
		}
		if g, w := tokens(t, got), tokens(t, want.String()); !reflect.DeepEqual(g, w) {
			t.Errorf("sample %d:\n got %q\nwant %q", i, g, w)
		}
	}
}

func TestBuildXMLEscapes(t *testing.T) {
	n := Notification{
		Title:               "a ]]> <b>&",
		Body:                "]]><script/>",
		ActivationArguments: `x" injected="1`,
		Actions:             []Action{{Type: Protocol, Content: `<"'&>`, Arguments: "y"}},
	}
	n.applyDefaults()
	got, err := n.buildXML()
	if err != nil {
		t.Fatal(err)
	}
	toks := tokens(t, got)
	want := []string{"<toast activationType=foreground launch=x\" injected=\"1 duration=short"}
	if toks[1] != want[0] {
		t.Fatalf("toast element %q", toks[1])
	}
	for _, s := range []string{"text:a ]]> <b>&", "text:]]><script/>", `<action activationType=protocol content=<"'&> arguments=y`} {
		found := false
		for _, tok := range toks {
			found = found || tok == s
		}
		if !found {
			t.Errorf("%q missing from %q", s, toks)
		}
	}
}
