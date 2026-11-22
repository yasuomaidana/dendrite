package analysis

import (
	"context"
	"errors"
	"strings"

	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/rea"
)

// Image is what the dendrite-core sidecar says about a target of any format (see docs/specs/rea-bin-inspect-contract.md).
type Image = coreclient.Image

// Inspector reads a target of any format: a PE, ELF or Mach-O image, a package, a bundle, an unknown file or a
// directory of source. CoreInspector is the real one (the dendrite-core sidecar).
type Inspector interface {
	Inspect(ctx context.Context, path string) (*Image, error)
}

// ErrFormatUnsupported means the installed dendrite-core can only read PE images (it predates bin.inspect) and the
// target is not one.
var ErrFormatUnsupported = errors.New("this dendrite-core only reads PE images: run `dendrite update --dev` to read other formats")

// CoreInspector inspects through bin.inspect and, for a dendrite-core that predates it, through pe.parse, which
// still reads PE images.
type CoreInspector struct{ Client *coreclient.Client }

// Inspect reads the target at path.
func (c *CoreInspector) Inspect(ctx context.Context, path string) (*Image, error) {
	img, err := c.Client.Inspect(ctx, path)
	if !errors.Is(err, coreclient.ErrInspectUnsupported) {
		return img, err
	}
	pe, err := c.Client.PEParse(ctx, path)
	switch {
	case err == nil:
		return coreclient.FromPE(pe), nil
	case errors.Is(err, coreclient.ErrNotPE), errors.Is(err, coreclient.ErrPEUnsupported):
		return nil, ErrFormatUnsupported
	}
	return nil, err
}

// formats are the formats an analysis can be told to expect.
var formats = []string{coreclient.FormatPE, coreclient.FormatELF, coreclient.FormatMachO, coreclient.FormatFatMachO,
	coreclient.FormatASAR, coreclient.FormatZIP, coreclient.FormatJSBundle, coreclient.FormatSourceTree, coreclient.FormatUnknown}

// normalizeFormat reads the optional --format: empty or auto means whatever the target is, pe32 and pe64 are pe.
func normalizeFormat(format string) (string, error) {
	f := strings.ToLower(strings.TrimSpace(format))
	switch f {
	case "", "auto":
		return "", nil
	case "pe32", "pe64":
		return coreclient.FormatPE, nil
	}
	return f, oneOf(f, formats, "format")
}

// reaFormat maps an image format to the format name the evidence bundle uses.
func reaFormat(format string) rea.BinaryFormat {
	switch format {
	case coreclient.FormatPE:
		return rea.FormatPE
	case coreclient.FormatELF:
		return rea.FormatELF
	case coreclient.FormatMachO, coreclient.FormatFatMachO:
		return rea.FormatMachO
	case coreclient.FormatASAR:
		return rea.FormatASAR
	}
	return rea.BinaryFormat(format)
}
