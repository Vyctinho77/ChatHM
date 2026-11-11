package conversations

import "testing"

func TestDownloadNameAlwaysHasAnExtension(t *testing.T) {
	id := "0f8fad5b-d9cb-469f-a165-70867728950e"
	cases := []struct{ name, mime, want string }{
		{"", "image/jpeg", "imagem-0f8fad5b.jpg"},
		{"", "image/png", "imagem-0f8fad5b.png"},
		{"", "image/webp", "imagem-0f8fad5b.webp"},
		{"", "audio/ogg; codecs=opus", "audio-0f8fad5b.ogg"},
		{"foto", "image/jpeg", "foto.jpg"},
		{"Folha_Outubro.pdf", "application/pdf", "Folha_Outubro.pdf"},
		{"", "", "arquivo-0f8fad5b"},
	}
	for _, c := range cases {
		if got := downloadName(c.name, c.mime, id); got != c.want {
			t.Errorf("downloadName(%q, %q) = %q, quero %q", c.name, c.mime, got, c.want)
		}
	}
}
