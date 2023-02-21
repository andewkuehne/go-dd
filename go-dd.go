package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	var (
		ifile  string
		ofile  string
		bsize  int
		count  int64
		skip   int64
		iseek  int64
		oseek  int64
		append bool
	)
	flag.StringVar(&ifile, "if", "", "read from infile instead of stdin")
	flag.StringVar(&ofile, "of", "", "write to outfile instead of stdout")
	flag.IntVar(&bsize, "bs", 512, "set the block size in bytes")
	flag.Int64Var(&count, "count", -1, "number of blocks to copy")
	flag.Int64Var(&skip, "skip", 0, "number of blocks to skip at start")
	flag.Int64Var(&iseek, "iseek", 0, "skip input data until 'iseek' bytes")
	flag.Int64Var(&oseek, "oseek", 0, "skip output data until 'oseek' bytes")
	flag.BoolVar(&append, "append", false, "append to outfile instead of overwriting it")
	flag.Parse()

	if ifile == "" {
		ifile = "/dev/stdin"
	}
	if ofile == "" {
		ofile = "/dev/stdout"
	}

	in, err := os.Open(ifile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not open input file: %v\n", err)
		os.Exit(1)
	}
	defer in.Close()

	out, err := os.OpenFile(ofile, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not open output file: %v\n", err)
		os.Exit(1)
	}
	defer out.Close()

	if append {
		_, err = out.Seek(0, io.SeekEnd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: could not seek to end of output file: %v\n", err)
			os.Exit(1)
		}
	} else if oseek > 0 {
		_, err = out.Seek(oseek, io.SeekStart)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: could not seek to offset %d in output file: %v\n", oseek, err)
			os.Exit(1)
		}
	}

	if iseek > 0 {
		_, err = in.Seek(iseek, io.SeekStart)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: could not seek to offset %d in input file: %v\n", iseek, err)
			os.Exit(1)
		}
	}

	buf := make([]byte, bsize)

	if skip > 0 {
		_, err = in.Seek(skip*int64(bsize), io.SeekStart)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: could not seek to skip input data: %v\n", err)
			os.Exit(1)
		}
	}

	var i int64
	for i = 0; count < 0 || i < count; i++ {
		n, err := in.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintf(os.Stderr, "error: could not read input data: %v\n", err)
			os.Exit(1)
		}
		_, err = out.Write(buf[:n])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: could not write output data: %v\n", err)
			os.Exit(1)
		}
	}
}
