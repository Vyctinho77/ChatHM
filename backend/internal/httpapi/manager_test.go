package httpapi

// Perfil gestor: supervisiona as equipes de que participa, sem mexer na configuração do escritório.

import (
	"testing"
	"time"

	"github.com/vyctinho77/chathm/backend/internal/auth"
)

func (w *world) manager(email string, team string) *client {
	w.e.t.Helper()
	u := w.e.createUser(email, auth.RoleManager, w.teams[team])
	w.users[email[:len(email)-len("@x.com")]] = u.ID
	return w.e.client().mustLogin(email)
}

func TestManagerSupervisesOwnTeamConversations(t *testing.T) {
	w := setupWorld(t)
	gesta := w.manager("gesta@x.com", "folha")
	outro := w.manager("gesto@x.com", "notas")

	if gesta.do("GET", "/api/v1/auth/me", nil).body["user"].(map[string]any)["role"] != "manager" {
		t.Fatal("o /me deveria informar o perfil manager")
	}

	w.receive("Cliente Gestor", "folha de pagamento")
	id := mustConv(t, w.ana, "Cliente Gestor")["id"].(string)
	path := "/api/v1/conversations/" + id
	until := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)

	// Ana assume. Uma colega da equipe (atendente) não mexe no trabalho dela...
	expect(t, w.send(w.ana, id, "Já vejo isso."), 201)
	expect(t, w.transfer(w.ana2, id, w.users["ana2"]), 409, "conversation_taken")
	expect(t, w.ana2.do("POST", path+"/snooze", map[string]string{"until": until}), 403)
	expect(t, w.ana2.do("POST", path+"/close", nil), 403)

	// ...mas a gestora da equipe vê e supervisiona: adia, retoma, transfere e encerra.
	if conv(t, gesta, "Cliente Gestor") == nil {
		t.Fatal("a gestora da equipe Folha deveria ver a conversa")
	}
	expect(t, gesta.do("POST", path+"/snooze", map[string]string{"until": until}), 204)
	expect(t, gesta.do("POST", path+"/resume", nil), 204)
	expect(t, w.transfer(gesta, id, w.users["ana2"]), 204)
	if got := mustConv(t, w.ana2, "Cliente Gestor"); got["assignee"].(map[string]any)["id"] != w.users["ana2"] {
		t.Fatalf("ana2 deveria ter recebido a conversa: %v", got)
	}
	// Assumir para si também vale; já falar com o cliente só depois de assumir (nunca em nome de outra pessoa).
	expect(t, w.send(gesta, id, "Oi, sou a gestora."), 409, "conversation_taken")
	expect(t, w.transfer(gesta, id, w.users["gesta"]), 204)
	expect(t, w.send(gesta, id, "Oi, sou a gestora."), 201)
	expect(t, gesta.do("POST", path+"/close", nil), 204)

	// Visibilidade não muda: gestor de OUTRA equipe não vê nem age (404, sem revelar que existe).
	w.receive("Cliente Gestor B", "folha de pagamento")
	idB := mustConv(t, w.ana, "Cliente Gestor B")["id"].(string)
	if conv(t, outro, "Cliente Gestor B") != nil {
		t.Fatal("gestor da equipe Notas não deveria ver conversa da Folha")
	}
	expect(t, outro.do("POST", "/api/v1/conversations/"+idB+"/close", nil), 404)
	expect(t, w.transfer(outro, idB, w.users["gesto"]), 404)
	expect(t, outro.do("POST", "/api/v1/conversations/"+idB+"/snooze", map[string]string{"until": until}), 404)
}

