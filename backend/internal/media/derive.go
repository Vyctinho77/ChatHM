package media

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// Derivados de imagem, como o WhatsApp faz: uma miniatura minúscula que vai junto com a mensagem (aparece desfocada na hora)
// e uma versão média para o balão, de modo que o arquivo original só é baixado quando a pessoa abre a foto.
const (
	previewMaxDim    = 640      // 2x a largura do balão (288 px), nítido em tela de alta densidade
	previewSkipBytes = 60 << 10 // imagens já pequenas são servidas como estão
	thumbMaxDim      = 28
)

// Preview devolve uma versão reduzida (JPEG) da imagem. ok=false significa "sirva o original": já é pequena, tem rotação
// em EXIF (o redimensionamento a perderia) ou não deu para decodificar.
func Preview(data []byte) (out []byte, ok bool) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (max(cfg.Width, cfg.Height) <= previewMaxDim && len(data) <= previewSkipBytes) {
		return nil, false
	}
	if jpegOrientation(data) > 1 {
		return nil, false
	}
	small, ok := scaled(data, previewMaxDim)
	if !ok {
		return nil, false
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, small, &jpeg.Options{Quality: 72}); err != nil || buf.Len() >= len(data) {
		return nil, false
	}
	return buf.Bytes(), true
}

// Thumb devolve a miniatura como data URI (alguns centenas de bytes), para embutir na mensagem.
func Thumb(data []byte) (string, bool) {
	if jpegOrientation(data) > 1 {
		return "", false
	}
	small, ok := scaled(data, thumbMaxDim)
	if !ok {
		return "", false
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, small, &jpeg.Options{Quality: 45}); err != nil {
		return "", false
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), true
}

// scaled decodifica e reduz para no máximo maxDim na maior dimensão, sobre fundo branco (PNG com transparência).
func scaled(data []byte, maxDim int) (*image.RGBA, bool) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return nil, false
	}
	if m := max(w, h); m > maxDim {
		w, h = max(1, w*maxDim/m), max(1, h*maxDim/m)
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, xdraw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
	return dst, true
}

// jpegOrientation lê a rotação (EXIF, 1 a 8) de um JPEG; 0 quando não há.
func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0
	}
	i := 2
	for i+4 <= len(data) && data[i] == 0xFF {
		marker := data[i+1]
		if marker == 0xDA || marker == 0xD9 { // início da imagem: acabaram os metadados
			return 0
		}
		size := int(data[i+2])<<8 | int(data[i+3])
		if size < 2 || i+2+size > len(data) {
			return 0
		}
		seg := data[i+4 : i+2+size]
		if marker == 0xE1 && len(seg) > 14 && strings.HasPrefix(string(seg[:6]), "Exif\x00\x00") {
			tiff := seg[6:]
			var le bool
			switch string(tiff[:2]) {
			case "II":
				le = true
			case "MM":
			default:
				return 0
			}
			u16 := func(b []byte) int {
				if le {
					return int(b[0]) | int(b[1])<<8
				}
				return int(b[0])<<8 | int(b[1])
			}
			u32 := func(b []byte) int {
				if le {
					return int(b[0]) | int(b[1])<<8 | int(b[2])<<16 | int(b[3])<<24
				}
				return int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
			}
			if len(tiff) < 8 {
				return 0
			}
			off := u32(tiff[4:8])
			if off < 8 || off+2 > len(tiff) {
				return 0
			}
			n := u16(tiff[off : off+2])
			for e := 0; e < n; e++ {
				p := off + 2 + e*12
				if p+12 > len(tiff) {
					return 0
				}
				if u16(tiff[p:p+2]) == 0x0112 { // Orientation
					return u16(tiff[p+8 : p+10])
				}
			}
			return 0
		}
		i += 2 + size
	}
	return 0
}
