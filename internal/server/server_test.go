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

	"github.com/readeem/hostebin/internal/store"
	"github.com/readeem/hostebin/internal/users"
	"github.com/readeem/hostebin/internal/users/filestore"
	"github.com/rs/zerolog"
)

func testServer(t *testing.T, maxUpload int64, maxFiles int) (*store.Store, *httptest.Server) {
	t.Helper()
	dataDir := t.TempDir()
	st, err := store.New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	userStore, err := filestore.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = userStore.Close() })
	userService := users.NewService(userStore)
	if _, _, err := userService.Bootstrap(t.Context(), "test-token"); err != nil {
		t.Fatal(err)
	}
	app, err := New(Config{Store: st, Users: userService, MaxUpload: maxUpload, MaxFiles: maxFiles})
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

func TestFolderPathReturnsNotFound(t *testing.T) {
	_, ts := testServer(t, 1024, 4)
	resp, result := rawUpload(t, ts, "docs/page.html", "nested", "test-token")
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("upload = %d: %s", resp.StatusCode, body)
	}

	folder, err := http.Get(result.URL + "docs")
	if err != nil {
		t.Fatal(err)
	}
	defer folder.Body.Close()
	body, err := io.ReadAll(folder.Body)
	if err != nil {
		t.Fatalf("read folder response: %v", err)
	}
	if folder.StatusCode != http.StatusNotFound {
		t.Fatalf("folder status = %d: %s", folder.StatusCode, body)
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
	meta, err := st.Create(store.Options{OwnerID: "u_test"}, []store.File{{Name: "a.txt", Reader: strings.NewReader("a")}, {Name: "b.txt", Reader: strings.NewReader("b")}})
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
	expired, _ := st.Create(store.Options{OwnerID: "u_test", ExpiresAt: &past, ExpiresSet: true}, []store.File{{Name: "x", Reader: strings.NewReader("x")}})
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

func TestCRUDActionLogging(t *testing.T) {
	dataDir := t.TempDir()
	st, err := store.New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	var logOutput bytes.Buffer
	logger := zerolog.New(&logOutput)
	userStore, err := filestore.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer userStore.Close()
	userService := users.NewService(userStore)
	if _, _, err := userService.Bootstrap(t.Context(), "test-token"); err != nil {
		t.Fatal(err)
	}
	app, err := New(Config{Store: st, Users: userService, Logger: &logger})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	created, result := rawUpload(t, ts, "hello.txt", "hello", "test-token")
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d", created.StatusCode)
	}

	update, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/bundles/"+result.ID+"?mode=merge", strings.NewReader("updated"))
	update.Header.Set("Authorization", "Bearer test-token")
	update.Header.Set("X-Hostebin-Filename", "updated.txt")
	updated, err := http.DefaultClient.Do(update)
	if err != nil {
		t.Fatal(err)
	}
	updated.Body.Close()
	if updated.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d", updated.StatusCode)
	}

	list, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/bundles", nil)
	list.Header.Set("Authorization", "Bearer test-token")
	listed, err := http.DefaultClient.Do(list)
	if err != nil {
		t.Fatal(err)
	}
	listed.Body.Close()
	if listed.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d", listed.StatusCode)
	}

	deleteRequest, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/bundles/"+result.ID, nil)
	deleteRequest.Header.Set("Authorization", "Bearer test-token")
	deleted, err := http.DefaultClient.Do(deleteRequest)
	if err != nil {
		t.Fatal(err)
	}
	deleted.Body.Close()
	if deleted.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", deleted.StatusCode)
	}

	events := make(map[string]map[string]any)
	for line := range strings.SplitSeq(strings.TrimSpace(logOutput.String()), "\n") {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("decode log line %q: %v", line, err)
		}
		action, _ := event["action"].(string)
		events[action] = event
	}
	for _, action := range []string{"create", "read", "update", "delete"} {
		if events[action] == nil {
			t.Errorf("missing %q action in logs: %s", action, logOutput.String())
		}
	}
	for _, action := range []string{"create", "update", "delete"} {
		if got := events[action]["bundle_id"]; got != result.ID {
			t.Errorf("%s bundle_id = %v, want %q", action, got, result.ID)
		}
	}
	if got := events["update"]["mode"]; got != "merge" {
		t.Errorf("update mode = %v, want merge", got)
	}
}

