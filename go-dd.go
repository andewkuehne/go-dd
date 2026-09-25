// Command go-dd is a Go implementation of the dd command-line utility.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
)

// options holds the parsed command-line configuration.
type options struct {
	ifile     string
	ofile     string
	bs        int
	count     int64
	skip      int64
	iseek     int64
	oseek     int64
	appendOut bool
	notrunc   bool
	fullblock bool
	status    bool
}

// stats records what was transferred, mirroring dd's summary.
type stats struct {
	inFull     int64
	inPartial  int64
	outFull    int64
	outPartial int64
	bytes      int64
}

func (s stats) String() string {
	return fmt.Sprintf("%d+%d records in\n%d+%d records out\n%d bytes copied\n",
		s.inFull, s.inPartial, s.outFull, s.outPartial, s.bytes)
}

func main() {
	opts, err := parseFlags(os.Args[1:], os.Stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	st, err := run(opts, os.Stdin, os.Stdout)
	if opts.status {
		fmt.Fprint(os.Stderr, st)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// parseFlags parses args into options. Usage output goes to errOut.
func parseFlags(args []string, errOut io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet("go-dd", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.StringVar(&o.ifile, "if", "", "read from infile instead of stdin")
	fs.StringVar(&o.ofile, "of", "", "write to outfile instead of stdout")
	fs.IntVar(&o.bs, "bs", 512, "set the block size in bytes")
	fs.Int64Var(&o.count, "count", -1, "number of blocks to copy (-1 copies everything)")
	fs.Int64Var(&o.skip, "skip", 0, "number of blocks to skip at start of input")
	fs.Int64Var(&o.iseek, "iseek", 0, "number of bytes to skip at start of input (applied before -skip)")
	fs.Int64Var(&o.oseek, "oseek", 0, "number of bytes to skip at start of output")
	fs.BoolVar(&o.appendOut, "append", false, "append to outfile instead of overwriting it")
	fs.BoolVar(&o.notrunc, "notrunc", false, "do not truncate the output file")
	fs.BoolVar(&o.fullblock, "fullblock", false, "accumulate full input blocks before counting them")
	fs.BoolVar(&o.status, "status", false, "print a transfer summary to stderr when done")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if fs.NArg() > 0 {
		return o, fmt.Errorf("unexpected argument: %q", fs.Arg(0))
	}
	return o, o.validate()
}

func (o options) validate() error {
	if o.bs <= 0 {
		return fmt.Errorf("block size must be greater than zero, got %d", o.bs)
	}
	if o.count < -1 {
		return fmt.Errorf("count must be -1 or greater, got %d", o.count)
	}
	if o.skip < 0 {
		return fmt.Errorf("skip must not be negative, got %d", o.skip)
	}
	if o.iseek < 0 {
		return fmt.Errorf("iseek must not be negative, got %d", o.iseek)
	}
	if o.oseek < 0 {
		return fmt.Errorf("oseek must not be negative, got %d", o.oseek)
	}
	if o.appendOut && o.oseek > 0 {
		return errors.New("-append and -oseek cannot be used together")
	}
	if o.skip > 0 && o.skip > math.MaxInt64/int64(o.bs) {
		return fmt.Errorf("skip of %d blocks of %d bytes overflows", o.skip, o.bs)
	}
	if o.iseek > math.MaxInt64-o.skip*int64(o.bs) {
		return errors.New("combined iseek and skip offset overflows")
	}
	return nil
}

// run performs the copy described by o. When no input or output file is
// given it uses stdin and stdout, which are the inherited descriptors in
// main and arbitrary readers and writers in tests.
func run(o options, stdin io.Reader, stdout io.Writer) (stats, error) {
	if err := o.validate(); err != nil {
		return stats{}, err
	}

	in := stdin
	if o.ifile != "" {
		f, err := os.Open(o.ifile)
		if err != nil {
			return stats{}, fmt.Errorf("could not open input file: %w", err)
		}
		defer f.Close()
		in = f
	}

	out := stdout
	closeOut := func() error { return nil }
	if o.ofile != "" {
		f, err := openOutput(o)
		if err != nil {
			return stats{}, err
		}
		out = f
		closeOut = f.Close
	} else if err := positionOutput(out, o); err != nil {
		return stats{}, err
	}

	st, err := copyBlocks(in, out, o)
	if cerr := closeOut(); err == nil && cerr != nil {
		err = fmt.Errorf("could not close output file: %w", cerr)
	}
	return st, err
}

// openOutput opens o.ofile, positions it, and truncates it as dd would.
func openOutput(o options) (*os.File, error) {
	flags := os.O_WRONLY | os.O_CREATE
	if o.appendOut {
		flags |= os.O_APPEND
	}
	f, err := os.OpenFile(o.ofile, flags, 0o644)
	if err != nil {
		return nil, fmt.Errorf("could not open output file: %w", err)
	}
	if o.appendOut {
		return f, nil
	}
	if err := positionOutput(f, o); err != nil {
		f.Close()
		return nil, err
	}
	if !o.notrunc {
		if err := truncateRegular(f, o.oseek); err != nil {
			f.Close()
			return nil, fmt.Errorf("could not truncate output file: %w", err)
		}
	}
	return f, nil
}

// positionOutput applies -append or -oseek to a writer that supports seeking.
func positionOutput(w io.Writer, o options) error {
	if !o.appendOut && o.oseek == 0 {
		return nil
	}
	s, ok := w.(io.Seeker)
	if !ok {
		return errors.New("output does not support seeking")
	}
	if o.appendOut {
		if _, err := s.Seek(0, io.SeekEnd); err != nil {
			return fmt.Errorf("could not seek to end of output: %w", err)
		}
		return nil
	}
	if _, err := s.Seek(o.oseek, io.SeekStart); err != nil {
		return fmt.Errorf("could not seek to offset %d in output: %w", o.oseek, err)
	}
	return nil
}

// truncateRegular truncates f to size bytes if it is a regular file.
// Devices and pipes cannot be truncated, and dd skips them too.
func truncateRegular(f *os.File, size int64) error {
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return nil
	}
	return f.Truncate(size)
}

// skipInput advances in by n bytes. It seeks when it can and otherwise
// reads and discards, so that -skip and -iseek work on pipes.
func skipInput(in io.Reader, n int64) error {
	if n <= 0 {
		return nil
	}
	if s, ok := in.(io.Seeker); ok {
		if _, err := s.Seek(n, io.SeekCurrent); err == nil {
			return nil
		}
	}
	_, err := io.CopyN(io.Discard, in, n)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("could not skip input data: %w", err)
	}
	return nil
}

// copyBlocks copies blocks from in to out according to o and reports what
// was transferred. It is the equivalent of dd's main loop.
func copyBlocks(in io.Reader, out io.Writer, o options) (stats, error) {
	var st stats
	if err := skipInput(in, o.iseek+o.skip*int64(o.bs)); err != nil {
		return st, err
	}

	buf := make([]byte, o.bs)
	for o.count < 0 || st.inFull+st.inPartial < o.count {
		var n int
		var err error
		if o.fullblock {
			n, err = io.ReadFull(in, buf)
		} else {
			n, err = in.Read(buf)
		}
		if n > 0 {
			if n == len(buf) {
				st.inFull++
			} else {
				st.inPartial++
			}
			if _, werr := out.Write(buf[:n]); werr != nil {
				return st, fmt.Errorf("could not write output data: %w", werr)
			}
			if n == len(buf) {
				st.outFull++
			} else {
				st.outPartial++
			}
			st.bytes += int64(n)
		}
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return st, fmt.Errorf("could not read input data: %w", err)
		}
	}
	return st, nil
}
