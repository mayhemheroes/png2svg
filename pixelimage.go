package png2svg

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"

	"github.com/xyproto/onthefly"
)

const VersionString = "1.1.0"

type Pixel struct {
	x       int
	y       int
	r       int
	g       int
	b       int
	a       int
	covered bool
}

type Pixels []*Pixel

type PixelImage struct {
	pixels  Pixels
	page    *onthefly.Page
	svgTag  *onthefly.Tag
	verbose bool
	w       int
	h       int
}

func ReadPNG(filename string, verbose bool) (image.Image, error) {
	if verbose {
		fmt.Printf("Reading %s...", filename)
	}
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	if verbose {
		fmt.Println("ok")
	}
	return img, nil
}

func NewPixelImage(img image.Image, verbose bool) *PixelImage {
	width := img.Bounds().Max.X - img.Bounds().Min.X
	height := img.Bounds().Max.Y - img.Bounds().Min.Y

	pixels := make(Pixels, width*height, width*height)

	var c color.NRGBA
	if verbose {
		fmt.Print("Converting image.Image to Pixels")
	}
	i := 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		if verbose {
			fmt.Print(".")
		}
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			c = color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			alpha := int(c.A)
			// Mark transparent pixels as already being "covered"
			covered := alpha == 0
			pixels[i] = &Pixel{x, y, int(c.R), int(c.G), int(c.B), alpha, covered}
			i++
		}
	}

	// Create a new XML page with a new SVG tag
	page, svgTag := onthefly.NewTinySVGPixels(width, height)

	if verbose {
		fmt.Println("ok")
	}

	return &PixelImage{pixels, page, svgTag, verbose, width, height}
}

// Done checks if all pixels are covered, in terms of being represented by an SVG element
func (pi *PixelImage) Done() bool {
	for _, p := range pi.pixels {
		if !(*p).covered {
			return false
		}
	}
	return true
}

// At returns the RGB color at the given coordinate
func (pi *PixelImage) At(x, y int) (r, g, b int) {
	i := y*pi.w + x
	//if i >= len(pi.pixels) {
	//	panic("At out of bounds, too large coordinate")
	//}
	p := *pi.pixels[i]
	return p.r, p.g, p.b
}

// Covered returns true if the pixel at the given coordinate is already covered by SVG elements
func (pi *PixelImage) Covered(x, y int) bool {
	i := y*pi.w + x
	p := *pi.pixels[i]
	return p.covered
}

// Cover all pixels that are not yet covered, by creating an svg rectangle per pixel
func (pi *PixelImage) CoverAllPixels() {
	coverCount := 0
	for _, p := range pi.pixels {
		if !(*p).covered {
			pi.svgTag.Pixel((*p).x, (*p).y, (*p).r, (*p).g, (*p).b)
			(*p).covered = true
			coverCount++
		}
	}
	if pi.verbose {
		fmt.Printf("Covered %d pixels with 1x1 rectangles.\n", coverCount)
	}
}

func (pi *PixelImage) FirstUncovered() (int, int) {
	for y := 0; y < pi.h; y++ {
		for x := 0; x < pi.w; x++ {
			i := y*pi.w + x
			if !pi.pixels[i].covered {
				return x, y
			}
		}
	}
	// This should never happen, except when debugging
	panic("All pixels are covered")
}

// Extract the fill color from a svg rect line (<rect ... fill="#ff0000" ...) would return #ff0000
// Returns an empty string if no fill color is found
func colorFromLine(line string) string {
	if !strings.Contains(line, " fill=\"") {
		return ""
	}
	fields := strings.Fields(line)
	for _, field := range fields {
		if strings.HasPrefix(field, "fill=") {
			elems := strings.Split(field, "\"")
			// if len(elems) < 3 { panic("invalid SVG: " + line) }
			return elems[1]
		}
	}
	// Only used during debugging
	// panic("No fill color on this line: " + line)
	return ""
}

