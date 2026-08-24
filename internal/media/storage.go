package media

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

var allowedTypes = map[string]struct{}{
	"video/mp4":  {},
	"video/webm": {},
	"video/ogg":  {},
}

type Storage struct {
	dir      string
	maxBytes int64
}
type SavedVideo struct {
	StorageKey, ContentType string
	Size                    int64
}

func NewStorage(dir string, maxBytes int64) (*Storage, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &Storage{dir: dir, maxBytes: maxBytes}, nil
}

func (s *Storage) Save(file io.Reader, filename, declaredType string) (SavedVideo, error) {
	if _, ok := allowedTypes[declaredType]; !ok {
		return SavedVideo{}, errors.New("only MP4, WebM, and Ogg video files are allowed")
	}
	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return SavedVideo{}, err
	}
	detected := http.DetectContentType(head[:n])
	if detected != "application/octet-stream" && detected != declaredType {
		return SavedVideo{}, errors.New("the uploaded file does not match its declared video type")
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		ext = extensionFor(declaredType)
	}
	key := uuid.NewString() + ext
	temporary, err := os.CreateTemp(s.dir, "upload-*")
	if err != nil {
		return SavedVideo{}, err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return SavedVideo{}, err
	}
	limited := io.LimitReader(io.MultiReader(bytes.NewReader(head[:n]), file), s.maxBytes+1)
	written, err := io.Copy(temporary, limited)
	closeErr := temporary.Close()
	if err != nil {
		return SavedVideo{}, err
	}
	if closeErr != nil {
		return SavedVideo{}, closeErr
	}
	if written > s.maxBytes {
		return SavedVideo{}, errors.New("video exceeds upload limit")
	}
	if err := os.Rename(temporaryName, filepath.Join(s.dir, key)); err != nil {
		return SavedVideo{}, err
	}
	return SavedVideo{StorageKey: key, ContentType: declaredType, Size: written}, nil
}

func (s *Storage) Open(key string) (*os.File, error) {
	if filepath.Base(key) != key {
		return nil, errors.New("invalid storage key")
	}
	return os.Open(filepath.Join(s.dir, key))
}

func extensionFor(contentType string) string {
	exts, _ := mime.ExtensionsByType(contentType)
	if len(exts) > 0 {
		return exts[0]
	}
	return ".video"
}
