# go-dd
`go-dd` is a Go language implementation of the dd command-line utility, which is used for copying and converting data. It supports many of the same options as dd, including setting the input and output files, block size, number of blocks to copy, and skipping or seeking to specific positions in the data.

## Usage
`go-dd [OPTIONS]`

## Options
`-if`: specify the input file to read from (default is stdin)
`-of`: specify the output file to write to (default is stdout)
`-bs`: set the block size in bytes (default is 512)
`-count`: number of blocks to copy (default is -1, meaning all blocks)
`-skip`: number of blocks to skip at start (default is 0)
`-iseek`: skip input data until 'iseek' bytes (default is 0)
`-oseek`: skip output data until 'oseek' bytes (default is 0)
`-append`: append to output file instead of overwriting it (default is false)

## License
go-dd is released under the Apache 2.0 License, which can be found in the LICENSE file.

## Credits
go-dd is created by Andrew Kuehne and is Copyright 2023.