// group lines that has a fill color by color, and organize under <g> tags
func groupLinesByFillColor(lines []string) []string {
	// Group lines by fill color
	groupedLines := make(map[string][]string)
	var fillColor string
	for i, line := range lines {
		fillColor = colorFromLine(line)
		if fillColor == "" {
			continue
		}
		// Erase the line. The grouped lines will be inserted at the first empty line.
		lines[i] = ""
		if groupedLines[fillColor] == nil {
			groupedLines[fillColor] = make([]string, 0)
		}
		groupedLines[fillColor] = append(groupedLines[fillColor], line)
	}

	// Build a string of all lines with fillcolor, grouped by fillcolor, inside <g> tags
	var sb strings.Builder
	for key, lines := range groupedLines {
		sb.WriteString("<g fill=\"" + key + "\">")
		for _, line := range lines {
			sb.WriteString(strings.Replace(line, " fill=\""+key+"\"", "", 1))
		}
		sb.WriteString("</g>")
	}
	contents := sb.String()

	// Insert the contents in the slice of lines
	inserted := false
	for i, line := range lines {
		if !inserted && line == "" {
			lines[i] = contents
			inserted = true
		}
	}

	// Return lines, some of them empty. One of them is a really long line with the above contents.
	return lines
}

// String returns the rendered SVG document as a string
func (pi *PixelImage) String() string {
	svgDocument := pi.page.String()

	// Group lines by fill color, insert <g> tags
	lines := groupLinesByFillColor(strings.Split(svgDocument, "\n"))

	// Use the new line contents as the new svgDocument
	svgDocument = strings.Join(lines, "\n")

	// Only non-destructive and spec-conforming optimizations goes here

	// NOTE: Removing width and height for "1" gave incorrect results in GIMP.
	// NOTE: Gimp complains about the width and height not being set, but it is set.

	// Remove all newlines
	svgDocument = strings.Replace(svgDocument, "\n", "", -1)
	// Remove all spaces before closing tags
	svgDocument = strings.Replace(svgDocument, " />", "/>", -1)
	// Remove double spaces
	svgDocument = strings.Replace(svgDocument, "  ", " ", -1)
	// Remove empty x attributes
	svgDocument = strings.Replace(svgDocument, " x=\"0\"", "", -1)
	// Remove empty y attributes
	svgDocument = strings.Replace(svgDocument, " y=\"0\"", "", -1)
	// Remove empty width attributes
	svgDocument = strings.Replace(svgDocument, " width=\"0\"", "", -1)
	// Remove empty height attributes
	svgDocument = strings.Replace(svgDocument, " height=\"0\"", "", -1)
	// Remove single spaces between tags
	svgDocument = strings.Replace(svgDocument, "> <", "><", -1)
	// "red" is shorter than #f00 or #ff0000
	svgDocument = strings.Replace(svgDocument, "#f00", "red", -1)
	svgDocument = strings.Replace(svgDocument, "#ff0000", "red", -1)
	// "white" is shorter than #ffffff
	svgDocument = strings.Replace(svgDocument, "#ffffff", "white", -1)
	// "black" is shorter than #000000
	svgDocument = strings.Replace(svgDocument, "#000000", "black", -1)

	return svgDocument
}

func (pi *PixelImage) WriteSVG(filename string) error {
	if !pi.Done() {
		return errors.New("the SVG representation does not cover all pixels")
	}
	if pi.verbose {
		fmt.Printf("Writing %s...", filename)
	}
	var (
		err error
		f   *os.File
	)
	if filename == "-" {
		f = os.Stdout
	} else {
		f, err = os.Create(filename)
		if err != nil {
			return err
		}
		defer f.Close()
	}

	if _, err = f.WriteString(pi.String()); err != nil {
		return err
	}
	if pi.verbose {
		fmt.Println("ok")
	}
	return nil
}
