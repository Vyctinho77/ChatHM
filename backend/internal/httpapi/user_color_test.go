package httpapi

import "testing"

// Cor do nome de cada pessoa: só o administrador define; todos veem em /people.
func TestUserColorIsSetByAdminAndSeenByEveryone(t *testing.T) {
	w := setupWorld(t)
	var anaID string
	for _, u := range w.admin.do("GET", "/api/v1/users", nil).body["users"].([]any) {
		if m := u.(map[string]any); m["email"] != nil && m["role"] == "agent" && anaID == "" {
			anaID = m["id"].(string)
			if m["color"] != "" {
				t.Fatalf("a cor começa automática (vazia): %v", m["color"])
			}
		}
	}
	if anaID == "" {
		t.Fatal("sem atendente no ambiente de teste")
	}
	colorOf := func(c *client) any {
		for _, p := range c.do("GET", "/api/v1/people", nil).body["people"].([]any) {
			if m := p.(map[string]any); m["id"] == anaID {
				return m["color"]
			}
		}
		return "?"
	}

	expect(t, w.admin.do("PATCH", "/api/v1/users/"+anaID, map[string]any{"color": "azul"}), 200)
	if got := colorOf(w.admin); got != "azul" {
		t.Fatalf("admin vê %v", got)
	}
	if got := colorOf(w.bia); got != "azul" {
		t.Fatalf("outra pessoa da equipe vê %v", got)
	}
	// Chave fora da paleta, código de cor e texto qualquer são recusados.
	for _, bad := range []string{"fucsia", "#ff0000", "AZUL", "azul escuro"} {
		expect(t, w.admin.do("PATCH", "/api/v1/users/"+anaID, map[string]any{"color": bad}), 422)
	}
	if got := colorOf(w.admin); got != "azul" {
		t.Fatalf("a recusa não pode mudar a cor: %v", got)
	}
	// Quem não é administrador não define a cor de ninguém (nem a própria).
	expect(t, w.bia.do("PATCH", "/api/v1/users/"+anaID, map[string]any{"color": "rosa"}), 403)
	expect(t, w.ana.do("PATCH", "/api/v1/users/"+anaID, map[string]any{"color": "rosa"}), 403)
	// Mudar só o nome não apaga a cor; vazio volta para a automática.
	expect(t, w.admin.do("PATCH", "/api/v1/users/"+anaID, map[string]any{"name": "Ana Paula"}), 200)
	if got := colorOf(w.admin); got != "azul" {
		t.Fatalf("a cor sumiu ao mudar o nome: %v", got)
	}
	expect(t, w.admin.do("PATCH", "/api/v1/users/"+anaID, map[string]any{"color": ""}), 200)
	if got := colorOf(w.admin); got != "" {
		t.Fatalf("vazio deveria voltar para a automática: %v", got)
	}
}
