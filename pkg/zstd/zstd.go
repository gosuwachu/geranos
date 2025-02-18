package zstd

import (
	"bufio"
	"io"

	"github.com/klauspost/compress/zstd"
)

// MagicHeader is the start of zstd files.
var MagicHeader = []byte{'\x28', '\xb5', '\x2f', '\xfd'}

// ReadCloser reads uncompressed input data from the io.ReadCloser and
// returns an io.ReadCloser from which compressed data may be read.
// This uses zstd level 1 for the compression.
func ReadCloser(r io.ReadCloser) io.ReadCloser {
	return ReadCloserLevel(r, 1)
}

// ReadCloserLevel reads uncompressed input data from the io.ReadCloser and
// returns an io.ReadCloser from which compressed data may be read.
func ReadCloserLevel(r io.ReadCloser, level int) io.ReadCloser {
	pr, pw := io.Pipe()

	// For highly compressible layers, zstd.Writer will output a very small
	// number of bytes per Write(). This is normally fine, but when pushing
	// to a registry, we want to ensure that we're taking full advantage of
	// the available bandwidth instead of sending tons of tiny writes over
	// the wire.
	bw := bufio.NewWriterSize(pw, 1<<20)

	// Returns err so we can pw.CloseWithError(err)
	go func() error {
		// TODO(go1.14): Just defer {pw,zw,r}.Close like you'd expect.
		// Context: https://golang.org/issue/24283
		zw, err := zstd.NewWriter(bw,
			zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)),
			zstd.WithEncoderConcurrency(1),
			zstd.WithZeroFrames(true),
		)
		if err != nil {
			return pw.CloseWithError(err)
		}

		buf := make([]byte, 64*1024)
		if _, err := io.CopyBuffer(zw, r, buf); err != nil {
			defer r.Close()
			defer zw.Close()
			return pw.CloseWithError(err)
		}

		// Close zstd writer to Flush it and write zstd trailers.
		if err := zw.Close(); err != nil {
			return pw.CloseWithError(err)
		}

		// Flush bufio writer to ensure we write out everything.
		if err := bw.Flush(); err != nil {
			return pw.CloseWithError(err)
		}

		// We don't really care if these fail.
		defer pw.Close()
		defer r.Close()

		return nil
	}()

	return pr
}
