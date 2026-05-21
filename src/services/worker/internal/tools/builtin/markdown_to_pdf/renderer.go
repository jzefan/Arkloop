package markdowntopdf

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	ghtml "github.com/yuin/goldmark/renderer/html"
)

// RenderOptions controls a single Markdown→PDF conversion.
type RenderOptions struct {
	// Ctx is used for image download cancellation and chromedp lifecycle.
	Ctx context.Context
	// Title appears as the document's <title> and first H1 (if the markdown
	// does not already begin with one).
	Title string
	// Markdown is the source content.
	Markdown string
	// AllowedImageRoots limits which local filesystem paths may be loaded.
	// Remote (http/https) and data-URI images are always allowed.
	AllowedImageRoots []string
	// Logger, if non-nil, receives debug messages from the renderer.
	Logger *slog.Logger
}

var mdParser = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		extension.Typographer,
	),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
	),
	goldmark.WithRendererOptions(
		ghtml.WithXHTML(),
		ghtml.WithUnsafe(),
	),
)

// Render converts Markdown to a PDF byte stream using Goldmark (Markdown → HTML)
// and a headless Chromium instance (HTML → PDF via CDP PrintToPDF).
func Render(opts RenderOptions) ([]byte, error) {
	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var htmlBuf bytes.Buffer
	if err := mdParser.Convert([]byte(opts.Markdown), &htmlBuf); err != nil {
		return nil, fmt.Errorf("markdown to html: %w", err)
	}

	doc, err := assembleDocument(opts.Title, htmlBuf.String())
	if err != nil {
		return nil, fmt.Errorf("assemble document: %w", err)
	}

	return printToPDF(ctx, doc, logger)
}

