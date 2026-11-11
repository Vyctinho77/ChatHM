package media

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/rand"
	"strings"
	"testing"
)

func noisyJPEG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	r := rand.New(rand.NewSource(1))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x/4 + r.Intn(40)), uint8(y/4 + r.Intn(40)), uint8(r.Intn(255)), 255})
		}
	}
	var b bytes.Buffer
	_ = jpeg.Encode(&b, img, &jpeg.Options{Quality: 85})
	return b.Bytes()
}

func TestPreviewShrinksBigImagesAndKeepsSmallOnes(t *testing.T) {
	big := noisyJPEG(1600, 1200)
	out, ok := Preview(big)
	if !ok || len(out) >= len(big)/2 {
		t.Fatalf("preview deveria ser bem menor: ok=%v %d -> %d", ok, len(big), len(out))
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil || max(cfg.Width, cfg.Height) != previewMaxDim || cfg.Width*3 != cfg.Height*4 {
		t.Fatalf("dimensões do preview: %v %v", cfg, err)
	}
	if _, ok := Preview(noisyJPEG(200, 150)); ok {
		t.Fatal("imagem pequena deve ser servida como está")
	}
	if _, ok := Preview([]byte("não é imagem")); ok {
		t.Fatal("lixo não gera preview")
	}
}

func TestThumbIsTinyDataURI(t *testing.T) {
	th, ok := Thumb(noisyJPEG(1200, 900))
	if !ok || !strings.HasPrefix(th, "data:image/jpeg;base64,") || len(th) > 2500 {
		t.Fatalf("miniatura: ok=%v len=%d", ok, len(th))
	}
	// PNG com transparência também.
	img := image.NewNRGBA(image.Rect(0, 0, 300, 200))
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	if _, ok := Thumb(b.Bytes()); !ok {
		t.Fatal("PNG deveria gerar miniatura")
	}
}

func TestOrientedJPEGsAreLeftAlone(t *testing.T) {
	// JPEG mínimo com EXIF Orientation = 6 (inserido logo após o SOI).
	exif := []byte{0xFF, 0xE1, 0x00, 0x22, 'E', 'x', 'i', 'f', 0, 0, 'M', 'M', 0, 0x2A, 0, 0, 0, 8, 0, 1, 0x01, 0x12, 0, 3, 0, 0, 0, 1, 0, 6, 0, 0, 0, 0, 0, 0}
	base := noisyJPEG(1600, 1200)
	oriented := append(append([]byte{0xFF, 0xD8}, exif...), base[2:]...)
	if got := jpegOrientation(oriented); got != 6 {
		t.Fatalf("orientação = %d, quero 6", got)
	}
	if _, ok := Preview(oriented); ok {
		t.Fatal("com rotação em EXIF, o original deve ser servido")
	}
	if got := jpegOrientation(base); got != 0 {
		t.Fatalf("sem EXIF: %d", got)
	}
}
