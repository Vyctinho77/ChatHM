package aiaccess

import "testing"

func TestEveryCapabilityIsKnownAndGateFailsClosed(t *testing.T) {
	valid := map[capability]bool{capRead: true, capWrite: true, capSend: true, capAttend: true, capAutomate: true, capAutomateOrAttend: true}
	for name, c := range toolCapabilities {
		if !valid[c] {
			t.Errorf("%s tem uma capacidade inválida: %q", name, c)
		}
	}
	// Sem entrada no mapa não há capacidade: o gate trata como desconhecida (ver gate) e nunca libera por esquecimento.
	if _, known := toolCapabilities["ferramenta_nova_sem_regra"]; known {
		t.Fatal("o mapa não deveria conhecer uma ferramenta inexistente")
	}
	read := Principal{}
	if !read.can(capRead) || read.can(capWrite) || read.can(capSend) || read.can(capAttend) || read.can(capAutomate) || read.can(capAutomateOrAttend) {
		t.Fatal("um principal sem permissões só pode ler")
	}
	if !(Principal{Attend: true}).can(capAutomateOrAttend) || !(Principal{Automate: true}).can(capAutomateOrAttend) {
		t.Fatal("automate ou attend bastam para a biblioteca de PDFs")
	}
	if (Principal{Write: true, Send: true}).can(capAttend) || (Principal{Write: true, Send: true}).can(capAutomate) {
		t.Fatal("escrever e pedir envio não dá atendente nem envio automático")
	}
}
