package modrinth

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
	c := New(nc)
	c.Base = srv.URL
	return c
}

const sodiumVersion = `{"id":"v2","project_id":"AANobbMI","version_number":"0.6.5","version_type":"release",
"date_published":"2025-02-01T00:00:00Z","loaders":["fabric"],"game_versions":["1.21.1"],
"files":[{"hashes":{"sha1":"1111111111111111111111111111111111111111","sha512":"aa"},"url":"https://cdn.modrinth.com/a.jar","filename":"sodium.jar","primary":true,"size":5}],
"dependencies":[{"project_id":"P7dR8mSH","dependency_type":"required"},{"project_id":null,"dependency_type":"optional"}]}`

func TestIdentify(t *testing.T) {
	var body map[string]any
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v2/version_files" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		_, _ = io.WriteString(w, `{"aa":`+sodiumVersion+`}`)
	})
	got, err := c.Identify(context.Background(), []modplatform.Hashes{{SHA1: "s1", SHA512: "aa"}, {SHA1: "s2", SHA512: "bb"}})
	if err != nil {
		t.Fatal(err)
	}
	v := got["s1"]
	if len(got) != 1 || v.ProjectID != "AANobbMI" || v.Number != "0.6.5" || v.File.Name != "sodium.jar" || len(v.Deps) != 1 || v.Deps[0].Kind != modplatform.DepRequired {
		t.Fatalf("%+v", got)
	}
	if body["algorithm"] != "sha512" {
		t.Fatalf("%v", body)
	}
}

func TestLatestSendsLoaderGameAndChannel(t *testing.T) {
	var body map[string]any
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		_, _ = io.WriteString(w, `{"aa":`+sodiumVersion+`}`)
	})
	got, err := c.Latest(context.Background(), []modplatform.Installed{{Key: "k", Hashes: modplatform.Hashes{SHA512: "aa"}}}, "fabric",
		modplatform.Query{Game: []string{"1.21.1"}, Channel: modplatform.Beta})
	if err != nil || got["k"].ID != "v2" {
		t.Fatalf("%+v %v", got, err)
	}
	if body["loaders"].([]any)[0] != "fabric" || body["game_versions"].([]any)[0] != "1.21.1" || len(body["version_types"].([]any)) != 2 {
		t.Fatalf("%v", body)
	}
}

func TestProjectsWithAuthors(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/projects":
			_, _ = io.WriteString(w, `[{"id":"AANobbMI","slug":"sodium","title":"Sodium","team":"t1","organization":"o1","categories":["optimization"]},
{"id":"lib0001","slug":"lib","title":"Lib","team":"t2","organization":null,"categories":["library"]}]`)
		case "/v3/organizations":
			_, _ = io.WriteString(w, `[{"id":"o1","name":"CaffeineMC"}]`)
		case "/v2/teams":
			_, _ = io.WriteString(w, `[[{"team_id":"t2","role":"Contributor","user":{"username":"helper"}},{"team_id":"t2","role":"Owner","user":{"username":"boss"}}]]`)
		default:
			t.Errorf("unexpected %s", r.URL)
		}
	})
	got, err := c.Projects(context.Background(), []string{"AANobbMI", "lib0001", "../bad"})
	if err != nil {
		t.Fatal(err)
	}
	if got["AANobbMI"].Author != "CaffeineMC" || got["lib0001"].Author != "boss" || !got["lib0001"].Library || got["AANobbMI"].Slug != "sodium" {
		t.Fatalf("%+v", got)
	}
}

func TestVersionsAndChangelogs(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/project/AANobbMI/version":
			if r.URL.Query().Get("include_changelog") != "false" {
				t.Error("changelogs must not be requested in lists")
			}
			_, _ = io.WriteString(w, `[`+sodiumVersion+`]`)
		case "/v2/versions":
			_, _ = io.WriteString(w, `[{"id":"v2","project_id":"AANobbMI","changelog":"- fix","files":[{"hashes":{}}]}]`)
		}
	})
	vs, err := c.Versions(context.Background(), "AANobbMI", modplatform.Query{Loaders: []string{"fabric"}, Game: []string{"1.21.1"}})
	if err != nil || len(vs) != 1 {
		t.Fatalf("%v %v", vs, err)
	}
	if err := c.Changelogs(context.Background(), vs); err != nil || vs[0].Changelog != "- fix" {
		t.Fatalf("%q %v", vs[0].Changelog, err)
	}
	if vs, _ := c.Versions(context.Background(), "../etc", modplatform.Query{}); vs != nil {
		t.Fatal("invalid ids never reach a URL")
	}
}
