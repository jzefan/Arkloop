//go:build desktop && !windows

package localshell

import "io"

func wrapProcessOutputWriter(dst io.Writer) io.Writer {
	return dst
}

func wrapProcessOutputReadCloser(src io.ReadCloser) io.ReadCloser {
	return src
}