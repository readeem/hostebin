package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/hostebin/hostebin/internal/store"
)

func testServer(t *testing.T, maxUpload int64, maxFiles int) (*store.Store, *httptest.Server) {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(Config{Store: st, Token: "test-token", MaxUpload: maxUpload, MaxFiles: maxFiles})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(app.Handler())
	t.Cleanup(ts.Close)
	return st, ts
}

func rawUpload(t *testing.T, ts *httptest.Server, name, content, token string) (*http.Response, uploadResponse) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/bundles", strings.NewReader(content))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Hostebin-Filename", name)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var result uploadResponse
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	return resp, result
}

func TestRawUploadFetchAndHeaders(t *testing.T) {
	_, ts := testServer(t, 1024, 4)
	resp, result := rawUpload(t, ts, "index.html", "<h1>Hello</h1>", "test-token")
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload = %d: %s", resp.StatusCode, body)
	}
	if result.URL != ts.URL+"/b/"+result.ID+"/" {
		t.Fatalf("url = %q", result.URL)
	}
	fetched, err := http.Get(result.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer fetched.Body.Close()
	body, _ := io.ReadAll(fetched.Body)
	if string(body) != "<h1>Hello</h1>" {
		t.Fatalf("body = %q", body)
	}
	if got := fetched.Header.Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
	if fetched.Header.Get("X-Content-Type-Options") != "nosniff" || fetched.Header.Get("Content-Security-Policy") == "" {
		t.Fatal("missing security headers")
	}
	traversal, err := http.Get(result.URL + "../../etc/passwd")
	if err != nil {
		t.Fatal(err)
	}
	traversal.Body.Close()
	if traversal.StatusCode != http.StatusBadRequest {
		t.Fatalf("traversal status = %d", traversal.StatusCode)
	}
}

func TestAuthLimitsAndUnknownType(t *testing.T) {
	_, ts := testServer(t, 4, 1)
	resp, _ := rawUpload(t, ts, "x.txt", "x", "wrong")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad auth = %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp, _ = rawUpload(t, ts, "x.txt", "12345", "test-token")
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize = %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp, result := rawUpload(t, ts, "archive.weird", "1234", "test-token")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload = %d", resp.StatusCode)
	}
	fetched, _ := http.Get(result.EntryURL)
	fetched.Body.Close()
	if fetched.Header.Get("Content-Type") != "application/octet-stream" || !strings.HasPrefix(fetched.Header.Get("Content-Disposition"), "attachment;") {
		t.Fatalf("unknown headers: %#v", fetched.Header)
	}
}

func TestMultipartPreservesPathsAndFileLimit(t *testing.T) {
	_, ts := testServer(t, 1024, 1)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, name := range []string{"img/diagram.txt", "second.txt"} {
		h := textproto.MIMEHeader{"Content-Disposition": {`form-data; name="file"; filename="` + name + `"`}}
		part, _ := mw.CreatePart(h)
		_, _ = part.Write([]byte(name))
	}
	mw.Close()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/bundles", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("file limit = %d", resp.StatusCode)
	}

	_, ts2 := testServer(t, 1024, 2)
	body.Reset()
	mw = multipart.NewWriter(&body)
	h := textproto.MIMEHeader{"Content-Disposition": {`form-data; name="file"; filename="img/diagram.txt"`}}
	part, _ := mw.CreatePart(h)
	_, _ = part.Write([]byte("nested"))
	mw.Close()
	req, _ = http.NewRequest(http.MethodPost, ts2.URL+"/api/v1/bundles", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var result uploadResponse
	_ = json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()
	if result.Files[0].Name != "img/diagram.txt" {
		t.Fatalf("nested name = %q", result.Files[0].Name)
	}
}

func TestMarkdownRawListingAndExpired(t *testing.T) {
	st, ts := testServer(t, 4096, 8)
	resp, md := rawUpload(t, ts, "note.md", "# hi", "test-token")
	if resp.StatusCode != http.StatusCreated {
		t.Fatal(resp.Status)
	}
	rendered, _ := http.Get(md.URL)
	html, _ := io.ReadAll(rendered.Body)
	rendered.Body.Close()
	if !strings.Contains(string(html), "<h1>hi</h1>") || rendered.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("markdown = %q", html)
	}
	raw, _ := http.Get(md.EntryURL + "?raw=1")
	rawBody, _ := io.ReadAll(raw.Body)
	raw.Body.Close()
	if string(rawBody) != "# hi" || !strings.HasPrefix(raw.Header.Get("Content-Type"), "text/plain") {
		t.Fatal("raw markdown mismatch")
	}
	meta, err := st.Create(store.Options{}, []store.File{{Name: "a.txt", Reader: strings.NewReader("a")}, {Name: "b.txt", Reader: strings.NewReader("b")}})
	if err != nil {
		t.Fatal(err)
	}
	listing, _ := http.Get(ts.URL + "/b/" + meta.ID + "/")
	listed, _ := io.ReadAll(listing.Body)
	listing.Body.Close()
	if !strings.Contains(string(listed), "a.txt") || !strings.Contains(string(listed), "b.txt") {
		t.Fatalf("listing = %q", listed)
	}
	past := time.Now().Add(-time.Second)
	expired, _ := st.Create(store.Options{ExpiresAt: &past, ExpiresSet: true}, []store.File{{Name: "x", Reader: strings.NewReader("x")}})
	expiredResp, _ := http.Get(ts.URL + "/b/" + expired.ID + "/")
	expiredResp.Body.Close()
	if expiredResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expired = %d", expiredResp.StatusCode)
	}
}

func TestDurationDays(t *testing.T) {
	d, err := ParseDuration("7d")
	if err != nil || d != 7*24*time.Hour {
		t.Fatalf("ParseDuration = %v, %v", d, err)
	}
}
