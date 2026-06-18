package attachments

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
)

const MaxAttachmentBytes int64 = 5 * 1024 * 1024

// BuildMultipart constructs a multipart/form-data request body with regular
// fields, repeated attachment_blobs[], and repeated attachments[] file parts.
func BuildMultipart(fields map[string]string, paths []string, blobs []string) ([]byte, string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, "", err
		}
	}
	for _, blob := range blobs {
		if strings.TrimSpace(blob) == "" {
			return nil, "", fmt.Errorf("--attach-blob requires a non-empty signed_id or sgid")
		}
		if err := writer.WriteField("attachment_blobs[]", blob); err != nil {
			return nil, "", err
		}
	}
	for _, path := range paths {
		if err := AddFile(writer, path); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), writer.FormDataContentType(), nil
}

func AddFile(writer *multipart.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open attachment %q: %w", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat attachment %q: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("attachment %q is a directory", path)
	}
	if info.Size() > MaxAttachmentBytes {
		return fmt.Errorf("attachment %q is too large (max 5 MB)", path)
	}
	peek := make([]byte, 512)
	n, err := file.Read(peek)
	if err != nil && err != io.EOF {
		return fmt.Errorf("read attachment %q: %w", path, err)
	}
	contentType := http.DetectContentType(peek[:n])
	if !AllowedContentType(contentType) {
		return fmt.Errorf("unsupported attachment type %q for %q (allowed: PNG, JPEG, GIF, WEBP)", contentType, path)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="attachments[]"; filename="%s"`, escapeQuotes(filepath.Base(path))))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(part, file)
	return err
}

func AllowedContentType(contentType string) bool {
	switch contentType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func escapeQuotes(value string) string {
	return strings.ReplaceAll(value, `"`, `\\"`)
}
