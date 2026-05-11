package markdowntopdf

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	// Decoders: registered as side-effects via image.RegisterFormat.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

const (
	// maxImageBytes caps the size of any single image loaded into a PDF.
	// Keeps memory use bounded and prevents accidental huge downloads.
	maxImageBytes = 5 * 1024 * 1024
	// httpImageTimeout caps remote image fetch time.
	httpImageTimeout = 5 * time.Second
)

// LoadedImage holds decoded image metadata plus the original file bytes
// (what gopdf actually embeds into the PDF).
type LoadedImage struct {
	Data   []byte // original encoded bytes (JPEG/PNG/GIF)
	Format string // "jpeg", "png", "gif"
	Width  int    // in pixels
	Height int    // in pixels
}

// LoadImage resolves a markdown image reference into bytes suitable for
// gopdf.ImageFrom. The reference may be:
//
//   - http:// or https:// URL
//   - data:image/<type>;base64,<b64> URI
//   - absolute or relative filesystem path
//
// Callers supply a base directory for resolving relative paths. Relative
// paths that resolve outside of allowedRoots (when non-empty) are rejected.
//
// The returned error wraps a category marker so callers can decide whether
// to omit the image and continue rendering, or to fail the whole document.
func LoadImage(ctx context.Context, ref string, allowedRoots []string) (*LoadedImage, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, errors.New("empty image reference")
	}

	switch {
	case strings.HasPrefix(ref, "data:"):
		return loadDataURIImage(ref)
	case strings.HasPrefix(ref, "http://"), strings.HasPrefix(ref, "https://"):
		return loadHTTPImage(ctx, ref)
	default:
		return loadLocalImage(ref, allowedRoots)
	}
}

func loadDataURIImage(ref string) (*LoadedImage, error) {
	// Format: data:[<mime>][;base64],<payload>
	comma := strings.IndexByte(ref, ',')
	if comma < 0 {
		return nil, errors.New("malformed data URI: missing comma")
	}
	meta := ref[5:comma] // strip "data:"
	payload := ref[comma+1:]

	var data []byte
	if strings.Contains(meta, ";base64") {
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil, fmt.Errorf("decode data URI base64: %w", err)
		}
		data = decoded
	} else {
		unescaped, err := url.QueryUnescape(payload)
		if err != nil {
			return nil, fmt.Errorf("decode data URI percent-encoding: %w", err)
		}
		data = []byte(unescaped)
	}
	if len(data) > maxImageBytes {
		return nil, fmt.Errorf("data URI image exceeds %d bytes", maxImageBytes)
	}
	return decodeImageBytes(data)
}

func loadHTTPImage(ctx context.Context, ref string) (*LoadedImage, error) {
	reqCtx, cancel := context.WithTimeout(ctx, httpImageTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, ref, nil)
	if err != nil {
		return nil, fmt.Errorf("build image request: %w", err)
	}
	req.Header.Set("User-Agent", "arkloop-markdown-pdf/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("fetch image: http %d", resp.StatusCode)
	}
	// Use LimitReader + 1 extra byte so we can detect overflow rather than silently truncating.
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read image body: %w", err)
	}
	if len(data) > maxImageBytes {
		return nil, fmt.Errorf("image exceeds %d bytes", maxImageBytes)
	}
	return decodeImageBytes(data)
}

func loadLocalImage(ref string, allowedRoots []string) (*LoadedImage, error) {
	path := ref
	if strings.HasPrefix(path, "file://") {
		u, err := url.Parse(path)
		if err != nil {
			return nil, fmt.Errorf("parse file URL: %w", err)
		}
		path = u.Path
	}
	// Resolve to an absolute, symlink-normalised path for the containment
	// check. EvalSymlinks fails if the file doesn't exist, so fall back to
	// Abs when needed.
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	if len(allowedRoots) > 0 {
		if !isUnderAnyRoot(abs, allowedRoots) {
			return nil, fmt.Errorf("image path %q outside allowed roots", abs)
		}
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("stat image: %w", err)
	}
	if info.Size() > maxImageBytes {
		return nil, fmt.Errorf("image %q exceeds %d bytes", abs, maxImageBytes)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}
	return decodeImageBytes(data)
}

func isUnderAnyRoot(absPath string, roots []string) bool {
	for _, root := range roots {
		cleanRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		if resolved, err := filepath.EvalSymlinks(cleanRoot); err == nil {
			cleanRoot = resolved
		}
		rel, err := filepath.Rel(cleanRoot, absPath)
		if err != nil {
			continue
		}
		if !strings.HasPrefix(rel, "..") && rel != ".." {
			return true
		}
	}
	return false
}

// decodeImageBytes inspects the byte stream with image.DecodeConfig (which
// reads only the header) and records the format + pixel dimensions.
func decodeImageBytes(data []byte) (*LoadedImage, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	switch format {
	case "png", "jpeg", "gif":
		// gopdf supports these natively via ImageFrom.
	default:
		return nil, fmt.Errorf("unsupported image format: %s", format)
	}
	return &LoadedImage{
		Data:   data,
		Format: format,
		Width:  cfg.Width,
		Height: cfg.Height,
	}, nil
}
