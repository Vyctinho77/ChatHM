package aiaccess

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/vyctinho77/chathm/backend/internal/conversations"
)

// Respostas por HTTP simples, para quem NÃO tem endereço público (app no computador, agente numa máquina atrás de roteador).
// Em vez de o ChatHM chamar o agente (webhook), o agente chama o ChatHM: GET /mcp/replies espera até ~50 s por uma resposta de cliente ainda não
// tratada e devolve assim que ela chega (ou volta vazio, e a quem chamou basta chamar de novo). É o mesmo que a ferramenta wait_for_replies,
// sem precisar de um cliente MCP, e com espera maior. O script scripts/chathm-listen.mjs faz esse laço 24 horas e acorda o agente.
const httpWaitMax = 50 * time.Second // abaixo do WriteTimeout do servidor (60 s)

func (m *MCP) ServeReplies(w http.ResponseWriter, r *http.Request) {
	p, ok := m.authenticate(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	wait := httpWaitMax
	if n, err := strconv.Atoi(q.Get("wait")); err == nil && n >= 0 && time.Duration(n)*time.Second < httpWaitMax {
		wait = time.Duration(n) * time.Second
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(wait + 10*time.Second))

	v := conversations.Viewer{ID: p.UserID, Name: p.Name, Admin: p.Admin, Supervisor: p.Manager}
	in := listIn{Since: q.Get("since"), Limit: limit, IncludeHandled: false}
	deadline := time.Now().Add(wait)
	tick := time.NewTicker(waitPoll)
	defer tick.Stop()
	for {
		res, err := m.pendingList(r.Context(), p, v, in)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		out, _ := res.(map[string]any)
		if n, _ := out["total"].(int); n > 0 || !time.Now().Before(deadline) {
			out["timedOut"] = n == 0
			m.svc.Record(r.Context(), p, "http_replies", map[string]any{"total": n}, true)
			m.writeMasked(w, r.Context(), p, out)
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
		}
	}
}

// writeMasked: a mesma proteção de privacidade das ferramentas MCP (o agente recebe o que receberia por elas).
func (m *MCP) writeMasked(w http.ResponseWriter, ctx context.Context, p Principal, out map[string]any) {
	st, err := m.svc.Settings(ctx)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal", "Erro interno.")
		return
	}
	b, _ := json.Marshal(out)
	var g map[string]any
	_ = json.Unmarshal(b, &g)
	g["aviso"] = untrustedNote
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(MaskerFor(st, p).Walk(g, p.OAuth))
}
