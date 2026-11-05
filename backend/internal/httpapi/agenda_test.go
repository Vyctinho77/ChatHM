package httpapi

import (
	"github.com/google/uuid"
	"github.com/vyctinho77/chathm/backend/internal/auth"
	"testing"
)

func TestAgendaPersistenceAndTeamAuthorization(t *testing.T) {
	e := setup(t)
	e.createUser("adm@agenda.test", auth.RoleAdmin)
	agentUser := e.createUser("agent@agenda.test", auth.RoleAgent)
	e.createUser("outsider@agenda.test", auth.RoleAgent)
	admin := e.client().mustLogin("adm@agenda.test")
	agent := e.client().mustLogin("agent@agenda.test")
	outsider := e.client().mustLogin("outsider@agenda.test")
	teamResp := admin.do("POST", "/api/v1/teams", map[string]string{"name": "Agenda A"})
	expect(t, teamResp, 201)
	team := teamResp.body["id"].(string)
	expect(t, admin.do("PUT", "/api/v1/teams/"+team+"/members/"+agentUser.ID, nil), 204)
	other := admin.do("POST", "/api/v1/teams", map[string]string{"name": "Agenda B"}).body["id"].(string)
	id := uuid.NewString()
	path := "/api/v1/agenda/" + id
	body := map[string]any{"kind": "appointment", "title": "Reunião", "date": "2026-11-12", "time": "09:30", "teamId": team, "note": "Documentos", "done": false}
	expect(t, e.client().do("PUT", path, body), 401)
	expect(t, agent.do("PUT", path, body), 200)
	list := agent.do("GET", "/api/v1/agenda", nil)
	expect(t, list, 200)
	items := list.body["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["time"] != "09:30" {
		t.Fatalf("agenda not persisted: %v", items)
	}
	hidden := outsider.do("GET", "/api/v1/agenda", nil)
	expect(t, hidden, 200)
	if len(hidden.body["items"].([]any)) != 0 {
		t.Fatal("outsider saw agenda")
	}
	expect(t, outsider.do("PUT", path, body), 404)
	expect(t, outsider.do("PATCH", path+"/done", map[string]bool{"done": true}), 404)
	expect(t, outsider.do("DELETE", path, nil), 404)
	body["teamId"] = other
	expect(t, agent.do("PUT", path, body), 404)
	body["teamId"] = team
	body["date"] = "2026-02-30"
	expect(t, agent.do("PUT", path, body), 422, "validation_failed")
	body["date"] = "2026-11-13"
	body["title"] = "Reunião atualizada"
	expect(t, agent.do("PUT", path, body), 200)
	expect(t, agent.do("PATCH", path+"/done", map[string]bool{"done": true}), 204)
	listed := admin.do("GET", "/api/v1/agenda", nil).body["items"].([]any)
	if !listed[0].(map[string]any)["done"].(bool) {
		t.Fatal("done not saved")
	}
	noCSRF := e.client().mustLogin("agent@agenda.test")
	noCSRF.csrf = ""
	expect(t, noCSRF.do("DELETE", path, nil), 403, "csrf_failed")
	expect(t, admin.do("DELETE", "/api/v1/teams/"+team, nil), 409, "team_in_use")
	expect(t, agent.do("DELETE", path, nil), 204)
	if len(agent.do("GET", "/api/v1/agenda", nil).body["items"].([]any)) != 0 {
		t.Fatal("item not deleted")
	}
}
