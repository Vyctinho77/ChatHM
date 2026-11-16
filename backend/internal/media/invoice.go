package media

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
)

// Tipos de documento fiscal reconhecidos pelo conteúdo do XML (Processed.DocType).
const (
	DocNFe    = "nfe"    // nota fiscal eletrônica (modelo 55)
	DocNFCe   = "nfce"   // nota fiscal de consumidor (modelo 65)
	DocCTe    = "cte"    // conhecimento de transporte
	DocMDFe   = "mdfe"   // manifesto de documentos fiscais
	DocNFSe   = "nfse"   // nota fiscal de serviços (padrão nacional e os municipais mais comuns)
	DocEvento = "evento" // evento fiscal (cancelamento, carta de correção, manifestação)
)

const (
	invoiceScanBytes  = 256 << 10 // o que importa está no começo do arquivo
	invoiceScanTokens = 400
)

// DetectInvoice diz que documento fiscal é este XML, pela estrutura (elemento raiz e, para NF-e/NFC-e, o modelo), sem depender do
// nome do arquivo. Devolve "" se não for um XML fiscal conhecido.
func DetectInvoice(data []byte) string {
	if len(data) > invoiceScanBytes {
		data = data[:invoiceScanBytes]
	}
	data = bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(data), []byte("\xef\xbb\xbf")))
	if len(data) == 0 || data[0] != '<' {
		return ""
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	// NF-e antigas vêm em ISO-8859-1: os nomes dos elementos são ASCII, então basta deixar o conteúdo passar como está.
	dec.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }

	root := ""
	for i := 0; i < invoiceScanTokens; i++ {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		name := strings.ToLower(start.Name.Local)
		if root == "" {
			root = name
			switch {
			case strings.HasPrefix(root, "proceventonfe"), strings.HasPrefix(root, "proceventocte"), strings.HasPrefix(root, "proceventomdfe"),
				root == "eventonfe", root == "evento" && strings.Contains(start.Name.Space, "portalfiscal"):
				return DocEvento
			case strings.Contains(root, "mdfe"):
				return DocMDFe
			case strings.Contains(root, "cte"):
				return DocCTe
			case strings.Contains(root, "nfse"), root == "compnfse", root == "listanfse", root == "consultarnfseresposta":
				return DocNFSe
			case root == "nfeproc", root == "nfe", root == "envinfe":
				// NF-e ou NFC-e: quem diz é o modelo, mais adiante.
			default:
				return ""
			}
			continue
		}
		if name == "mod" {
			var model string
			if err := dec.DecodeElement(&model, &start); err == nil {
				switch strings.TrimSpace(model) {
				case "65":
					return DocNFCe
				case "55":
					return DocNFe
				}
			}
		}
	}
	if root == "nfeproc" || root == "nfe" || root == "envinfe" {
		return DocNFe // sem o modelo legível, ainda é uma NF-e
	}
	return ""
}
