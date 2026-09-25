package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// noSeek hides the Seeker interface so a reader behaves like a pipe.
type noSeek struct{ io.Reader }

// chunkReader returns at most n bytes per Read, like a slow pipe.
type chunkReader struct {
	r io.Reader
	n int
}

func (c chunkReader) Read(p []byte) (int, error) {
	if len(p) > c.n {
		p = p[:c.n]
	}
	return c.r.Read(p)
}

// eofWithData returns its whole payload together with io.EOF in one call,
// which the io.Reader contract permits.
type eofWithData struct {
	data []byte
	done bool
}

func (e *eofWithData) Read(p []byte) (int, error) {
	if e.done {
		return 0, io.EOF
	}
	e.done = true
	return copy(p, e.data), io.EOF
}

func defaults() options {
	o, err := parseFlags(nil, io.Discard)
	if err != nil {
		panic(err)
	}
	return o
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCopyStdinToStdout(t *testing.T) {
	var out bytes.Buffer
	st, err := run(defaults(), strings.NewReader("hello"), &out)
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "hello" {
		t.Errorf("got %q, want %q", out.String(), "hello")
	}
	want := stats{inPartial: 1, outPartial: 1, bytes: 5}
	if st != want {
		t.Errorf("stats = %+v, want %+v", st, want)
	}
}

func TestOutputFileIsTruncated(t *testing.T) {
	p := writeTemp(t, "out", "AAAAAAAAAA")
	o := defaults()
	o.ofile = p
	if _, err := run(o, strings.NewReader("bb"), nil); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, p); got != "bb" {
		t.Errorf("got %q, want %q", got, "bb")
	}
}

func TestNotruncKeepsTail(t *testing.T) {
	p := writeTemp(t, "out", "AAAAAAAAAA")
	o := defaults()
	o.ofile = p
	o.notrunc = true
	if _, err := run(o, strings.NewReader("bb"), nil); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, p); got != "bbAAAAAAAA" {
		t.Errorf("got %q, want %q", got, "bbAAAAAAAA")
	}
}

func TestOseekTruncatesAtOffset(t *testing.T) {
	p := writeTemp(t, "out", "ABCDEFGHIJ")
	o := defaults()
	o.ofile = p
	o.oseek = 2
	if _, err := run(o, strings.NewReader("xy"), nil); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, p); got != "ABxy" {
		t.Errorf("got %q, want %q", got, "ABxy")
	}
}

func TestOseekNotrunc(t *testing.T) {
	p := writeTemp(t, "out", "ABCDEFGHIJ")
	o := defaults()
	o.ofile = p
	o.oseek = 2
	o.notrunc = true
	if _, err := run(o, strings.NewReader("xy"), nil); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, p); got != "ABxyEFGHIJ" {
		t.Errorf("got %q, want %q", got, "ABxyEFGHIJ")
	}
}

func TestAppend(t *testing.T) {
	p := writeTemp(t, "out", "ABC")
	o := defaults()
	o.ofile = p
	o.appendOut = true
	if _, err := run(o, strings.NewReader("z"), nil); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, p); got != "ABCz" {
		t.Errorf("got %q, want %q", got, "ABCz")
	}
}

func TestCreatesMissingOutput(t *testing.T) {
	p := filepath.Join(t.TempDir(), "new")
	o := defaults()
	o.ofile = p
	if _, err := run(o, strings.NewReader("data"), nil); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, p); got != "data" {
		t.Errorf("got %q, want %q", got, "data")
	}
}

