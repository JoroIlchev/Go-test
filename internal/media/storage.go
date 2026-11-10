// Package media stores uploaded files by content hash, inspects them with
// ffprobe, makes thumbnails and converts videos to an Android-safe format.
package media

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrTooLarge = errors.New("file too large")

// Kinds by extension. Anything else is refused at upload.
var kindByExt = map[string]string{
	"jpg": "image", "jpeg": "image", "png": "image", "gif": "image", "webp": "image",
	"mp4": "video", "m4v": "video", "mov": "video", "webm": "video", "mkv": "video", "avi": "video",
	"mp3": "audio", "m4a": "audio", "aac": "audio", "ogg": "audio", "wav": "audio",
}

var mimeByExt = map[string]string{
	"jpg": "image/jpeg", "jpeg": "image/jpeg", "png": "image/png", "gif": "image/gif", "webp": "image/webp",
	"mp4": "video/mp4", "m4v": "video/mp4", "mov": "video/quicktime", "webm": "video/webm",
	"mkv": "video/x-matroska", "avi": "video/x-msvideo",
	"mp3": "audio/mpeg", "m4a": "audio/mp4", "aac": "audio/aac", "ogg": "audio/ogg", "wav": "audio/wav",
}

// KindOf returns the media kind and normalised extension of a file name.
func KindOf(filename string) (kind, ext string, ok bool) {
	ext = strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
	if ext == "jpeg" {
		ext = "jpg"
	}
	kind, ok = kindByExt[ext]
	return kind, ext, ok
}

func MimeOf(ext string) string {
	if m, ok := mimeByExt[ext]; ok {
		return m
	}
	return "application/octet-stream"
}

// Storage keeps blobs at <root>/<sha[:2]>/<sha>.<ext> and thumbnails at
// <root>/thumbs/<sha>.jpg.
type Storage struct {
	root string
}

func NewStorage(root string) (*Storage, error) {
	for _, d := range []string{root, filepath.Join(root, "tmp"), filepath.Join(root, "thumbs")} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			return nil, err
		}
	}
	return &Storage{root: root}, nil
}

func (s *Storage) Path(sha, ext string) string {
	return filepath.Join(s.root, sha[:2], sha+"."+ext)
}

func (s *Storage) ThumbPath(sha string) string {
	return filepath.Join(s.root, "thumbs", sha+".jpg")
}

// TempFile returns a new file in the storage's tmp dir (same volume, so the
// final rename is atomic).
func (s *Storage) TempFile(pattern string) (*os.File, error) {
	return os.CreateTemp(filepath.Join(s.root, "tmp"), pattern)
}

// TempDir returns a new directory in the storage's tmp dir; the caller removes it.
func (s *Storage) TempDir(pattern string) (string, error) {
	return os.MkdirTemp(filepath.Join(s.root, "tmp"), pattern)
}

// Save streams r to storage, hashing on the way. An identical existing blob
// is kept and the new copy discarded.
func (s *Storage) Save(r io.Reader, ext string, maxSize int64) (sha string, size int64, err error) {
	tmp, err := s.TempFile("upload-*")
	if err != nil {
		return "", 0, err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	size, err = io.Copy(io.MultiWriter(tmp, h), io.LimitReader(r, maxSize+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", 0, err
	}
	if size > maxSize {
		return "", 0, ErrTooLarge
	}
	sha = hex.EncodeToString(h.Sum(nil))
	return sha, size, s.adopt(tmp.Name(), sha, ext)
}

// Adopt moves a finished file (e.g. a conversion output) into storage.
func (s *Storage) Adopt(path, ext string) (sha string, size int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	h := sha256.New()
	size, err = io.Copy(h, f)
	f.Close()
	if err != nil {
		return "", 0, err
	}
	sha = hex.EncodeToString(h.Sum(nil))
	return sha, size, s.adopt(path, sha, ext)
}

func (s *Storage) adopt(path, sha, ext string) error {
	dst := s.Path(sha, ext)
	if _, err := os.Stat(dst); err == nil {
		return os.Remove(path)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	return os.Rename(path, dst)
}

func (s *Storage) Remove(sha, ext string) {
	os.Remove(s.Path(sha, ext))
	os.Remove(s.ThumbPath(sha))
}
