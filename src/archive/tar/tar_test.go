// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tar

import (
	"bytes"
	"internal/testenv"
	"io/ioutil"
	"math"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestFileInfoHeader(t *testing.T) {
	fi, err := os.Stat("testdata/small.txt")
	if err != nil {
		t.Fatal(err)
	}
	h, err := FileInfoHeader(fi, "")
	if err != nil {
		t.Fatalf("FileInfoHeader: %v", err)
	}
	if g, e := h.Name, "small.txt"; g != e {
		t.Errorf("Name = %q; want %q", g, e)
	}
	if g, e := h.Mode, int64(fi.Mode().Perm()); g != e {
		t.Errorf("Mode = %#o; want %#o", g, e)
	}
	if g, e := h.Size, int64(5); g != e {
		t.Errorf("Size = %v; want %v", g, e)
	}
	if g, e := h.ModTime, fi.ModTime(); !g.Equal(e) {
		t.Errorf("ModTime = %v; want %v", g, e)
	}
	// FileInfoHeader should error when passing nil FileInfo
	if _, err := FileInfoHeader(nil, ""); err == nil {
		t.Fatalf("Expected error when passing nil to FileInfoHeader")
	}
}

func TestFileInfoHeaderDir(t *testing.T) {
	fi, err := os.Stat("testdata")
	if err != nil {
		t.Fatal(err)
	}
	h, err := FileInfoHeader(fi, "")
	if err != nil {
		t.Fatalf("FileInfoHeader: %v", err)
	}
	if g, e := h.Name, "testdata/"; g != e {
		t.Errorf("Name = %q; want %q", g, e)
	}
	// Ignoring c_ISGID for golang.org/issue/4867
	if g, e := h.Mode&^c_ISGID, int64(fi.Mode().Perm()); g != e {
		t.Errorf("Mode = %#o; want %#o", g, e)
	}
	if g, e := h.Size, int64(0); g != e {
		t.Errorf("Size = %v; want %v", g, e)
	}
	if g, e := h.ModTime, fi.ModTime(); !g.Equal(e) {
		t.Errorf("ModTime = %v; want %v", g, e)
	}
}

func TestFileInfoHeaderSymlink(t *testing.T) {
	testenv.MustHaveSymlink(t)

	tmpdir, err := ioutil.TempDir("", "TestFileInfoHeaderSymlink")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	link := filepath.Join(tmpdir, "link")
	target := tmpdir
	err = os.Symlink(target, link)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}

	h, err := FileInfoHeader(fi, target)
	if err != nil {
		t.Fatal(err)
	}
	if g, e := h.Name, fi.Name(); g != e {
		t.Errorf("Name = %q; want %q", g, e)
	}
	if g, e := h.Linkname, target; g != e {
		t.Errorf("Linkname = %q; want %q", g, e)
	}
	if g, e := h.Typeflag, byte(TypeSymlink); g != e {
		t.Errorf("Typeflag = %v; want %v", g, e)
	}
}

