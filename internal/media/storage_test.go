package media

import (
	"bytes"
	"io"
	"testing"
)

func TestSaveAndOpen(t *testing.T) {
	storage, err := NewStorage(t.TempDir(), 32)
	if err != nil {
		t.Fatal(err)
	}
	content := append(make([]byte, 4), []byte("ftypisom")...)
	content = append(content, []byte("video data")...)
	saved, err := storage.Save(bytes.NewReader(content), "clip.mp4", "video/mp4")
	if err != nil {
		t.Fatal(err)
	}
	file, err := storage.Open(saved.StorageKey)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, content) {
		t.Fatalf("unexpected contents %q", data)
	}
}

func TestRejectsOversizedUpload(t *testing.T) {
	storage, err := NewStorage(t.TempDir(), 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Save(bytes.NewBufferString("12345"), "clip.mp4", "video/mp4"); err == nil {
		t.Fatal("oversized upload accepted")
	}
}
