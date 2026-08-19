package mediascan

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"golang.org/x/image/bmp"
)

// The fixtures below are real encoded images rather than canned bytes, built at
// a deliberately non-square size so a transposed width and height cannot pass.
const (
	fixtureWidth  = 300
	fixtureHeight = 450
)

// encodedImage builds an image of the fixture size and encodes it with the given
// encoder, returning the bytes as a string so buildTree can write them.
func encodedImage(t *testing.T, encode func(*bytes.Buffer, image.Image) error) string {
	t.Helper()

	source := image.NewRGBA(image.Rect(0, 0, fixtureWidth, fixtureHeight))
	// A single flat colour compresses to almost nothing, which keeps the
	// fixtures small without affecting what the header reports.
	for y := range fixtureHeight {
		for x := range fixtureWidth {
			source.Set(x, y, color.RGBA{R: 0x20, G: 0x40, B: 0x80, A: 0xff})
		}
	}

	var encoded bytes.Buffer
	if err := encode(&encoded, source); err != nil {
		t.Fatalf("encoding the fixture image: %v", err)
	}
	return encoded.String()
}

func pngFixture(t *testing.T) string {
	t.Helper()
	return encodedImage(t, func(w *bytes.Buffer, m image.Image) error { return png.Encode(w, m) })
}

func jpegFixture(t *testing.T) string {
	t.Helper()
	return encodedImage(t, func(w *bytes.Buffer, m image.Image) error { return jpeg.Encode(w, m, nil) })
}

func gifFixture(t *testing.T) string {
	t.Helper()
	return encodedImage(t, func(w *bytes.Buffer, m image.Image) error { return gif.Encode(w, m, nil) })
}

func bmpFixture(t *testing.T) string {
	t.Helper()
	return encodedImage(t, func(w *bytes.Buffer, m image.Image) error { return bmp.Encode(w, m) })
}

// assertFixtureDimensions checks a sidecar's image block against the codec it
// was encoded with and the size every fixture is built at.
func assertFixtureDimensions(t *testing.T, sidecar SidecarFile, wantCodec string) {
	t.Helper()

	if sidecar.Image == nil {
		t.Fatalf("%s: Image = nil, want an image record", sidecar.FileName)
	}
	if sidecar.Image.Error != "" {
		t.Fatalf("%s: Image.Error = %q, want the header to read cleanly", sidecar.FileName, sidecar.Image.Error)
	}
	if sidecar.Image.Codec != wantCodec {
		t.Errorf("%s: Image.Codec = %q, want %q", sidecar.FileName, sidecar.Image.Codec, wantCodec)
	}
	if sidecar.Image.Width != fixtureWidth || sidecar.Image.Height != fixtureHeight {
		t.Errorf("%s: Image dimensions = %dx%d, want %dx%d",
			sidecar.FileName, sidecar.Image.Width, sidecar.Image.Height, fixtureWidth, fixtureHeight)
	}
}

// TestImageInfoRecordedPerCodec covers every format that has an encoder to build
// a fixture with. bmp is the one that exercises the golang.org/x/image
// decoders, which webp and tiff reach through the same registration.
func TestImageInfoRecordedPerCodec(t *testing.T) {
	result := scanTree(t, "The Movie (2019)", TypeMovie, map[string]string{
		"The Movie (2019).mkv": "video",
		"poster.jpg":           jpegFixture(t),
		"fanart.png":           pngFixture(t),
		"banner.gif":           gifFixture(t),
		"landscape.bmp":        bmpFixture(t),
	})

	assertFixtureDimensions(t, directorySidecarByName(t, result, "poster.jpg"), "jpeg")
	assertFixtureDimensions(t, directorySidecarByName(t, result, "fanart.png"), "png")
	assertFixtureDimensions(t, directorySidecarByName(t, result, "banner.gif"), "gif")
	assertFixtureDimensions(t, directorySidecarByName(t, result, "landscape.bmp"), "bmp")
}

// webpFixture is a 4x6 lossless WebP, produced with `cwebp -lossless` and
// embedded because golang.org/x/image/webp is decoder-only — there is no
// encoder to build one with at test time, and webp artwork is common enough
// that the format deserves a real fixture rather than an assumption that it
// works because bmp does.
const webpFixture = "UklGRnYAAABXRUJQVlA4TGkAAAAvA0ABAAWbAADSNKEHSfzndJi7u4ZXGslW0383/5PBJYX7qAxNgMIwjiQrFa4JcOT8IsDJP5vvLv2PFlvJq5SFu35hWnmMVRCm/B7uBGkGU3zEnSarKq/Os4Ec6Y7HBu9/x/mfA3o7+AgA"

