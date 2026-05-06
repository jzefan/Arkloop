//go:build desktop && windows

package localshell

import (
	"io"

	"golang.org/x/sys/windows"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

func wrapProcessOutputWriter(dst io.Writer) io.Writer {
	enc := windowsProcessOutputEncoding()
	if enc == nil {
		return dst
	}
	return transform.NewWriter(dst, enc.NewDecoder())
}

func wrapProcessOutputReadCloser(src io.ReadCloser) io.ReadCloser {
	enc := windowsProcessOutputEncoding()
	if enc == nil {
		return src
	}
	return decodedReadCloser{
		Reader: transform.NewReader(src, enc.NewDecoder()),
		Closer: src,
	}
}

type decodedReadCloser struct {
	io.Reader
	io.Closer
}

func windowsProcessOutputEncoding() encoding.Encoding {
	if consoleCodePage, err := windows.GetConsoleOutputCP(); err == nil {
		if consoleCodePage == 65001 {
			// Console output is already UTF-8. Do not fall back to ACP decoders.
			return nil
		}
		if enc := processOutputEncodingForCodePage(consoleCodePage); enc != nil {
			return enc
		}
	}
	if enc := processOutputEncodingForCodePage(windows.GetACP()); enc != nil {
		return enc
	}
	return nil
}

func processOutputEncodingForCodePage(codePage uint32) encoding.Encoding {
	switch codePage {
	case 0, 65001:
		return nil
	case 437:
		return charmap.CodePage437
	case 850:
		return charmap.CodePage850
	case 852:
		return charmap.CodePage852
	case 866:
		return charmap.CodePage866
	case 874:
		return charmap.Windows874
	case 932:
		return japanese.ShiftJIS
	case 936:
		return simplifiedchinese.GB18030
	case 949:
		return korean.EUCKR
	case 950:
		return traditionalchinese.Big5
	case 1250:
		return charmap.Windows1250
	case 1251:
		return charmap.Windows1251
	case 1252:
		return charmap.Windows1252
	case 1253:
		return charmap.Windows1253
	case 1254:
		return charmap.Windows1254
	case 1255:
		return charmap.Windows1255
	case 1256:
		return charmap.Windows1256
	case 1257:
		return charmap.Windows1257
	case 1258:
		return charmap.Windows1258
	default:
		return nil
	}
}