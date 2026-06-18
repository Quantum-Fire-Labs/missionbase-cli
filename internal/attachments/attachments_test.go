package attachments

import (
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildMultipartFieldsAttachmentsAndBlobs(t *testing.T) {
	png := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(png, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}, 0o600); err != nil {
		t.Fatal(err)
	}
	body, contentType, err := BuildMultipart(map[string]string{"comment": "hello"}, []string{png}, []string{"signed-1"})
	if err != nil {
		t.Fatalf("BuildMultipart: %v", err)
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "multipart/form-data" {
		t.Fatalf("parse content type: %q %v", mediaType, err)
	}
	reader := multipart.NewReader(strings.NewReader(string(body)), params["boundary"])
	seen := map[string]bool{}
	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}
		seen[part.FormName()] = true
	}
	for _, name := range []string{"comment", "attachment_blobs[]", "attachments[]"} {
		if !seen[name] {
			t.Fatalf("missing multipart field %q; saw %#v", name, seen)
		}
	}
}

func TestAddFileRejectsUnsupportedType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.txt")
	if err := os.WriteFile(path, []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	writer := multipart.NewWriter(&stringWriter{&b})
	if err := AddFile(writer, path); err == nil {
		t.Fatal("AddFile succeeded for text file, want error")
	}
}

type stringWriter struct{ b *strings.Builder }

func (w *stringWriter) Write(p []byte) (int, error) { return w.b.Write(p) }