func TestRoundTrip(t *testing.T) {
	data := []byte("some file contents")

	var b bytes.Buffer
	tw := NewWriter(&b)
	hdr := &Header{
		Name: "file.txt",
		Uid:  1 << 21, // too big for 8 octal digits
		Size: int64(len(data)),
		// AddDate to strip monotonic clock reading,
		// and Round to discard sub-second precision,
		// both of which are not included in the tar header
		// and would otherwise break the round-trip check
		// below.
		ModTime: time.Now().AddDate(0, 0, 0).Round(1 * time.Second),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("tw.WriteHeader: %v", err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatalf("tw.Write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tw.Close: %v", err)
	}

	// Read it back.
	tr := NewReader(&b)
	rHdr, err := tr.Next()
	if err != nil {
		t.Fatalf("tr.Next: %v", err)
	}
	if !reflect.DeepEqual(rHdr, hdr) {
		t.Errorf("Header mismatch.\n got %+v\nwant %+v", rHdr, hdr)
	}
	rData, err := ioutil.ReadAll(tr)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !bytes.Equal(rData, data) {
		t.Errorf("Data mismatch.\n got %q\nwant %q", rData, data)
	}
}

type headerRoundTripTest struct {
	h  *Header
	fm os.FileMode
}

func TestHeaderRoundTrip(t *testing.T) {
	vectors := []headerRoundTripTest{{
		// regular file.
		h: &Header{
			Name:     "test.txt",
			Mode:     0644,
			Size:     12,
			ModTime:  time.Unix(1360600916, 0),
			Typeflag: TypeReg,
		},
		fm: 0644,
	}, {
		// symbolic link.
		h: &Header{
			Name:     "link.txt",
			Mode:     0777,
			Size:     0,
			ModTime:  time.Unix(1360600852, 0),
			Typeflag: TypeSymlink,
		},
		fm: 0777 | os.ModeSymlink,
	}, {
		// character device node.
		h: &Header{
			Name:     "dev/null",
			Mode:     0666,
			Size:     0,
			ModTime:  time.Unix(1360578951, 0),
			Typeflag: TypeChar,
		},
		fm: 0666 | os.ModeDevice | os.ModeCharDevice,
	}, {
		// block device node.
		h: &Header{
			Name:     "dev/sda",
			Mode:     0660,
			Size:     0,
			ModTime:  time.Unix(1360578954, 0),
			Typeflag: TypeBlock,
		},
		fm: 0660 | os.ModeDevice,
	}, {
		// directory.
		h: &Header{
			Name:     "dir/",
			Mode:     0755,
			Size:     0,
			ModTime:  time.Unix(1360601116, 0),
			Typeflag: TypeDir,
		},
		fm: 0755 | os.ModeDir,
	}, {
		// fifo node.
		h: &Header{
			Name:     "dev/initctl",
			Mode:     0600,
			Size:     0,
			ModTime:  time.Unix(1360578949, 0),
			Typeflag: TypeFifo,
		},
		fm: 0600 | os.ModeNamedPipe,
	}, {
		// setuid.
		h: &Header{
			Name:     "bin/su",
			Mode:     0755 | c_ISUID,
			Size:     23232,
			ModTime:  time.Unix(1355405093, 0),
			Typeflag: TypeReg,
		},
		fm: 0755 | os.ModeSetuid,
	}, {
		// setguid.
		h: &Header{
			Name:     "group.txt",
			Mode:     0750 | c_ISGID,
			Size:     0,
			ModTime:  time.Unix(1360602346, 0),
			Typeflag: TypeReg,
		},
		fm: 0750 | os.ModeSetgid,
	}, {
		// sticky.
		h: &Header{
			Name:     "sticky.txt",
			Mode:     0600 | c_ISVTX,
			Size:     7,
			ModTime:  time.Unix(1360602540, 0),
			Typeflag: TypeReg,
		},
		fm: 0600 | os.ModeSticky,
	}, {
		// hard link.
		h: &Header{
			Name:     "hard.txt",
			Mode:     0644,
			Size:     0,
			Linkname: "file.txt",
			ModTime:  time.Unix(1360600916, 0),
			Typeflag: TypeLink,
		},
		fm: 0644,
	}, {
		// More information.
		h: &Header{
			Name:     "info.txt",
			Mode:     0600,
			Size:     0,
			Uid:      1000,
			Gid:      1000,
			ModTime:  time.Unix(1360602540, 0),
			Uname:    "slartibartfast",
			Gname:    "users",
			Typeflag: TypeReg,
		},
		fm: 0600,
	}}

	for i, v := range vectors {
		fi := v.h.FileInfo()
		h2, err := FileInfoHeader(fi, "")
		if err != nil {
			t.Error(err)
			continue
		}
		if strings.Contains(fi.Name(), "/") {
			t.Errorf("FileInfo of %q contains slash: %q", v.h.Name, fi.Name())
		}
		name := path.Base(v.h.Name)
		if fi.IsDir() {
			name += "/"
		}
		if got, want := h2.Name, name; got != want {
			t.Errorf("i=%d: Name: got %v, want %v", i, got, want)
		}
		if got, want := h2.Size, v.h.Size; got != want {
			t.Errorf("i=%d: Size: got %v, want %v", i, got, want)
		}
		if got, want := h2.Uid, v.h.Uid; got != want {
			t.Errorf("i=%d: Uid: got %d, want %d", i, got, want)
		}
		if got, want := h2.Gid, v.h.Gid; got != want {
			t.Errorf("i=%d: Gid: got %d, want %d", i, got, want)
		}
		if got, want := h2.Uname, v.h.Uname; got != want {
			t.Errorf("i=%d: Uname: got %q, want %q", i, got, want)
		}
		if got, want := h2.Gname, v.h.Gname; got != want {
			t.Errorf("i=%d: Gname: got %q, want %q", i, got, want)
		}
		if got, want := h2.Linkname, v.h.Linkname; got != want {
			t.Errorf("i=%d: Linkname: got %v, want %v", i, got, want)
		}
		if got, want := h2.Typeflag, v.h.Typeflag; got != want {
			t.Logf("%#v %#v", v.h, fi.Sys())
			t.Errorf("i=%d: Typeflag: got %q, want %q", i, got, want)
		}
		if got, want := h2.Mode, v.h.Mode; got != want {
			t.Errorf("i=%d: Mode: got %o, want %o", i, got, want)
		}
		if got, want := fi.Mode(), v.fm; got != want {
			t.Errorf("i=%d: fi.Mode: got %o, want %o", i, got, want)
		}
		if got, want := h2.AccessTime, v.h.AccessTime; got != want {
			t.Errorf("i=%d: AccessTime: got %v, want %v", i, got, want)
		}
		if got, want := h2.ChangeTime, v.h.ChangeTime; got != want {
			t.Errorf("i=%d: ChangeTime: got %v, want %v", i, got, want)
		}
		if got, want := h2.ModTime, v.h.ModTime; got != want {
			t.Errorf("i=%d: ModTime: got %v, want %v", i, got, want)
		}
		if sysh, ok := fi.Sys().(*Header); !ok || sysh != v.h {
			t.Errorf("i=%d: Sys didn't return original *Header", i)
		}
	}
}

func TestHeaderAllowedFormats(t *testing.T) {
	prettyFormat := func(f int) string {
		if f == formatUnknown {
			return "(formatUnknown)"
		}
		var fs []string
		if f&formatUSTAR > 0 {
			fs = append(fs, "formatUSTAR")
		}
		if f&formatPAX > 0 {
			fs = append(fs, "formatPAX")
		}
		if f&formatGNU > 0 {
			fs = append(fs, "formatGNU")
		}
		return "(" + strings.Join(fs, " | ") + ")"
	}

	vectors := []struct {
		header  *Header           // Input header
		paxHdrs map[string]string // Expected PAX headers that may be needed
		formats int               // Expected formats that can encode the header
	}{{
		header:  &Header{},
		formats: formatUSTAR | formatPAX | formatGNU,
	}, {
		header:  &Header{Size: 077777777777},
		formats: formatUSTAR | formatPAX | formatGNU,
	}, {
		header:  &Header{Size: 077777777777 + 1},
		paxHdrs: map[string]string{paxSize: "8589934592"},
		formats: formatPAX | formatGNU,
	}, {
		header:  &Header{Mode: 07777777},
		formats: formatUSTAR | formatPAX | formatGNU,
	}, {
		header:  &Header{Mode: 07777777 + 1},
		formats: formatGNU,
	}, {
		header:  &Header{Devmajor: -123},
		formats: formatGNU,
	}, {
		header:  &Header{Devmajor: 1<<56 - 1},
		formats: formatGNU,
	}, {
		header:  &Header{Devmajor: 1 << 56},
		formats: formatUnknown,
	}, {
		header:  &Header{Devmajor: -1 << 56},
		formats: formatGNU,
	}, {
		header:  &Header{Devmajor: -1<<56 - 1},
		formats: formatUnknown,
	}, {
		header:  &Header{Name: "用戶名", Devmajor: -1 << 56},
		formats: formatGNU,
	}, {
		header:  &Header{Size: math.MaxInt64},
		paxHdrs: map[string]string{paxSize: "9223372036854775807"},
		formats: formatPAX | formatGNU,
	}, {
		header:  &Header{Size: math.MinInt64},
		paxHdrs: map[string]string{paxSize: "-9223372036854775808"},
		formats: formatUnknown,
	}, {
		header:  &Header{Uname: "0123456789abcdef0123456789abcdef"},
		formats: formatUSTAR | formatPAX | formatGNU,
	}, {
		header:  &Header{Uname: "0123456789abcdef0123456789abcdefx"},
		paxHdrs: map[string]string{paxUname: "0123456789abcdef0123456789abcdefx"},
		formats: formatPAX,
	}, {
		header:  &Header{Name: "foobar"},
		formats: formatUSTAR | formatPAX | formatGNU,
	}, {
		header:  &Header{Name: strings.Repeat("a", nameSize)},
		formats: formatUSTAR | formatPAX | formatGNU,
	}, {
		header:  &Header{Name: strings.Repeat("a", nameSize+1)},
		paxHdrs: map[string]string{paxPath: strings.Repeat("a", nameSize+1)},
		formats: formatPAX | formatGNU,
	}, {
		header:  &Header{Linkname: "用戶名"},
		paxHdrs: map[string]string{paxLinkpath: "用戶名"},
		formats: formatPAX | formatGNU,
	}, {
		header:  &Header{Linkname: strings.Repeat("用戶名\x00", nameSize)},
		paxHdrs: map[string]string{paxLinkpath: strings.Repeat("用戶名\x00", nameSize)},
		formats: formatUnknown,
	}, {
		header:  &Header{Linkname: "\x00hello"},
		paxHdrs: map[string]string{paxLinkpath: "\x00hello"},
		formats: formatUnknown,
	}, {
		header:  &Header{Uid: 07777777},
		formats: formatUSTAR | formatPAX | formatGNU,
	}, {
		header:  &Header{Uid: 07777777 + 1},
		paxHdrs: map[string]string{paxUid: "2097152"},
		formats: formatPAX | formatGNU,
	}, {
		header:  &Header{Xattrs: nil},
		formats: formatUSTAR | formatPAX | formatGNU,
	}, {
		header:  &Header{Xattrs: map[string]string{"foo": "bar"}},
		paxHdrs: map[string]string{paxXattr + "foo": "bar"},
		formats: formatPAX,
	}, {
		header:  &Header{Xattrs: map[string]string{"用戶名": "\x00hello"}},
		paxHdrs: map[string]string{paxXattr + "用戶名": "\x00hello"},
		formats: formatPAX,
	}, {
		header:  &Header{Xattrs: map[string]string{"foo=bar": "baz"}},
		formats: formatUnknown,
	}, {
		header:  &Header{Xattrs: map[string]string{"foo": ""}},
		formats: formatUnknown,
	}, {
		header:  &Header{ModTime: time.Unix(0, 0)},
		formats: formatUSTAR | formatPAX | formatGNU,
	}, {
		header:  &Header{ModTime: time.Unix(077777777777, 0)},
		formats: formatUSTAR | formatPAX | formatGNU,
	}, {
		header:  &Header{ModTime: time.Unix(077777777777+1, 0)},
		paxHdrs: map[string]string{paxMtime: "8589934592"},
		formats: formatPAX | formatGNU,
	}, {
		header:  &Header{ModTime: time.Unix(math.MaxInt64, 0)},
		paxHdrs: map[string]string{paxMtime: "9223372036854775807"},
		formats: formatPAX | formatGNU,
	}, {
		header:  &Header{ModTime: time.Unix(-1, 0)},
		paxHdrs: map[string]string{paxMtime: "-1"},
		formats: formatPAX | formatGNU,
	}, {
		header:  &Header{ModTime: time.Unix(-1, 500)},
		paxHdrs: map[string]string{paxMtime: "-0.9999995"},
		formats: formatPAX,
	}, {
		header:  &Header{AccessTime: time.Unix(0, 0)},
		paxHdrs: map[string]string{paxAtime: "0"},
		formats: formatPAX | formatGNU,
	}, {
		header:  &Header{AccessTime: time.Unix(-123, 0)},
		paxHdrs: map[string]string{paxAtime: "-123"},
		formats: formatPAX | formatGNU,
	}, {
		header:  &Header{ChangeTime: time.Unix(123, 456)},
		paxHdrs: map[string]string{paxCtime: "123.000000456"},
		formats: formatPAX,
	}}

	for i, v := range vectors {
		formats, paxHdrs := v.header.allowedFormats()
		if formats != v.formats {
			t.Errorf("test %d, allowedFormats(...): got %v, want %v", i, prettyFormat(formats), prettyFormat(v.formats))
		}
		if formats&formatPAX > 0 && !reflect.DeepEqual(paxHdrs, v.paxHdrs) && !(len(paxHdrs) == 0 && len(v.paxHdrs) == 0) {
			t.Errorf("test %d, allowedFormats(...):\ngot  %v\nwant %s", i, paxHdrs, v.paxHdrs)
		}
	}
}
