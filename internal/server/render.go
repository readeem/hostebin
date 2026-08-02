package server

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io"
	"path"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

//go:embed assets/*
var assets embed.FS

type renderer struct {
	markdown goldmark.Markdown
	md       *template.Template
	dir      *template.Template
	css      template.CSS
}

func newRenderer() (*renderer, error) {
	css, err := assets.ReadFile("assets/base.css")
	if err != nil {
		return nil, err
	}
	md, err := template.ParseFS(assets, "assets/md.html.tmpl")
	if err != nil {
		return nil, err
	}
	dir, err := template.ParseFS(assets, "assets/dir.html.tmpl")
	if err != nil {
		return nil, err
	}
	return &renderer{
		markdown: goldmark.New(goldmark.WithExtensions(extension.GFM), goldmark.WithRendererOptions(html.WithUnsafe())),
		md:       md, dir: dir, css: template.CSS(css),
	}, nil
}

func (r *renderer) renderMarkdown(dst io.Writer, name string, src io.Reader) error {
	input, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	var body bytes.Buffer
	if err := r.markdown.Convert(input, &body); err != nil {
		return err
	}
	return r.md.ExecuteTemplate(dst, "md.html.tmpl", map[string]any{
		"Title": path.Base(name), "CSS": r.css, "Body": template.HTML(body.String()),
	})
}

type listingFile struct{ Name, URL, Size string }

func (r *renderer) renderListing(dst io.Writer, title, description string, files []listingFile) error {
	if title == "" {
		title = "hostebin bundle"
	}
	return r.dir.ExecuteTemplate(dst, "dir.html.tmpl", map[string]any{
		"Title": title, "Description": description, "Files": files, "CSS": r.css,
	})
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for q := n / unit; q >= unit; q /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
