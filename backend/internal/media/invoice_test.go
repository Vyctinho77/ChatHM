package media

import (
	"context"
	"testing"
)

func TestDetectInvoice(t *testing.T) {
	const ns = `xmlns="http://www.portalfiscal.inf.br/nfe"`
	cases := []struct{ name, xml, want string }{
		{"NF-e com protocolo", `<?xml version="1.0" encoding="UTF-8"?><nfeProc ` + ns + ` versao="4.00"><NFe><infNFe Id="NFe35240512345678000199550010000012341000012345"><ide><cUF>35</cUF><mod>55</mod><serie>1</serie></ide></infNFe></NFe></nfeProc>`, DocNFe},
		{"NFC-e", `<?xml version="1.0"?><nfeProc ` + ns + `><NFe><infNFe><ide><mod>65</mod></ide></infNFe></NFe></nfeProc>`, DocNFCe},
		{"NF-e sem declaração e sem protocolo", `<NFe ` + ns + `><infNFe><ide><mod>55</mod></ide></infNFe></NFe>`, DocNFe},
		{"NF-e em ISO-8859-1", "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><nfeProc><NFe><infNFe><ide><natOp>Venda de produ\xe7\xe3o</natOp><mod>55</mod></ide></infNFe></NFe></nfeProc>", DocNFe},
		{"NF-e com BOM e espaços", "\xef\xbb\xbf  \n<?xml version=\"1.0\"?><nfeProc><NFe><infNFe><ide><mod>55</mod></ide></infNFe></NFe></nfeProc>", DocNFe},
		{"NF-e sem modelo legível", `<nfeProc><NFe><infNFe/></NFe></nfeProc>`, DocNFe},
		{"CT-e", `<?xml version="1.0"?><cteProc xmlns="http://www.portalfiscal.inf.br/cte"><CTe><infCte><ide><mod>57</mod></ide></infCte></CTe></cteProc>`, DocCTe},
		{"MDF-e", `<mdfeProc xmlns="http://www.portalfiscal.inf.br/mdfe"><MDFe/></mdfeProc>`, DocMDFe},
		{"NFS-e nacional", `<?xml version="1.0"?><NFSe xmlns="http://www.sped.fazenda.gov.br/nfse"><infNFSe/></NFSe>`, DocNFSe},
		{"NFS-e ABRASF", `<CompNfse xmlns="http://www.abrasf.org.br/nfse.xsd"><Nfse/></CompNfse>`, DocNFSe},
		{"evento (cancelamento)", `<procEventoNFe xmlns="http://www.portalfiscal.inf.br/nfe"><evento/></procEventoNFe>`, DocEvento},
		{"XML qualquer", `<?xml version="1.0"?><configuration><mod>55</mod></configuration>`, ""},
		{"XML de planilha", `<?xml version="1.0"?><Workbook xmlns="urn:schemas-microsoft-com:office:spreadsheet"/>`, ""},
		{"texto", `olá, segue a nota`, ""},
		{"vazio", ``, ""},
		{"quebrado", `<nfeProc><NFe><infNFe><ide><mod>5`, DocNFe},
	}
	for _, c := range cases {
		if got := DetectInvoice([]byte(c.xml)); got != c.want {
			t.Errorf("%s: %q, esperava %q", c.name, got, c.want)
		}
	}
}

// Nem toda nota tem o nome "certinho": o conteúdo é que manda.
func TestProcessRecognizesInvoiceByContent(t *testing.T) {
	s := NewService(1<<20, nil)
	nfe := []byte(`<?xml version="1.0" encoding="UTF-8"?><nfeProc><NFe><infNFe><ide><mod>55</mod></ide></infNFe></NFe></nfeProc>`)
	for _, name := range []string{"nota.xml", "nota", "arquivo.txt", "Documento (1).xml", "123.dat"} {
		for _, inbound := range []bool{false, true} {
			var p Processed
			var err error
			if inbound {
				p, err = s.ProcessInbound(context.Background(), Document, nfe, name, "")
			} else {
				p, err = s.Process(context.Background(), Document, nfe, name, false)
			}
			if err != nil || p.DocType != DocNFe || p.MimeType != "application/xml" {
				t.Errorf("%s (recebido=%v): %+v %v", name, inbound, p.DocType, err)
			}
		}
	}
	other, err := s.Process(context.Background(), Document, []byte(`<?xml version="1.0"?><a/>`), "dados.xml", false)
	if err != nil || other.DocType != "" || other.MimeType != "application/xml" {
		t.Errorf("xml comum: %+v %v", other, err)
	}
}
