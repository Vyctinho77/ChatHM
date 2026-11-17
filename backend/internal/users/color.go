package users

// ColorKeys: a paleta de cores do nome das pessoas (a mesma do app: apps/web/src/features/team/personColor.ts). O app escolhe o tom de
// cada uma para o tema claro e para o escuro; aqui só se guarda a chave.
var ColorKeys = []string{"verde", "azul", "roxo", "rosa", "laranja", "ambar", "ciano", "vermelho", "lima", "indigo", "marrom", "grafite"}

// ValidColor: uma chave da paleta, ou vazio (cor automática).
func ValidColor(k string) bool {
	if k == "" {
		return true
	}
	for _, c := range ColorKeys {
		if c == k {
			return true
		}
	}
	return false
}
