// Package apk describes the Marquee Player APK the hub hands out: its
// checksum, which installed players compare with their own, and the version
// written in its manifest, which the install page and the admin show.
package apk

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
	"unicode/utf16"
)

// Release is one APK file.
type Release struct {
	SHA256  string
	Size    int64
	Mod     time.Time
	Version string // versionName, "" when the manifest could not be read
	Code    int64  // versionCode
}

// File is the APK at Path; it is read again only when it changes, so a new
// player version ships by replacing the file.
type File struct {
	Path string

	mu  sync.Mutex
	rel Release
}

func NewFile(path string) *File { return &File{Path: path} }

// Release describes the file; false when there is none.
func (f *File) Release() (Release, bool) {
	if f == nil || f.Path == "" {
		return Release{}, false
	}
	st, err := os.Stat(f.Path)
	if err != nil || st.IsDir() {
		return Release{}, false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rel.SHA256 != "" && f.rel.Size == st.Size() && f.rel.Mod.Equal(st.ModTime()) {
		return f.rel, true
	}
	fh, err := os.Open(f.Path)
	if err != nil {
		return Release{}, false
	}
	defer fh.Close()
	hs := sha256.New()
	if _, err := io.Copy(hs, fh); err != nil {
		return Release{}, false
	}
	rel := Release{SHA256: hex.EncodeToString(hs.Sum(nil)), Size: st.Size(), Mod: st.ModTime()}
	// A file that is no APK still downloads; only its version stays unknown.
	rel.Version, rel.Code, _ = ReadVersion(fh, st.Size())
	f.rel = rel
	return rel, true
}

// Label is the version for people: "0.3.0 (3)".
func (r Release) Label() string {
	if r.Version == "" {
		return ""
	}
	return fmt.Sprintf("%s (%d)", r.Version, r.Code)
}

// ReadVersion reads versionName and versionCode from the APK's compiled
// AndroidManifest.xml.
func ReadVersion(r io.ReaderAt, size int64) (string, int64, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return "", 0, err
	}
	for _, zf := range zr.File {
		if zf.Name != "AndroidManifest.xml" {
			continue
		}
		if zf.UncompressedSize64 > 8<<20 {
			return "", 0, errors.New("apk: manifest too large")
		}
		rc, err := zf.Open()
		if err != nil {
			return "", 0, err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return "", 0, err
		}
		return manifestVersion(b)
	}
	return "", 0, errors.New("apk: no AndroidManifest.xml")
}

// Chunk types and constants of Android's binary XML.
const (
	chunkStringPool = 0x0001
	chunkXML        = 0x0003
	chunkResMap     = 0x0180
	chunkStartElem  = 0x0102

	utf8Flag = 1 << 8

	typeString = 0x03
	typeIntDec = 0x10
	typeIntHex = 0x11

	attrVersionCode = 0x0101021b
	attrVersionName = 0x0101021c
)

var errFormat = errors.New("apk: unreadable manifest")

// manifestVersion walks the chunks up to the <manifest> element and reads
// its version attributes, matched by resource id (or by name when the map is
// missing).
func manifestVersion(b []byte) (string, int64, error) {
	if len(b) < 8 || le16(b, 0) != chunkXML {
		return "", 0, errFormat
	}
	var pool []string
	var resIDs []uint32
	for off, size := int(le16(b, 2)), 0; off+8 <= len(b); off += size {
		var typ uint16
		var hdr int
		typ, hdr, size = le16(b, off), int(le16(b, off+2)), int(le32(b, off+4))
		if size < 8 || off+size > len(b) {
			return "", 0, errFormat
		}
		c := b[off : off+size]
		switch typ {
		case chunkStringPool:
			var err error
			if pool, err = readPool(c); err != nil {
				return "", 0, err
			}
		case chunkResMap:
			for i := hdr; i+4 <= len(c); i += 4 {
				resIDs = append(resIDs, le32(c, i))
			}
		case chunkStartElem:
			if len(c) < 36 {
				return "", 0, errFormat
			}
			if str(pool, le32(c, 20)) != "manifest" {
				continue
			}
			start, step, n := 16+int(le16(c, 24)), int(le16(c, 26)), int(le16(c, 28))
			var name string
			var code int64
			for i := 0; i < n; i++ {
				a := start + i*step
				if step < 20 || a+20 > len(c) {
					return "", 0, errFormat
				}
				nameIdx := le32(c, a+4)
				id := uint32(0)
				if int(nameIdx) < len(resIDs) {
					id = resIDs[nameIdx]
				}
				attr := str(pool, nameIdx)
				dataType, data := c[a+15], le32(c, a+16)
				switch {
				case id == attrVersionCode || (id == 0 && attr == "versionCode"):
					if dataType == typeIntDec || dataType == typeIntHex {
						code = int64(data)
					}
				case id == attrVersionName || (id == 0 && attr == "versionName"):
					if raw := le32(c, a+8); raw != 0xffffffff {
						name = str(pool, raw)
					} else if dataType == typeString {
						name = str(pool, data)
					}
				}
			}
			return name, code, nil
		}
	}
	return "", 0, errFormat
}

func readPool(c []byte) ([]string, error) {
	if len(c) < 28 {
		return nil, errFormat
	}
	count, flags, start := int(le32(c, 8)), le32(c, 16), int(le32(c, 20))
	if 28+count*4 > len(c) {
		return nil, errFormat
	}
	out := make([]string, count)
	for i := range out {
		p := start + int(le32(c, 28+i*4))
		if p >= len(c) {
			return nil, errFormat
		}
		var ok bool
		if flags&utf8Flag != 0 {
			out[i], ok = utf8At(c, p)
		} else {
			out[i], ok = utf16At(c, p)
		}
		if !ok {
			return nil, errFormat
		}
	}
	return out, nil
}

// utf8At reads a string stored as UTF-16 length, UTF-8 length, bytes.
func utf8At(c []byte, p int) (string, bool) {
	_, p = len8(c, p)
	n, p := len8(c, p)
	if p < 0 || p+n > len(c) {
		return "", false
	}
	return string(c[p : p+n]), true
}

func len8(c []byte, p int) (int, int) {
	if p < 0 || p >= len(c) {
		return 0, -1
	}
	n := int(c[p])
	if n&0x80 == 0 {
		return n, p + 1
	}
	if p+1 >= len(c) {
		return 0, -1
	}
	return (n&0x7f)<<8 | int(c[p+1]), p + 2
}

func utf16At(c []byte, p int) (string, bool) {
	if p+2 > len(c) {
		return "", false
	}
	n := int(le16(c, p))
	p += 2
	if n&0x8000 != 0 {
		if p+2 > len(c) {
			return "", false
		}
		n = (n&0x7fff)<<16 | int(le16(c, p))
		p += 2
	}
	if p+2*n > len(c) {
		return "", false
	}
	u := make([]uint16, n)
	for i := range u {
		u[i] = le16(c, p+2*i)
	}
	return string(utf16.Decode(u)), true
}

func str(pool []string, i uint32) string {
	if int(i) < len(pool) {
		return pool[i]
	}
	return ""
}

func le16(b []byte, off int) uint16 { return binary.LittleEndian.Uint16(b[off:]) }
func le32(b []byte, off int) uint32 { return binary.LittleEndian.Uint32(b[off:]) }
