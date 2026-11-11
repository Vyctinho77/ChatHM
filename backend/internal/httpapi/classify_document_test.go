package httpapi

import (
	"testing"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

// Documento sem legenda também é classificado, pelo nome do arquivo.
func TestDocumentWithoutCaptionIsClassifiedByFileName(t *testing.T) {
	w := setupWorld(t)
	doc := func(phone, name, file string) {
		m := inbound("D-"+phone, phone, name, "")
		m.Kind = whatsapp.KindDocument
		m.Payload = map[string]any{"fileName": file, "mimeType": "application/pdf"}
		w.e.wa.Receive(m)
	}
	doc("5511977000001", "Cliente Doc Folha", "Folha_Outubro.pdf")
	doc("5511977000002", "Cliente Doc Aleatorio", "scan0001.pdf")
	c := mustConv(t, w.admin, "Cliente Doc Folha")
	if c["segment"] != "folha" || c["classification"] != "keyword" {
		t.Fatalf("documento 'Folha_Outubro.pdf': segmento=%v classificação=%v", c["segment"], c["classification"])
	}
	if c2 := mustConv(t, w.admin, "Cliente Doc Aleatorio"); c2["segment"] != "geral" {
		t.Fatalf("nome sem assunto deveria ficar na fila geral: %v", c2["segment"])
	}
}
