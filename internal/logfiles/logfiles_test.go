package logfiles

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quartz/internal/content"
	"quartz/internal/core/logging"
	"quartz/internal/core/netx"
	"quartz/internal/instance"
	"quartz/internal/launch"
)

type fakeRuns struct{ dir string }

func (f fakeRuns) Runs(string) []launch.Record {
	return []launch.Record{{ID: "20250321-193340.000", LengthMin: 3, ExitCode: 1, Result: launch.ResultCrash, Java: "21.0.5", Crash: &launch.Crash{Title: "Sodium failed to load"}}}
}
func (f fakeRuns) Run(id, runID string) (launch.Record, error) { return f.Runs(id)[0], nil }
func (f fakeRuns) Evidence(_, runID, kind string) (string, error) {
	p, err := launch.EvidencePath(f.dir, runID, kind)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(p); err != nil {
		return "", err
	}
	return p, nil
}

func setup(t *testing.T, upload http.HandlerFunc) (*Service, string, string) {
	t.Helper()
	dir := t.TempDir()
	insts, _ := instance.Open(filepath.Join(dir, "instances"), nil)
	in, _ := insts.Create(instance.CreateRequest{Name: "FO", Minecraft: "1.21.4"})
	game := insts.GameDir(in.ID)
	_ = os.MkdirAll(filepath.Join(game, "logs"), 0o700)
	_ = os.MkdirAll(filepath.Join(game, "crash-reports"), 0o700)
	_ = os.MkdirAll(insts.RunsDir(in.ID), 0o700)
	srv := httptest.NewTLSServer(upload)
	t.Cleanup(srv.Close)
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	hc, err := netx.New(netx.Proxy{Mode: netx.ProxyNone}, netx.Options{TLS: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}})
	if err != nil {
		t.Fatal(err)
	}
	svc := New(Deps{
		Instances: insts, Runs: fakeRuns{insts.RunsDir(in.ID)}, HTTP: hc, Redactor: logging.NewRedactor("steve"),
		Names: func() []string { return []string{"Kestrel_Vane"} },
		Mods:  func(context.Context, string) []content.Row { return []content.Row{{Name: "Sodium", Version: "0.6.5", Enabled: true}, {Name: "Iris", Version: "1.8", Enabled: false}} },
		uploadURL: srv.URL + "/1/log",
	})
	return svc, in.ID, game
}

func TestFilesAndText(t *testing.T) {
	svc, id, game := setup(t, nil)
	_ = os.WriteFile(filepath.Join(game, "logs", "latest.log"), []byte("[21:14:03] [main/INFO]: Setting user: Kestrel_Vane\n/home/steve/.minecraft\n"), 0o600)
	gz, _ := os.Create(filepath.Join(game, "logs", "2025-03-22-1.log.gz"))
	zw := gzip.NewWriter(gz)
	_, _ = zw.Write([]byte("old line\n"))
	_ = zw.Close()
	_ = gz.Close()
	old := time.Now().Add(-40 * 24 * time.Hour)
	_ = os.Chtimes(filepath.Join(game, "logs", "2025-03-22-1.log.gz"), old, old)
	_ = os.WriteFile(filepath.Join(game, "crash-reports", "crash-2025-03-21_19.33.40-client.txt"), []byte("boom"), 0o600)
	_ = os.WriteFile(filepath.Join(game, "logs", "notes.txt"), []byte("x"), 0o600)

	files := svc.Files(id)
	if len(files) != 3 || files[0].Name != "latest.log" || !files[2].Crash {
		t.Fatalf("%+v", files)
	}
	text, err := svc.Text(id, "latest.log")
	if err != nil || strings.Contains(text, "/home/steve") {
		t.Fatalf("%q %v", text, err)
	}
	if text, _ := svc.Text(id, "2025-03-22-1.log.gz"); text != "old line\n" {
		t.Fatalf("gz %q", text)
	}
	for _, bad := range []string{"../options.txt", "notes.txt", "/etc/passwd", "crash-../../x.txt"} {
		if _, err := svc.Text(id, bad); !errors.Is(err, ErrNoFile) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if svc.OldSize(id) == 0 || svc.OldSize("") != svc.OldSize(id) {
		t.Fatal("old size")
	}
	if svc.DeleteOld("") == 0 || len(svc.Files(id)) != 2 {
		t.Fatal("old logs not deleted")
	}
}

func TestUploadRedacts(t *testing.T) {
	var got string
	svc, id, game := setup(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got = r.Form.Get("content")
		_, _ = w.Write([]byte(`{"success":true,"id":"abc","url":"https://mclo.gs/abc"}`))
	})
	_ = os.WriteFile(filepath.Join(game, "logs", "latest.log"), []byte("Setting user: Kestrel_Vane\nkestrel_vane joined\nemail me@example.com\n"), 0o600)
	u, err := svc.UploadFile(context.Background(), id, "latest.log")
	if err != nil || u != "https://mclo.gs/abc" {
		t.Fatalf("%q %v", u, err)
	}
	if strings.Contains(strings.ToLower(got), "kestrel_vane") || strings.Contains(got, "me@example.com") {
		t.Fatalf("uploaded unredacted: %q", got)
	}
	info, err := svc.DebugInfo(context.Background(), id, "")
	if err != nil || !strings.Contains(info, "Mods: 2, 1 on") || !strings.Contains(info, "Crash: Sodium failed to load") || !strings.Contains(info, "Iris 1.8 (off)") {
		t.Fatalf("%s %v", info, err)
	}
}

func TestUploadRefusesOddAnswers(t *testing.T) {
	svc, id, game := setup(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"error":"too big"}`))
	})
	_ = os.WriteFile(filepath.Join(game, "logs", "latest.log"), []byte("x"), 0o600)
	if _, err := svc.UploadFile(context.Background(), id, "latest.log"); !errors.Is(err, ErrUpload) {
		t.Fatalf("%v", err)
	}
}