func TestManagerManagesOnlyItsOwnTeamsMembers(t *testing.T) {
	w := setupWorld(t)
	gesta := w.manager("gesta@x.com", "folha")
	folha, notas := w.teams["folha"], w.teams["notas"]
	member := func(team, user string) string { return "/api/v1/teams/" + team + "/members/" + w.users[user] }

	// Monta a própria equipe: adiciona e remove (e é auditado como ação da gestora).
	expect(t, gesta.do("PUT", member(folha, "dani"), nil), 204)
	expect(t, gesta.do("DELETE", member(folha, "dani"), nil), 204)
	var audits int
	_ = w.e.pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE actor_user_id = $1 AND action LIKE 'team.member%'`, w.users["gesta"]).Scan(&audits)
	if audits != 2 {
		t.Fatalf("auditoria de membros = %d, esperado 2", audits)
	}
	// Equipe de que não participa: responde como inexistente. Atendente não gerencia nada.
	expect(t, gesta.do("PUT", member(notas, "dani"), nil), 404)
	expect(t, gesta.do("DELETE", member(notas, "bia"), nil), 404)
	expect(t, w.ana.do("PUT", member(folha, "dani"), nil), 403)
	expect(t, w.admin.do("PUT", member(notas, "dani"), nil), 204)
}

func TestManagerCannotChangeOfficeConfiguration(t *testing.T) {
	w := setupWorld(t)
	gesta := w.manager("gesta@x.com", "folha")
	folha := w.teams["folha"]

	// Equipes: criar, renomear e apagar continuam do administrador.
	expect(t, gesta.do("POST", "/api/v1/teams", map[string]string{"name": "Nova"}), 403)
	expect(t, gesta.do("PATCH", "/api/v1/teams/"+folha, map[string]string{"name": "Renomeada"}), 403)
	expect(t, gesta.do("DELETE", "/api/v1/teams/"+folha, nil), 403)
	// Pessoas, perfis e convites.
	expect(t, gesta.do("GET", "/api/v1/users", nil), 403)
	expect(t, gesta.do("POST", "/api/v1/users", map[string]any{"email": "novo@x.com", "name": "Novo", "password": "SenhaForte#2024", "role": "agent"}), 403)
	expect(t, gesta.do("PATCH", "/api/v1/users/"+w.users["ana"], map[string]any{"role": "admin"}), 403)
	expect(t, gesta.do("PATCH", "/api/v1/users/"+w.users["gesta"], map[string]any{"role": "admin"}), 403)
	expect(t, gesta.do("POST", "/api/v1/users/invite", map[string]any{"email": "x@x.com", "name": "X", "role": "admin"}), 403)
	// Filas, etiquetas, WhatsApp e integrações de IA.
	expect(t, gesta.do("PUT", "/api/v1/segments/"+w.segs["folha"]+"/team", map[string]any{"teamId": w.teams["notas"]}), 403)
	expect(t, gesta.do("POST", "/api/v1/tags", map[string]string{"name": "x"}), 403)
	expect(t, gesta.do("POST", "/api/v1/whatsapp/connect", nil), 403)
	expect(t, gesta.do("PUT", "/api/v1/ai/settings", map[string]any{"enabled": true, "maskDocuments": false}), 403)
}

func TestAdminAssignsManagerRole(t *testing.T) {
	w := setupWorld(t)
	expect(t, w.admin.do("PATCH", "/api/v1/users/"+w.users["ana"], map[string]any{"role": "chefe"}), 422)
	expect(t, w.admin.do("PATCH", "/api/v1/users/"+w.users["ana"], map[string]any{"role": "manager"}), 200)

	// A promoção vale na hora, sem novo login (papel é lido do banco a cada requisição).
	expect(t, w.ana.do("PUT", "/api/v1/teams/"+w.teams["folha"]+"/members/"+w.users["dani"], nil), 204)
	// O gestor continua sem ser administrador: a última administradora segue protegida e ele não vira admin sozinho.
	expect(t, w.ana.do("PATCH", "/api/v1/users/"+w.users["ana"], map[string]any{"role": "admin"}), 403)
	expect(t, w.admin.do("PATCH", "/api/v1/users/"+w.users["ana"], map[string]any{"role": "agent"}), 200)
	expect(t, w.ana.do("PUT", "/api/v1/teams/"+w.teams["folha"]+"/members/"+w.users["dani"], nil), 403)
}