func TestMultiUserIsolationAndRevocation(t *testing.T) {
	_, ts := testServer(t, 4096, 8)

	createBody := strings.NewReader(`{"name":"bob","role":"user"}`)
	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/users", createBody)
	createReq.Header.Set("Authorization", "Bearer test-token")
	createReq.Header.Set("Content-Type", "application/json")
	created, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	var createResponse struct {
		User      users.User `json:"user"`
		Plaintext string     `json:"plaintext"`
	}
	if err := json.NewDecoder(created.Body).Decode(&createResponse); err != nil {
		t.Fatal(err)
	}
	created.Body.Close()
	if created.StatusCode != http.StatusCreated || createResponse.Plaintext == "" {
		t.Fatalf("create user = %d, %#v", created.StatusCode, createResponse)
	}

	adminUpload, adminBundle := rawUpload(t, ts, "admin.txt", "admin", "test-token")
	if adminUpload.StatusCode != http.StatusCreated {
		t.Fatal(adminUpload.Status)
	}
	bobUpload, bobBundle := rawUpload(t, ts, "bob.txt", "bob", createResponse.Plaintext)
	if bobUpload.StatusCode != http.StatusCreated {
		t.Fatal(bobUpload.Status)
	}

	bobListReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/bundles?all=1", nil)
	bobListReq.Header.Set("Authorization", "Bearer "+createResponse.Plaintext)
	bobList, err := http.DefaultClient.Do(bobListReq)
	if err != nil {
		t.Fatal(err)
	}
	var bobMetas []store.BundleMeta
	if err := json.NewDecoder(bobList.Body).Decode(&bobMetas); err != nil {
		t.Fatal(err)
	}
	bobList.Body.Close()
	if bobList.StatusCode != http.StatusOK || len(bobMetas) != 1 || bobMetas[0].ID != bobBundle.ID || bobMetas[0].OwnerID != "" {
		t.Fatalf("Bob list = %d, %#v", bobList.StatusCode, bobMetas)
	}

	adminListReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/bundles?all=1", nil)
	adminListReq.Header.Set("Authorization", "Bearer test-token")
	adminList, err := http.DefaultClient.Do(adminListReq)
	if err != nil {
		t.Fatal(err)
	}
	var allMetas []store.BundleMeta
	if err := json.NewDecoder(adminList.Body).Decode(&allMetas); err != nil {
		t.Fatal(err)
	}
	adminList.Body.Close()
	if len(allMetas) != 2 || allMetas[0].OwnerID == "" || allMetas[1].OwnerID == "" {
		t.Fatalf("admin all list = %#v", allMetas)
	}

	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		var body io.Reader
		if method == http.MethodPut {
			body = strings.NewReader("intrusion")
		}
		req, _ := http.NewRequest(method, ts.URL+"/api/v1/bundles/"+adminBundle.ID, body)
		req.Header.Set("Authorization", "Bearer "+createResponse.Plaintext)
		if method == http.MethodPut {
			req.Header.Set("X-Hostebin-Filename", "x.txt")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("Bob %s Alice bundle = %d", method, resp.StatusCode)
		}
	}

	public, err := http.Get(bobBundle.EntryURL)
	if err != nil {
		t.Fatal(err)
	}
	public.Body.Close()
	if public.StatusCode != http.StatusOK {
		t.Fatalf("public read = %d", public.StatusCode)
	}

	disable, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/users/"+createResponse.User.ID, strings.NewReader(`{"disabled":true}`))
	disable.Header.Set("Authorization", "Bearer test-token")
	disable.Header.Set("Content-Type", "application/json")
	disabled, err := http.DefaultClient.Do(disable)
	if err != nil {
		t.Fatal(err)
	}
	disabled.Body.Close()
	if disabled.StatusCode != http.StatusNoContent {
		t.Fatalf("disable = %d", disabled.StatusCode)
	}
	bobListReq, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/v1/bundles", nil)
	bobListReq.Header.Set("Authorization", "Bearer "+createResponse.Plaintext)
	bobList, _ = http.DefaultClient.Do(bobListReq)
	bobList.Body.Close()
	if bobList.StatusCode != http.StatusUnauthorized {
		t.Fatalf("disabled auth = %d", bobList.StatusCode)
	}

	enable, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/users/"+createResponse.User.ID, strings.NewReader(`{"disabled":false}`))
	enable.Header.Set("Authorization", "Bearer test-token")
	enable.Header.Set("Content-Type", "application/json")
	enabled, _ := http.DefaultClient.Do(enable)
	enabled.Body.Close()
	if enabled.StatusCode != http.StatusNoContent {
		t.Fatalf("enable = %d", enabled.StatusCode)
	}

	whoReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/whoami", nil)
	whoReq.Header.Set("Authorization", "Bearer "+createResponse.Plaintext)
	who, _ := http.DefaultClient.Do(whoReq)
	who.Body.Close()
	revoke, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/users/"+createResponse.User.ID+"/token", nil)
	revoke.Header.Set("Authorization", "Bearer test-token")
	revoked, _ := http.DefaultClient.Do(revoke)
	revoked.Body.Close()
	if revoked.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke = %d", revoked.StatusCode)
	}
	whoReq, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/v1/whoami", nil)
	whoReq.Header.Set("Authorization", "Bearer "+createResponse.Plaintext)
	who, _ = http.DefaultClient.Do(whoReq)
	who.Body.Close()
	if who.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked next request = %d", who.StatusCode)
	}
}