// docTmpl is the full HTML document template. The body is already safe HTML
// produced by goldmark, so it is wrapped in template.HTML to skip escaping.
var docTmpl = template.Must(template.New("pdf").Parse(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{.Title}}</title>
<style>
/* ───── Reset ───── */
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

/* ───── Typography ───── */
body {
  font-family:
    'AR PL UMing CN', 'AR PL UKai CN', 'WenQuanYi Zen Hei',
    'Noto Serif CJK SC', 'Source Han Serif SC',
    'Songti SC', 'STSong', 'SimSun', '宋体',
    Georgia, 'Times New Roman', serif;
  font-size: 11pt;
  line-height: 1.85;
  color: #1a1a1a;
  background: #fff;
  word-break: break-word;
  overflow-wrap: break-word;
}

/* ───── Headings ───── */
h1, h2, h3, h4, h5, h6 {
  font-weight: bold;
  line-height: 1.4;
  break-after: avoid;
  page-break-after: avoid;
  margin-top: 1.5em;
  margin-bottom: 0.5em;
}
h1:first-child { margin-top: 0; }

h1 {
  font-size: 20pt;
  text-align: center;
  padding-bottom: 8pt;
  border-bottom: 2px solid #222;
  margin-bottom: 18pt;
}

h2 {
  font-size: 15pt;
}

h3 { font-size: 13pt; }
h4 { font-size: 12pt; }
h5, h6 { font-size: 11pt; }

/* ───── Paragraphs ───── */
p {
  margin-bottom: 0.75em;
  text-align: justify;
  text-indent: 2em;
  orphans: 3;
  widows: 3;
}
h1 + p, h2 + p, h3 + p, h4 + p, h5 + p, h6 + p,
blockquote p, li p {
  text-indent: 0;
}

/* ───── Lists ───── */
ul, ol {
  margin: 0 0 0.75em 0;
  padding-left: 2em;
}
li {
  margin-bottom: 0.3em;
  line-height: 1.85;
}
li > ul, li > ol { margin: 0.3em 0 0; }

/* ───── Tables ───── */
table {
  width: 100%;
  border-collapse: collapse;
  margin: 1em 0;
  font-size: 10pt;
  break-inside: avoid;
  page-break-inside: avoid;
}
th {
  background: #f0f0f0;
  font-weight: bold;
  text-align: center;
  padding: 6pt 8pt;
  border: 1px solid #aaa;
}
td {
  padding: 5pt 8pt;
  border: 1px solid #aaa;
  vertical-align: top;
}
tr:nth-child(even) td { background: #f8f8f8; }

/* ───── Code ───── */
code {
  font-family: 'Noto Sans Mono', 'DejaVu Sans Mono', 'Courier New', monospace;
  font-size: 9.5pt;
  background: #f5f5f5;
  padding: 1pt 4pt;
  border-radius: 2pt;
}
pre {
  background: #f5f5f5;
  padding: 10pt 12pt;
  margin: 0.8em 0;
  border-left: 3px solid #bbb;
  white-space: pre-wrap;
  word-break: break-all;
  break-inside: avoid;
  page-break-inside: avoid;
}
pre code {
  background: none;
  padding: 0;
  font-size: 9pt;
  line-height: 1.6;
}

/* ───── Blockquotes ───── */
blockquote {
  margin: 0.8em 0;
  padding: 6pt 14pt;
  border-left: 4px solid #bbb;
  background: #f9f9f9;
  color: #444;
  break-inside: avoid;
  page-break-inside: avoid;
}
blockquote p { margin-bottom: 0.3em; }
blockquote p:last-child { margin-bottom: 0; }

/* ───── Horizontal rule ───── */
hr {
  border: none;
  border-top: 1px solid #ccc;
  margin: 1.4em 0;
}

/* ───── Links ───── */
a { color: #1d4ed8; text-decoration: none; }

/* ───── Images ───── */
img { max-width: 100%; display: block; margin: 0.8em auto; }

/* ───── Inline emphasis ───── */
strong, b { font-weight: bold; }
em, i { font-style: italic; }

/* ───── Print helpers ───── */
@media print {
  body { print-color-adjust: exact; -webkit-print-color-adjust: exact; }
  h1, h2, h3 { break-after: avoid; page-break-after: avoid; }
  table, pre, blockquote { break-inside: avoid; page-break-inside: avoid; }
}
</style>
</head>
<body>
{{.Body}}
</body>
</html>`))

type docData struct {
	Title string
	Body  template.HTML
}

func assembleDocument(title, htmlBody string) (string, error) {
	var buf bytes.Buffer
	if err := docTmpl.Execute(&buf, docData{
		Title: title,
		Body:  template.HTML(htmlBody), // goldmark output is safe HTML
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// chromiumFlags are appended to chromedp.DefaultExecAllocatorOptions.
// --no-sandbox is required for Docker; it is harmless on macOS.
var chromiumFlags = []chromedp.ExecAllocatorOption{
	chromedp.NoSandbox,
	chromedp.Flag("run-all-compositor-stages-before-draw", true),
}

func printToPDF(ctx context.Context, htmlContent string, logger *slog.Logger) ([]byte, error) {
	// Write HTML to a temp file so Chrome can access it via a file:// URL.
	// Using file:// avoids data-URI length limits and allows relative image paths.
	tmpFile, err := os.CreateTemp("", "arkloop-pdf-*.html")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.WriteString(htmlContent); err != nil {
		tmpFile.Close()
		return nil, fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()

	allocOpts := append(chromedp.DefaultExecAllocatorOptions[:], chromiumFlags...)
	if cp := strings.TrimSpace(os.Getenv("ARK_CHROMIUM_PATH")); cp != "" {
		allocOpts = append(allocOpts, chromedp.ExecPath(cp))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, allocOpts...)
	defer cancelAlloc()

	taskCtx, cancelTask := chromedp.NewContext(allocCtx,
		chromedp.WithLogf(func(format string, args ...any) {
			logger.Debug("chromedp: " + fmt.Sprintf(format, args...))
		}),
	)
	defer cancelTask()

	printCtx, cancelPrint := context.WithTimeout(taskCtx, 2*time.Minute)
	defer cancelPrint()

	fileURL := "file://" + tmpFile.Name()

	var pdfBytes []byte
	if err := chromedp.Run(printCtx,
		chromedp.Navigate(fileURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// A4: 210mm × 297mm = 8.2677 × 11.6929 inches
			buf, _, err := page.PrintToPDF().
				WithPrintBackground(true).
				WithPaperWidth(8.27).
				WithPaperHeight(11.69).
				WithMarginTop(1.0).
				WithMarginBottom(1.0).
				WithMarginLeft(0.98).
				WithMarginRight(0.98).
				WithDisplayHeaderFooter(true).
				WithHeaderTemplate(`<div></div>`).
				WithFooterTemplate(`<div style="font-size:9px;color:#999;text-align:center;width:100%;margin:0 20px;font-family:Arial,sans-serif;">第 <span class="pageNumber"></span> 页，共 <span class="totalPages"></span> 页</div>`).
				Do(ctx)
			if err != nil {
				return err
			}
			pdfBytes = buf
			return nil
		}),
	); err != nil {
		return nil, err
	}

	return pdfBytes, nil
}
