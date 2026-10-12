package curseforge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"quartz/internal/core/netx"
	"quartz/internal/modplatform"
)

func newClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	nc, err := netx.New(netx.Proxy{Mode: netx.ProxyNone}, netx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	c := New(nc, func() bool { return true })
	c.Base = srv.URL
	return c
}

const file = `{"id":3509043,"modId":327154,"displayName":"Screenshot to Clipboard 1.0.7","fileName":"stc.jar","releaseType":1,
"fileDate":"2021-11-01T00:00:00Z","fileLength":15066,"downloadUrl":"https://edge.forgecdn.net/files/3509/43/stc.jar",
"gameVersions":["1.18","Fabric","Client"],"fileFingerprint":1781245820,"hashes":[{"value":"760B3C0BE40A3860BEE1860BBEE92CF25109E484","algo":1}],
"dependencies":[{"modId":306612,"relationType":3},{"modId":1,"relationType":4}]}`

func TestIdentifyByFingerprint(t *testing.T) {
	var body map[string]any
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		_, _ = io.WriteString(w, `{"data":{"exactMatches":[{"file":`+file+`}]}}`)
	})
	got, err := c.Identify(context.Background(), []modplatform.Hashes{
		{SHA1: "760b3c0be40a3860bee1860bbee92cf25109e484", Murmur2: 1781245820},
		{SHA1: "ffffffffffffffffffffffffffffffffffffffff", Murmur2: 1781245820}, // same fingerprint, other bytes
	})
	if err != nil {
		t.Fatal(err)
	}
	v := got["760b3c0be40a3860bee1860bbee92cf25109e484"]
	if len(got) != 1 || v.ProjectID != "327154" || v.ID != "3509043" || v.Type != "release" ||
		len(v.Loaders) != 1 || v.Loaders[0] != "fabric" || len(v.Game) != 1 || len(v.Deps) != 1 {
		t.Fatalf("%+v", got)
	}
	if body["fingerprints"].([]any)[0].(float64) != 1781245820 {
		t.Fatalf("%v", body)
	}
}

func TestLatestFiltersChannelAndLoader(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("modLoaderType") != "4" || r.URL.Query().Get("gameVersion") != "1.21.1" {
			t.Errorf("query %v", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"data":[
{"id":3,"modId":7,"displayName":"beta","releaseType":2,"fileDate":"2025-03-01T00:00:00Z","gameVersions":["1.21.1","Fabric"]},
{"id":2,"modId":7,"displayName":"forge build","releaseType":1,"fileDate":"2025-02-15T00:00:00Z","gameVersions":["1.21.1","Forge"]},
{"id":1,"modId":7,"displayName":"good","releaseType":1,"fileDate":"2025-02-01T00:00:00Z","gameVersions":["1.21.1","Fabric"]}]}`)
	})
	got, err := c.Latest(context.Background(), []modplatform.Installed{{Key: "k", ProjectID: "7"}}, "fabric",
		modplatform.Query{Game: []string{"1.21.1"}, Channel: modplatform.Release})
	if err != nil || got["k"].ID != "1" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestProjectsAndChangelog(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/mods":
			_, _ = io.WriteString(w, `{"data":[{"id":327154,"name":"Screenshot to Clipboard","slug":"screenshot-to-clipboard","authors":[{"name":"Rick"}]}]}`)
		case "/v1/mods/327154/files/3509043/changelog":
			_, _ = io.WriteString(w, `{"data":"<ul><li>First</li></ul>"}`)
		default:
			t.Errorf("%s", r.URL)
		}
	})
	ps, err := c.Projects(context.Background(), []string{"327154", "nope"})
	if err != nil || ps["327154"].Author != "Rick" || ps["327154"].Slug != "screenshot-to-clipboard" {
		t.Fatalf("%+v %v", ps, err)
	}
	vs := []modplatform.Version{{ProjectID: "327154", ID: "3509043"}, {ProjectID: "../x", ID: "1"}}
	if err := c.Changelogs(context.Background(), vs); err != nil || vs[0].Changelog != "<ul><li>First</li></ul>" || vs[1].Changelog != "" {
		t.Fatalf("%+v %v", vs, err)
	}
}
