package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

type uploadResult struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	EntryURL string `json:"entry_url"`
	Files    []struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
		URL  string `json:"url"`
	} `json:"files"`
	ExpiresAt any `json:"expires_at"`
}

type localFile struct {
	name, diskPath string
	stdin          bool
}

func runUp(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cfg, err := NewConfig()
	if err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return exitUsage
	}
	fs := flag.NewFlagSet("up", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var title, ttl, entry, id, stdinName string
	var jsonOutput, openBrowser, quiet bool
	cfg.registerClientFlags(fs)
	fs.StringVar(&title, "title", "", "bundle title")
	fs.StringVar(&ttl, "ttl", "", "expiry duration (for example 7d or 30m) or never")
	fs.StringVar(&entry, "entry", "", "entry file")
	boolVar(fs, &jsonOutput, "json", "print structured JSON")
	fs.StringVar(&id, "id", "", "replace an existing bundle")
	boolVar(fs, &openBrowser, "open", "open the resulting URL")
	boolVar(fs, &quiet, "quiet", "suppress diagnostics")
	fs.StringVar(&stdinName, "name", "", "file name for stdin")
	fs.StringVar(&stdinName, "n", "", "file name for stdin (shorthand)")
	if err := parseConfig(fs, args); err != nil {
		return exitUsage
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "usage: hostebin up [flags] <file|directory|->...")
		return exitUsage
	}
	if err := resolveClientConfig(cfg); err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return exitUsage
	}
	files, err := collectFiles(fs.Args(), stdinName)
	if err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return exitUsage
	}
	pipeReader, pipeWriter := io.Pipe()
	mw := multipart.NewWriter(pipeWriter)
	writeErr := make(chan error, 1)

	go func() {
		defer close(writeErr)
		for key, value := range map[string]string{"title": title, "ttl": ttl, "entry": entry} {
			if value != "" {
				if err := mw.WriteField(key, value); err != nil {
					_ = pipeWriter.CloseWithError(err)
					writeErr <- err
					return
				}
			}
		}
		for _, file := range files {
			header := make(map[string][]string)
			header["Content-Disposition"] = []string{mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": file.name})}
			if typ := mime.TypeByExtension(filepath.Ext(file.name)); typ != "" {
				header["Content-Type"] = []string{typ}
			}
			part, err := mw.CreatePart(textproto.MIMEHeader(header))
			if err != nil {
				_ = pipeWriter.CloseWithError(err)
				writeErr <- err
				return
			}
			var src io.Reader
			var opened *os.File
			if file.stdin {
				src = stdin
			} else {
				opened, err = os.Open(file.diskPath)
				if err != nil {
					_ = pipeWriter.CloseWithError(err)
					writeErr <- err
					return
				}
				src = opened
			}
			_, err = io.Copy(part, src)
			if opened != nil {
				_ = opened.Close()
			}
			if err != nil {
				_ = pipeWriter.CloseWithError(err)
				writeErr <- err
				return
			}
		}
		if err := mw.Close(); err != nil {
			_ = pipeWriter.CloseWithError(err)
			writeErr <- err
			return
		}
		writeErr <- pipeWriter.Close()
	}()
	endpoint := cfg.Server + "/api/v1/bundles"
	method := http.MethodPost
	if id != "" {
		method = http.MethodPut
		endpoint += "/" + url.PathEscape(id) + "?mode=replace"
	}
	req, err := http.NewRequest(method, endpoint, pipeReader)
	if err != nil {
		_ = pipeReader.CloseWithError(err)
		_ = pipeWriter.CloseWithError(err)
		fmt.Fprintln(stderr, "hostebin:", err)
		return exitNetwork
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		_ = pipeReader.CloseWithError(err)
		fmt.Fprintln(stderr, "hostebin:", err)
		return exitNetwork
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return exitNetwork
	}
	if streamErr := <-writeErr; streamErr != nil && !errors.Is(streamErr, io.ErrClosedPipe) {
		fmt.Fprintln(stderr, "hostebin:", streamErr)
		return exitNetwork
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &apiErr)
		if apiErr.Error == "" {
			apiErr.Error = strings.TrimSpace(string(body))
		}
		fmt.Fprintf(stderr, "hostebin: server returned %s: %s\n", resp.Status, apiErr.Error)
		return exitNetwork
	}
	var result uploadResult
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Fprintln(stderr, "hostebin: invalid server response:", err)
		return exitNetwork
	}
	if jsonOutput {
		var compact any
		if err := json.Unmarshal(body, &compact); err != nil {
			fmt.Fprintln(stderr, "hostebin:", err)
			return exitNetwork
		}
		encoded, _ := json.Marshal(compact)
		fmt.Fprintln(stdout, string(encoded))
	} else {
		fmt.Fprintln(stdout, result.URL)
	}
	if openBrowser {
		openURL(result.URL, quiet, stderr)
	}
	return exitOK
}

func collectFiles(args []string, stdinName string) ([]localFile, error) {
	var files []localFile
	seen := map[string]bool{}
	add := func(file localFile) error {
		name := filepath.ToSlash(filepath.Clean(file.name))
		name = strings.TrimPrefix(name, "./")
		if name == "." || name == "" || filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "\\") {
			return fmt.Errorf("unsafe upload name %q", file.name)
		}
		if seen[name] {
			return fmt.Errorf("duplicate upload name %q", name)
		}
		seen[name], file.name = true, name
		files = append(files, file)
		return nil
	}
	stdinSeen := false
	for _, arg := range args {
		if arg == "-" {
			if stdinSeen {
				return nil, errors.New("stdin may only be uploaded once")
			}
			if stdinName == "" {
				return nil, errors.New("-n/--name is required when uploading stdin")
			}
			stdinSeen = true
			if err := add(localFile{name: stdinName, stdin: true}); err != nil {
				return nil, err
			}
			continue
		}
		info, err := os.Stat(arg)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			err := filepath.WalkDir(arg, func(filePath string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if entry.Type()&os.ModeSymlink != 0 {
					return fmt.Errorf("refusing symlink %s", filePath)
				}
				if entry.IsDir() {
					return nil
				}
				if !entry.Type().IsRegular() {
					return fmt.Errorf("refusing non-regular file %s", filePath)
				}
				rel, err := filepath.Rel(arg, filePath)
				if err != nil {
					return err
				}
				return add(localFile{name: rel, diskPath: filePath})
			})
			if err != nil {
				return nil, err
			}
		} else {
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("refusing non-regular file %s", arg)
			}
			name := arg
			if filepath.IsAbs(name) || strings.HasPrefix(filepath.Clean(name), ".."+string(filepath.Separator)) {
				name = filepath.Base(name)
			}
			if err := add(localFile{name: name, diskPath: arg}); err != nil {
				return nil, err
			}
		}
	}
	if len(files) == 0 {
		return nil, errors.New("no files found")
	}
	slices.SortFunc(files, func(a, b localFile) int { return strings.Compare(a.name, b.name) })
	return files, nil
}

func openURL(target string, quiet bool, stderr io.Writer) {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name = "open"
		args = []string{target}
	case "windows":
		name = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", target}
	default:
		name = "xdg-open"
		args = []string{target}
	}
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil && !quiet {
		fmt.Fprintln(stderr, "hostebin: could not open browser:", err)
	}
}