func TestSkipAndIseekCompose(t *testing.T) {
	p := writeTemp(t, "in", "0123456789")
	o := defaults()
	o.ifile = p
	o.bs = 2
	o.iseek = 2
	o.skip = 1
	var out bytes.Buffer
	if _, err := run(o, nil, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "456789" {
		t.Errorf("got %q, want %q", out.String(), "456789")
	}
}

func TestSkipOnPipe(t *testing.T) {
	o := defaults()
	o.bs = 2
	o.skip = 1
	o.iseek = 1
	var out bytes.Buffer
	if _, err := run(o, noSeek{strings.NewReader("0123456789")}, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "3456789" {
		t.Errorf("got %q, want %q", out.String(), "3456789")
	}
}

func TestSkipPastEOF(t *testing.T) {
	o := defaults()
	o.skip = 5
	var out bytes.Buffer
	st, err := run(o, noSeek{strings.NewReader("short")}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 || st.bytes != 0 {
		t.Errorf("expected no output, got %q with stats %+v", out.String(), st)
	}
}

func TestCount(t *testing.T) {
	o := defaults()
	o.bs = 3
	o.count = 2
	var out bytes.Buffer
	st, err := run(o, strings.NewReader("0123456789"), &out)
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "012345" {
		t.Errorf("got %q, want %q", out.String(), "012345")
	}
	want := stats{inFull: 2, outFull: 2, bytes: 6}
	if st != want {
		t.Errorf("stats = %+v, want %+v", st, want)
	}
}

func TestCountZeroCopiesNothing(t *testing.T) {
	o := defaults()
	o.count = 0
	var out bytes.Buffer
	if _, err := run(o, strings.NewReader("data"), &out); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Errorf("got %q, want empty output", out.String())
	}
}

func TestPartialReadsCountAsBlocks(t *testing.T) {
	o := defaults()
	o.bs = 4
	o.count = 2
	var out bytes.Buffer
	st, err := run(o, chunkReader{strings.NewReader("0123456789"), 2}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "0123" {
		t.Errorf("got %q, want %q", out.String(), "0123")
	}
	if st.inPartial != 2 || st.inFull != 0 {
		t.Errorf("stats = %+v, want 2 partial records", st)
	}
}

func TestFullblock(t *testing.T) {
	o := defaults()
	o.bs = 4
	o.count = 2
	o.fullblock = true
	var out bytes.Buffer
	st, err := run(o, chunkReader{strings.NewReader("0123456789"), 2}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "01234567" {
		t.Errorf("got %q, want %q", out.String(), "01234567")
	}
	if st.inFull != 2 || st.inPartial != 0 {
		t.Errorf("stats = %+v, want 2 full records", st)
	}
}

func TestFullblockTrailingPartial(t *testing.T) {
	o := defaults()
	o.bs = 4
	o.fullblock = true
	var out bytes.Buffer
	st, err := run(o, chunkReader{strings.NewReader("0123456789"), 3}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "0123456789" {
		t.Errorf("got %q, want %q", out.String(), "0123456789")
	}
	want := stats{inFull: 2, inPartial: 1, outFull: 2, outPartial: 1, bytes: 10}
	if st != want {
		t.Errorf("stats = %+v, want %+v", st, want)
	}
}

func TestDataReturnedWithEOFIsWritten(t *testing.T) {
	var out bytes.Buffer
	if _, err := run(defaults(), &eofWithData{data: []byte("tail")}, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "tail" {
		t.Errorf("got %q, want %q", out.String(), "tail")
	}
}

func TestReadDirectoryFails(t *testing.T) {
	o := defaults()
	o.ifile = t.TempDir()
	if _, err := run(o, nil, io.Discard); err == nil {
		t.Fatal("expected an error reading a directory")
	}
}

func TestMissingInputFails(t *testing.T) {
	o := defaults()
	o.ifile = filepath.Join(t.TempDir(), "missing")
	_, err := run(o, nil, io.Discard)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("got %v, want a not-exist error", err)
	}
}

func TestSeekOnUnseekableOutputFails(t *testing.T) {
	o := defaults()
	o.oseek = 1
	if _, err := run(o, strings.NewReader("x"), &bytes.Buffer{}); err == nil {
		t.Fatal("expected an error seeking an unseekable output")
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*options)
		ok     bool
	}{
		{"defaults", func(o *options) {}, true},
		{"bs zero", func(o *options) { o.bs = 0 }, false},
		{"bs negative", func(o *options) { o.bs = -1 }, false},
		{"count below -1", func(o *options) { o.count = -2 }, false},
		{"count -1", func(o *options) { o.count = -1 }, true},
		{"skip negative", func(o *options) { o.skip = -1 }, false},
		{"iseek negative", func(o *options) { o.iseek = -1 }, false},
		{"oseek negative", func(o *options) { o.oseek = -1 }, false},
		{"append with oseek", func(o *options) { o.appendOut = true; o.oseek = 1 }, false},
		{"append alone", func(o *options) { o.appendOut = true }, true},
		{"skip overflow", func(o *options) { o.bs = 1 << 20; o.skip = 1 << 50 }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := defaults()
			tc.mutate(&o)
			err := o.validate()
			if tc.ok && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tc.ok && err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestParseFlags(t *testing.T) {
	o, err := parseFlags([]string{"-if=in", "-of=out", "-bs=4", "-count=3", "-skip=1",
		"-iseek=2", "-oseek=5", "-notrunc", "-fullblock", "-status"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	want := options{ifile: "in", ofile: "out", bs: 4, count: 3, skip: 1, iseek: 2, oseek: 5,
		notrunc: true, fullblock: true, status: true}
	if o != want {
		t.Errorf("got %+v, want %+v", o, want)
	}

	if _, err := parseFlags([]string{"-bs=0"}, io.Discard); err == nil {
		t.Error("expected -bs=0 to be rejected")
	}
	if _, err := parseFlags([]string{"stray"}, io.Discard); err == nil {
		t.Error("expected a positional argument to be rejected")
	}
	if _, err := parseFlags([]string{"-nope"}, io.Discard); err == nil {
		t.Error("expected an unknown flag to be rejected")
	}
}

func TestStatsString(t *testing.T) {
	got := stats{inFull: 2, inPartial: 1, outFull: 2, outPartial: 1, bytes: 10}.String()
	want := "2+1 records in\n2+1 records out\n10 bytes copied\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