// TestImageInfoRecordsWebP covers the format the project's default sidecar
// table lists but the standard library cannot read, which is the reason
// golang.org/x/image is a dependency at all.
func TestImageInfoRecordsWebP(t *testing.T) {
	decoded, err := base64.StdEncoding.DecodeString(webpFixture)
	if err != nil {
		t.Fatalf("decoding the webp fixture: %v", err)
	}

	result := scanTree(t, "The Movie (2019)", TypeMovie, map[string]string{
		"The Movie (2019).mkv": "video",
		"poster.webp":          string(decoded),
	})

	sidecar := directorySidecarByName(t, result, "poster.webp")
	if sidecar.Image == nil {
		t.Fatal("Image = nil, want an image record")
	}
	if sidecar.Image.Error != "" {
		t.Fatalf("Image.Error = %q, want the header to read cleanly", sidecar.Image.Error)
	}
	if sidecar.Image.Codec != "webp" {
		t.Errorf("Image.Codec = %q, want %q", sidecar.Image.Codec, "webp")
	}
	if sidecar.Image.Width != 4 || sidecar.Image.Height != 6 {
		t.Errorf("Image dimensions = %dx%d, want 4x6", sidecar.Image.Width, sidecar.Image.Height)
	}
}

// TestImageInfoReadsContentsNotExtension pins the sniffing behaviour. A .tbn
// holds whatever the encoder produced, and artwork saved under the wrong
// extension is common enough that the bytes have to win.
func TestImageInfoReadsContentsNotExtension(t *testing.T) {
	result := scanTree(t, "The Movie (2019)", TypeMovie, map[string]string{
		"The Movie (2019).mkv": "video",
		"poster.tbn":           jpegFixture(t),
		// Named .png, encoded as JPEG.
		"fanart.png": jpegFixture(t),
	})

	assertFixtureDimensions(t, directorySidecarByName(t, result, "poster.tbn"), "jpeg")
	assertFixtureDimensions(t, directorySidecarByName(t, result, "fanart.png"), "jpeg")
}

// TestImageInfoRecordsUnreadableImage covers the corrupt-artwork path: the
// record says why it could not be read, and the scan says so too.
func TestImageInfoRecordsUnreadableImage(t *testing.T) {
	result := scanTree(t, "The Movie (2019)", TypeMovie, map[string]string{
		"The Movie (2019).mkv": "video",
		"poster.jpg":           "not an image",
	})

	sidecar := directorySidecarByName(t, result, "poster.jpg")
	if sidecar.Image == nil {
		t.Fatal("Image = nil for an unreadable image, want a record carrying the error")
	}
	if sidecar.Image.Error == "" {
		t.Error("Image.Error is empty, want the decode failure")
	}
	if sidecar.Image.Codec != "" || sidecar.Image.Width != 0 || sidecar.Image.Height != 0 {
		t.Errorf("Image = %+v, want only Error set", *sidecar.Image)
	}

	if !warningsMention(result.Directory.Warnings, "poster.jpg") {
		t.Errorf("no warning naming poster.jpg; got %v", result.Directory.Warnings)
	}
}

// TestImageInfoOnlyForImages confirms nothing but artwork reaches the decoder.
func TestImageInfoOnlyForImages(t *testing.T) {
	result := scanTree(t, "The Movie (2019)", TypeMovie, map[string]string{
		"The Movie (2019).mkv": "video",
		"The Movie (2019).srt": "subtitle text",
		"movie.nfo":            "<movie><title>The Movie</title></movie>",
		"poster.jpg":           jpegFixture(t),
	})

	if sidecar := directorySidecarByName(t, result, "movie.nfo"); sidecar.Image != nil {
		t.Errorf("movie.nfo Image = %+v, want nil", *sidecar.Image)
	}

	mediaFile := mediaFileByName(t, result, "The Movie (2019).mkv")
	if sidecar := mediaSidecarByName(t, mediaFile, "The Movie (2019).srt"); sidecar.Image != nil {
		t.Errorf("subtitle Image = %+v, want nil", *sidecar.Image)
	}

	if sidecar := directorySidecarByName(t, result, "poster.jpg"); sidecar.Image == nil {
		t.Error("poster.jpg Image = nil, want an image record")
	}
}

// TestImageInfoOnSeasonAndMediaSidecars covers the other two owners a sidecar
// can have, each built by a different branch of classifySidecars.
func TestImageInfoOnSeasonAndMediaSidecars(t *testing.T) {
	result := scanTree(t, "The Show", TypeTV, map[string]string{
		"Season 01/The Show S01E01.mkv":       "episode",
		"Season 01/The Show S01E01-thumb.jpg": jpegFixture(t),
		"Season 01/poster.png":                pngFixture(t),
	})

	mediaFile := mediaFileByName(t, result, "The Show S01E01.mkv")
	assertFixtureDimensions(t, mediaSidecarByName(t, mediaFile, "The Show S01E01-thumb.jpg"), "jpeg")
	assertFixtureDimensions(t, seasonSidecarByName(t, result, 1, "poster.png"), "png")
}

func warningsMention(warnings []string, fileName string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, fileName) {
			return true
		}
	}
	return false
}
