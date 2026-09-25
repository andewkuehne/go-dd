# go-dd

`go-dd` is a Go implementation of the dd command-line utility, which is used for copying data. It supports many of the same options as dd, including setting the input and output files, block size, number of blocks to copy, and skipping or seeking to specific positions in the data.

## Install

```
go install github.com/andewkuehne/go-dd@latest
```

Or build from a checkout:

```
go build
```

## Usage

`go-dd [OPTIONS]`

With no options, `go-dd` copies stdin to stdout in 512-byte blocks.

## Options

`-if`: input file to read from (default is stdin)

`-of`: output file to write to (default is stdout)

`-bs`: block size in bytes (default is 512)

`-count`: number of blocks to copy (default is -1, meaning all blocks)

`-skip`: number of blocks to skip at the start of the input (default is 0)

`-iseek`: number of bytes to skip at the start of the input, applied before `-skip` (default is 0)

`-oseek`: number of bytes to skip at the start of the output (default is 0)

`-append`: append to the output file instead of overwriting it. Cannot be combined with `-oseek`.

`-notrunc`: do not truncate the output file. By default the output file is truncated at the `-oseek` offset, as dd does.

`-fullblock`: keep reading until each block is full before counting it toward `-count`. Without this flag a short read from a pipe counts as one block, matching dd's default.

`-status`: print a transfer summary to stderr when the copy finishes

## Notes

Skipping works on pipes as well as files. When the input cannot be seeked, the skipped bytes are read and discarded.

Truncation only applies to regular files. Devices and pipes are left alone.

## License

go-dd is released under the Apache 2.0 License, which can be found in the LICENSE file.

## Credits

go-dd is created by Andrew Kuehne.
