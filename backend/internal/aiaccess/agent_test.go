package aiaccess

import "testing"

func TestAgentKeySeparatesAgentsOfTheSamePerson(t *testing.T) {
	chefao := Principal{UserID: "u1", Provider: "claude", Agent: "Chefão"}
	fin := Principal{UserID: "u1", Provider: "claude", Agent: "HM Financeiro"}
	if agentKey(chefao) == agentKey(fin) {
		t.Fatal("dois agentes da mesma pessoa e do mesmo provedor precisam de filas separadas")
	}
	// Renovar o token com o mesmo nome (com ou sem acento, caixa ou espaços) mantém a mesma identidade.
	if agentKey(Principal{UserID: "u1", Provider: "claude", Agent: "  hm  FINANCEIRO "}) != agentKey(fin) {
		t.Fatal("o mesmo nome deve dar a mesma chave")
	}
	if agentKey(Principal{UserID: "u1", Provider: "claude"}) != "u1:claude" {
		t.Fatal("sem nome vale a chave antiga (provedor + pessoa)")
	}
	if got := agentSlug("Chefão"); got != "chefao" {
		t.Fatalf("%q", got)
	}
	if got := agentSlug("E-CAC  (Receita)"); got != "e-cac-receita" {
		t.Fatalf("%q", got)
	}
}

func TestAuthorIsYouDistinguishesAgents(t *testing.T) {
	msg := map[string]any{"automated": true, "aiProvider": "claude", "aiUserId": "u1", "aiAgent": "HM Financeiro"}
	me := Principal{UserID: "u1", Provider: "claude", Agent: "HM Financeiro"}
	other := Principal{UserID: "u1", Provider: "claude", Agent: "Chefão"}
	if a := authorOf(msg, me, "c1"); a["isYou"] != true || a["agent"] != "HM Financeiro" {
		t.Fatalf("%v", a)
	}
	if a := authorOf(msg, other, "c1"); a["isYou"] != false {
		t.Fatalf("outro agente da mesma pessoa não é você: %v", a)
	}
	old := map[string]any{"automated": true, "aiProvider": "claude", "aiUserId": "u1"} // antes desta versão: sem nome do agente
	if a := authorOf(old, other, "c1"); a["isYou"] != true {
		t.Fatalf("mensagem antiga mantém a regra anterior: %v", a)
	}
}