// authRequest issues a bearer-authenticated request with an optional JSON body.
func authRequest(t *testing.T, method, url, token, body string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestCreateUserAppliesTokenOptionsAtomically(t *testing.T) {
	_, ts := testServer(t, 4096, 8)

	created := authRequest(t, http.MethodPost, ts.URL+"/api/v1/users", "test-token", `{"name":"bob","label":"agent","ttl":"1h"}`)
	var response struct {
		User  users.User `json:"user"`
		Token struct {
			ID        string     `json:"id"`
			Label     string     `json:"label"`
			ExpiresAt *time.Time `json:"expires_at"`
		} `json:"token"`
		Plaintext string `json:"plaintext"`
	}
	if err := json.NewDecoder(created.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	created.Body.Close()
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d", created.StatusCode)
	}
	// The returned plaintext must be the one the server actually stored: an
	// earlier implementation created the user then rotated the token, which
	// could leave a user whose advertised token never existed.
	if response.Token.Label != "agent" || response.Token.ExpiresAt == nil {
		t.Fatalf("token = %#v", response.Token)
	}
	who := authRequest(t, http.MethodGet, ts.URL+"/api/v1/whoami", response.Plaintext, "")
	var principal users.Principal
	if err := json.NewDecoder(who.Body).Decode(&principal); err != nil {
		t.Fatal(err)
	}
	who.Body.Close()
	if who.StatusCode != http.StatusOK || principal.UserID != response.User.ID || principal.TokenID != response.Token.ID {
		t.Fatalf("whoami = %d, %#v", who.StatusCode, principal)
	}

	// A rotation with no body means "defaults", not a malformed request.
	rotated := authRequest(t, http.MethodPut, ts.URL+"/api/v1/users/"+response.User.ID+"/token", "test-token", "")
	var rotation struct {
		Plaintext string `json:"plaintext"`
	}
	if err := json.NewDecoder(rotated.Body).Decode(&rotation); err != nil {
		t.Fatal(err)
	}
	rotated.Body.Close()
	if rotated.StatusCode != http.StatusOK || rotation.Plaintext == "" {
		t.Fatalf("bodyless rotate = %d, %q", rotated.StatusCode, rotation.Plaintext)
	}
	stale := authRequest(t, http.MethodGet, ts.URL+"/api/v1/whoami", response.Plaintext, "")
	stale.Body.Close()
	if stale.StatusCode != http.StatusUnauthorized {
		t.Fatalf("token after bodyless rotate = %d", stale.StatusCode)
	}
}

func TestAdminReachesEveryBundle(t *testing.T) {
	_, ts := testServer(t, 4096, 8)

	created := authRequest(t, http.MethodPost, ts.URL+"/api/v1/users", "test-token", `{"name":"bob"}`)
	var response struct {
		User      users.User `json:"user"`
		Plaintext string     `json:"plaintext"`
	}
	if err := json.NewDecoder(created.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	created.Body.Close()

	upload, bundle := rawUpload(t, ts, "bob.txt", "bob", response.Plaintext)
	if upload.StatusCode != http.StatusCreated {
		t.Fatal(upload.Status)
	}

	// Admins moderate content without having to delete its owner, and update
	// and delete agree on who counts as authorized.
	update, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/bundles/"+bundle.ID, strings.NewReader("moderated"))
	update.Header.Set("Authorization", "Bearer test-token")
	update.Header.Set("X-Hostebin-Filename", "bob.txt")
	updated, err := http.DefaultClient.Do(update)
	if err != nil {
		t.Fatal(err)
	}
	updated.Body.Close()
	if updated.StatusCode != http.StatusOK {
		t.Fatalf("admin update = %d", updated.StatusCode)
	}
	deleted := authRequest(t, http.MethodDelete, ts.URL+"/api/v1/bundles/"+bundle.ID, "test-token", "")
	deleted.Body.Close()
	if deleted.StatusCode != http.StatusNoContent {
		t.Fatalf("admin delete = %d", deleted.StatusCode)
	}
}
