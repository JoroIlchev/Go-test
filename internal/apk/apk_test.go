package apk

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// testdata/AndroidManifest.xml is the compiled manifest of Marquee Player 0.1.0.
func testAPK(t *testing.T) []byte {
	t.Helper()
	man, err := os.ReadFile("testdata/AndroidManifest.xml")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string][]byte{"classes.dex": []byte("dex"), "AndroidManifest.xml": man} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(body)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReadVersion(t *testing.T) {
	b := testAPK(t)
	name, code, err := ReadVersion(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if name != "0.1.0" || code != 1 {
		t.Fatalf("got %q %d", name, code)
	}
}

func TestReadVersionGarbage(t *testing.T) {
	for _, b := range [][]byte{nil, []byte("not a zip"), {3, 0, 8, 0, 255, 255, 0, 0}} {
		if _, _, err := ReadVersion(bytes.NewReader(b), int64(len(b))); err == nil {
			t.Errorf("%q: no error", b)
		}
	}
	// Truncated manifests must fail, not panic.
	man, _ := os.ReadFile("testdata/AndroidManifest.xml")
	for n := 0; n < len(man); n += 7 {
		manifestVersion(man[:n])
	}
}

func TestFileRelease(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marquee-player.apk")
	f := NewFile(path)
	if _, ok := f.Release(); ok {
		t.Fatal("release without a file")
	}
	os.WriteFile(path, testAPK(t), 0o644)
	r, ok := f.Release()
	if !ok || r.Label() != "0.1.0 (1)" || len(r.SHA256) != 64 {
		t.Fatalf("got %+v %v", r, ok)
	}
	// Not an APK: still served, version unknown.
	os.WriteFile(path, []byte("something else"), 0o644)
	r, ok = f.Release()
	if !ok || r.Version != "" || r.Label() != "" || r.Size != 14 {
		t.Fatalf("got %+v %v", r, ok)
	}
}
