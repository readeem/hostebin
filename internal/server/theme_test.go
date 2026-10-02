package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/readeem/hostebin/internal/users"
)

func TestConcurrentThemeUpdates(t *testing.T) {
	st, ts := testServer(t, 4096, 8)
	who := authRequest(t, http.MethodGet, ts.URL+"/api/v1/whoami", "test-token", "")
	var principal users.Principal
	if err := json.NewDecoder(who.Body).Decode(&principal); err != nil {
		t.Fatal(err)
	}
	who.Body.Close()

	const writers = 24
	start := make(chan struct{})
	errors := make(chan error, writers)
	var wg sync.WaitGroup
	for i := range writers {
		wg.Go(func() {
			<-start
			css := "/*" + strings.Repeat(fmt.Sprintf("%02d", i), 30000) + "*/"
			req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/users/"+principal.UserID+"/theme", strings.NewReader(css))
			if err != nil {
				errors <- err
				return
			}
			req.Header.Set("Authorization", "Bearer test-token")
			resp, err := ts.Client().Do(req)
			if err != nil {
				errors <- err
				return
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusNoContent {
				errors <- fmt.Errorf("update %d: %d: %s", i, resp.StatusCode, body)
			}
		})
	}
	close(start)
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	css, err := os.ReadFile(filepath.Join(st.DataDir(), "themes", principal.UserID+".css"))
	if err != nil {
		t.Fatal(err)
	}
	for i := range writers {
		if string(css) == "/*"+strings.Repeat(fmt.Sprintf("%02d", i), 30000)+"*/" {
			return
		}
	}
	t.Fatal("stored theme is not a complete uploaded stylesheet")
}

func TestThemesStayInsideDataRoot(t *testing.T) {
	st, ts := testServer(t, 4096, 8)
	who := authRequest(t, http.MethodGet, ts.URL+"/api/v1/whoami", "test-token", "")
	var principal users.Principal
	if err := json.NewDecoder(who.Body).Decode(&principal); err != nil {
		t.Fatal(err)
	}
	who.Body.Close()

	outside := t.TempDir()
	path := filepath.Join(outside, principal.UserID+".css")
	const original = "/* outside the data root */"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(st.DataDir(), "themes")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, bundle := rawUpload(t, ts, "notes.md", "# notes", "test-token")
	resp, err := ts.Client().Get(ts.URL + "/b/" + bundle.ID + "/.hostebin/theme.css")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), original) {
		t.Error("served a theme outside the data root")
	}
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		resp := authRequest(t, method, ts.URL+"/api/v1/users/"+principal.UserID+"/theme", "test-token", ":root { --canvas: #fff; }")
		resp.Body.Close()
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("%s through symlink = %d, want 500", method, resp.StatusCode)
		}
		if css, err := os.ReadFile(path); err != nil || string(css) != original {
			t.Fatalf("%s changed the external file: %q, %v", method, css, err)
		}
	}
}
