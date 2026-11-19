package httpapi

import (
	"sort"
	"testing"
	"time"
)

func (w *wsClient) presenceEvents() map[string][]bool { // pessoa -> sequência de online/offline recebida
	w.mu.Lock()
	defer w.mu.Unlock()
	out := map[string][]bool{}
	for _, e := range w.events {
		if e["type"] != "presence" {
			continue
		}
		d, _ := e["data"].(map[string]any)
		id, _ := d["userId"].(string)
		on, _ := d["online"].(bool)
		out[id] = append(out[id], on)
	}
	return out
}

func onlineIDs(t *testing.T, c *client) []string {
	t.Helper()
	r := c.do("GET", "/api/v1/presence", nil)
	expect(t, r, 200)
	out := []string{}
	for _, x := range r.body["online"].([]any) {
		out = append(out, x.(string))
	}
	sort.Strings(out)
	return out
}

// Quem está online: tem uma aba do app aberta (WebSocket). Entrar e sair é avisado a toda a equipe, com uma carência na saída.
func TestPresenceOnlineOfflineAndGrace(t *testing.T) {
	w := setupWorld(t)
	w.e.svcs.Hub.SetPresenceGrace(300 * time.Millisecond)
	ana, bia := w.users["ana"], w.users["bia"]

	// Sem ninguém conectado, ninguém está online.
	if got := onlineIDs(t, w.admin); len(got) != 0 {
		t.Fatalf("ninguém conectado: %v", got)
	}
	// Sem login não se vê quem está online.
	expect(t, w.e.client().do("GET", "/api/v1/presence", nil), 401)

	anaWS := w.ana.websocket(t)
	eventually(t, "ana online", func() bool { return len(onlineIDs(t, w.admin)) == 1 })
	biaWS := w.bia.websocket(t)
	eventually(t, "ana vê a bia entrar", func() bool { ev := anaWS.presenceEvents(); return len(ev[bia]) == 1 && ev[bia][0] })
	if got := onlineIDs(t, w.ana); len(got) != 2 {
		t.Fatalf("ana e bia online: %v", got)
	}
	// A própria pessoa também recebe o aviso dela (e um segundo aparelho/aba da bia não avisa de novo).
	bia2 := w.bia.websocket(t)
	time.Sleep(150 * time.Millisecond)
	if ev := anaWS.presenceEvents(); len(ev[bia]) != 1 || len(ev[ana]) != 1 {
		t.Fatalf("segunda aba não avisa de novo: %v", ev)
	}

	// Fecha uma das duas abas da bia: continua online.
	biaWS.conn.CloseNow()
	time.Sleep(600 * time.Millisecond) // mais que a carência
	if got := onlineIDs(t, w.ana); len(got) != 2 {
		t.Fatalf("ainda tem uma aba: %v", got)
	}
	// Fecha a última: some depois da carência, e todos são avisados.
	bia2.conn.CloseNow()
	eventually(t, "bia sai da lista", func() bool { return len(onlineIDs(t, w.ana)) == 1 })
	eventually(t, "ana é avisada da saída", func() bool { ev := anaWS.presenceEvents()[bia]; return len(ev) == 2 && !ev[1] })
}

// Um F5 (sai e volta dentro da carência) não pisca "offline" para a equipe.
func TestPresenceReconnectWithinGraceDoesNotFlicker(t *testing.T) {
	w := setupWorld(t)
	w.e.svcs.Hub.SetPresenceGrace(1500 * time.Millisecond)
	bia := w.users["bia"]
	anaWS := w.ana.websocket(t)
	biaWS := w.bia.websocket(t)
	eventually(t, "bia online", func() bool { return len(anaWS.presenceEvents()[bia]) == 1 })

	biaWS.conn.CloseNow()
	eventually(t, "conexão caiu", func() bool { return w.e.svcs.Hub.Count() == 1 })
	if got := onlineIDs(t, w.ana); len(got) != 2 {
		t.Fatalf("durante a carência a bia ainda aparece online: %v", got)
	}
	w.bia.websocket(t) // voltou
	eventually(t, "voltou", func() bool { return w.e.svcs.Hub.Count() == 2 })
	time.Sleep(1800 * time.Millisecond) // passa da carência original
	if ev := anaWS.presenceEvents()[bia]; len(ev) != 1 || !ev[0] {
		t.Fatalf("nenhum aviso de saída nem de nova entrada: %v", ev)
	}
	if got := onlineIDs(t, w.ana); len(got) != 2 {
		t.Fatalf("continua online: %v", got)
	}
}
