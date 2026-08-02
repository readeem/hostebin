package store

import "time"

// BundleMeta is the durable description of an uploaded bundle.
type BundleMeta struct {
	ID        string     `json:"id"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at"`
	Title     string     `json:"title,omitempty"`
	Entry     string     `json:"entry,omitempty"`
	Bytes     int64      `json:"bytes"`
	Uploader  string     `json:"uploader,omitempty"`
	Files     []FileMeta `json:"files"`
}

type FileMeta struct {
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type"`
}

type Options struct {
	Title      string
	Entry      string
	EntrySet   bool
	ExpiresAt  *time.Time
	ExpiresSet bool
	Uploader   string
}

type File struct {
	Name        string
	ContentType string
	Reader      interface{ Read([]byte) (int, error) }
}
