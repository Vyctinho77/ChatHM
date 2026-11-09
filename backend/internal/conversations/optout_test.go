package conversations

import "testing"

func TestIsOptOutOnlyRecognizesClearRequests(t *testing.T) {
	for _, yes := range []string{"PARAR", "parar", " Parar. ", "Pare!", "STOP", "sair", "Descadastrar", "parar avisos", "Não quero receber", "nao quero mais receber", "Não quero mais mensagens"} {
		if !IsOptOut(yes) {
			t.Errorf("%q deveria ser pedido para parar", yes)
		}
	}
	for _, no := range []string{
		"", "oi", "cancelar", "quero cancelar o contrato", "pare de me cobrar, já paguei ontem pelo pix e mandei o comprovante",
		"preciso parar de usar o serviço mês que vem", "Pode parar na minha casa?", "parar", // última linha repete o primeiro caso de propósito abaixo
	} {
		if no == "parar" {
			continue
		}
		if IsOptOut(no) {
			t.Errorf("%q NÃO é um pedido inequívoco para parar", no)
		}
	}
}
