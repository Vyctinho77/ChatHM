package httpapi

// Acesso por IA (MCP): tokens pessoais, chave do administrador, visibilidade por equipe, mascaramento e auditoria.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// mcpCall envia uma requisição JSON-RPC ao /mcp com o token (vazio = sem cabeçalho).
func (w *world) mcpCall(token, method string, params any) (int, map[string]any) {
	w.e.t.Helper()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	req, _ := http.NewRequest("POST", w.e.srv.URL+"/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		w.e.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out
}

// tool chama uma ferramenta e devolve (conteúdo estruturado, erro?).
func (w *world) tool(token, name string, args map[string]any) (map[string]any, bool) {
	w.e.t.Helper()
	status, out := w.mcpCall(token, "tools/call", map[string]any{"name": name, "arguments": args})
	if status != 200 {
		w.e.t.Fatalf("tools/call %s: HTTP %d %v", name, status, out)
	}
	res, _ := out["result"].(map[string]any)
	if res == nil {
		w.e.t.Fatalf("tools/call %s sem result: %v", name, out)
	}
	isErr, _ := res["isError"].(bool)
	sc, _ := res["structuredContent"].(map[string]any)
	return sc, isErr
}

func (w *world) mkToken(c *client, name string) string {
	w.e.t.Helper()
	r := c.do("POST", "/api/v1/ai/tokens", map[string]any{"name": name})
	expect(w.e.t, r, 201)
	tok, _ := r.body["token"].(string)
	if !strings.HasPrefix(tok, "chm_") {
		w.e.t.Fatalf("token = %q", tok)
	}
	return tok
}

func TestAIAccessTokensAndSwitch(t *testing.T) {
	w := setupWorld(t)

	// Desligado por padrão: ninguém cria token nem usa o /mcp.
	r := w.ana.do("GET", "/api/v1/ai/settings", nil)
	expect(t, r, 200)
	if r.body["enabled"] != false || r.body["maskDocuments"] != true {
		t.Fatalf("padrão = %v", r.body)
	}
	expect(t, w.ana.do("POST", "/api/v1/ai/tokens", map[string]any{"name": "x"}), 409)
	// Só administrador altera a configuração.
	expect(t, w.ana.do("PUT", "/api/v1/ai/settings", map[string]any{"enabled": true, "maskDocuments": true}), 403)
	expect(t, w.admin.do("PUT", "/api/v1/ai/settings", map[string]any{"enabled": true, "maskDocuments": true, "maskPhones": false}), 200)

	// Validação do nome e do prazo.
	expect(t, w.ana.do("POST", "/api/v1/ai/tokens", map[string]any{"name": "  "}), 422)
	expect(t, w.ana.do("POST", "/api/v1/ai/tokens", map[string]any{"name": "ok", "expiresInDays": 9999}), 422)

	tok := w.mkToken(w.ana, "Claude no notebook")
	// A listagem nunca devolve o segredo; o prefixo identifica o token.
	r = w.ana.do("GET", "/api/v1/ai/tokens", nil)
	expect(t, r, 200)
	list := r.body["tokens"].([]any)
	if len(list) != 1 {
		t.Fatalf("tokens = %v", list)
	}
	item := list[0].(map[string]any)
	if item["prefix"] != tok[:10] || item["token"] != nil || item["lastUsedAt"] != nil {
		t.Fatalf("item = %v", item)
	}
	// Outra pessoa não vê nem revoga o token alheio; o administrador vê todos.
	r = w.bia.do("GET", "/api/v1/ai/tokens", nil)
	if n := len(r.body["tokens"].([]any)); n != 0 {
		t.Fatalf("bia vê %d tokens de outra pessoa", n)
	}
	expect(t, w.bia.do("DELETE", "/api/v1/ai/tokens/"+item["id"].(string), nil), 404)
	r = w.admin.do("GET", "/api/v1/ai/tokens?all=1", nil)
	if n := len(r.body["tokens"].([]any)); n != 1 || r.body["tokens"].([]any)[0].(map[string]any)["owner"] == nil {
		t.Fatalf("admin deveria ver todos com o dono: %v", r.body)
	}

	// Sem token / token errado: 401. Com token: lista as ferramentas.
	if st, _ := w.mcpCall("", "tools/list", nil); st != 401 {
		t.Fatalf("sem token = %d", st)
	}
	if st, _ := w.mcpCall("chm_naoexiste", "tools/list", nil); st != 401 {
		t.Fatalf("token errado = %d", st)
	}
	st, out := w.mcpCall(tok, "tools/list", nil)
	if st != 200 {
		t.Fatalf("tools/list = %d %v", st, out)
	}
	names := map[string]bool{}
	for _, x := range out["result"].(map[string]any)["tools"].([]any) {
		names[x.(map[string]any)["name"].(string)] = true
	}
	for _, n := range []string{"list_conversations", "get_conversation", "read_messages", "search", "list_contacts", "get_contact", "list_queues", "list_tags", "list_agenda"} {
		if !names[n] {
			t.Fatalf("ferramenta %s ausente: %v", n, names)
		}
	}
	// O uso foi registrado.
	r = w.ana.do("GET", "/api/v1/ai/tokens", nil)
	if r.body["tokens"].([]any)[0].(map[string]any)["lastUsedAt"] == nil {
		t.Fatal("último uso não registrado")
	}

	// Desligar o acesso derruba todos os tokens na hora (403); ligar de novo restaura.
	expect(t, w.admin.do("PUT", "/api/v1/ai/settings", map[string]any{"enabled": false, "maskDocuments": true}), 200)
	if st, _ := w.mcpCall(tok, "tools/list", nil); st != 403 {
		t.Fatalf("desligado = %d", st)
	}
	expect(t, w.admin.do("PUT", "/api/v1/ai/settings", map[string]any{"enabled": true, "maskDocuments": true}), 200)
	if st, _ := w.mcpCall(tok, "tools/list", nil); st != 200 {
		t.Fatalf("religado = %d", st)
	}

	// Revogado: 401.
	expect(t, w.ana.do("DELETE", "/api/v1/ai/tokens/"+item["id"].(string), nil), 204)
	if st, _ := w.mcpCall(tok, "tools/list", nil); st != 401 {
		t.Fatalf("revogado = %d", st)
	}
	// Pessoa desativada: token não vale.
	tok2 := w.mkToken(w.bia, "Bia")
	if _, err := w.e.pool.Exec(t.Context(), `UPDATE users SET is_active = false WHERE email = (SELECT email FROM users WHERE name = 'bia' OR email LIKE 'bia@%' LIMIT 1)`); err != nil {
		t.Fatal(err)
	}
	if st, _ := w.mcpCall(tok2, "tools/list", nil); st != 401 {
		t.Fatalf("desativada = %d", st)
	}
}

func TestAIAccessVisibilityMaskingAndAudit(t *testing.T) {
	w := setupWorld(t)
	expect(t, w.admin.do("PUT", "/api/v1/ai/settings", map[string]any{"enabled": true, "maskDocuments": true, "maskPhones": false}), 200)
	anaTok := w.mkToken(w.ana, "ana")
	biaTok := w.mkToken(w.bia, "bia")
	adminTok := w.mkToken(w.admin, "admin")

	// Conversa da fila Folha (equipe da ana), com CPF e CNPJ no texto.
	w.receive("Cliente Folha", "Preciso da folha de pagamento. CPF 123.456.789-01, CNPJ 12.345.678/0001-95 e celular 92988001111")
	cid := mustConv(t, w.ana, "Cliente Folha")["id"].(string)

	// ana (Folha) enxerga; bia (Notas) não enxerga nem lendo direto pelo id.
	out, isErr := w.tool(anaTok, "list_conversations", map[string]any{})
	if isErr || len(out["conversations"].([]any)) == 0 {
		t.Fatalf("ana deveria ver a conversa: %v", out)
	}
	out, _ = w.tool(biaTok, "list_conversations", map[string]any{})
	for _, c := range out["conversations"].([]any) {
		if c.(map[string]any)["id"] == cid {
			t.Fatal("bia viu conversa de outra equipe pelo MCP")
		}
	}
	if _, isErr := w.tool(biaTok, "get_conversation", map[string]any{"id": cid}); !isErr {
		t.Fatal("get_conversation de outra equipe deveria dar erro")
	}
	if _, isErr := w.tool(biaTok, "read_messages", map[string]any{"conversation_id": cid}); !isErr {
		t.Fatal("read_messages de outra equipe deveria dar erro")
	}
	if sr, _ := w.tool(biaTok, "search", map[string]any{"q": "folha de pagamento"}); len(sr["messages"].([]any)) != 0 {
		t.Fatalf("busca da bia vazou mensagens de outra equipe: %v", sr)
	}

	// Leitura: texto presente, documentos mascarados, celular preservado, aviso de terceiros, mídia sem URL.
	out, isErr = w.tool(anaTok, "read_messages", map[string]any{"conversation_id": cid})
	if isErr {
		t.Fatalf("read_messages: %v", out)
	}
	if out["aviso"] == nil {
		t.Fatal("falta o aviso de conteúdo de terceiros")
	}
	text := out["messages"].([]any)[0].(map[string]any)["text"].(string)
	if strings.Contains(text, "123.456.789-01") || strings.Contains(text, "12.345.678/0001-95") || !strings.Contains(text, "[CPF oculto]") || !strings.Contains(text, "[CNPJ oculto]") {
		t.Fatalf("documentos deveriam estar mascarados: %q", text)
	}
	if !strings.Contains(text, "92988001111") {
		t.Fatalf("celular não deveria ser ocultado com maskPhones desligado: %q", text)
	}

	// Administrador pode liberar os documentos.
	expect(t, w.admin.do("PUT", "/api/v1/ai/settings", map[string]any{"enabled": true, "maskDocuments": false, "maskPhones": true}), 200)
	out, _ = w.tool(adminTok, "read_messages", map[string]any{"conversation_id": cid})
	text = out["messages"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "123.456.789-01") || strings.Contains(text, "92988001111") {
		t.Fatalf("documentos liberados e telefone mascarado: %q", text)
	}

	// Outras ferramentas respondem.
	for _, c := range []struct {
		name string
		args map[string]any
	}{
		{"get_conversation", map[string]any{"id": cid}},
		{"list_contacts", map[string]any{"q": "folha"}},
		{"list_queues", map[string]any{}},
		{"list_tags", map[string]any{}},
		{"list_agenda", map[string]any{}},
		{"search", map[string]any{"q": "Cliente Folha"}},
	} {
		if o, isErr := w.tool(anaTok, c.name, c.args); isErr || o == nil {
			t.Fatalf("%s falhou: %v", c.name, o)
		}
	}
	if _, isErr := w.tool(anaTok, "read_messages", map[string]any{"conversation_id": cid, "before": "ontem"}); !isErr {
		t.Fatal("before inválido deveria dar erro")
	}

	// Auditoria: chamadas registradas, sem o conteúdo das mensagens.
	var n int
	var leaked bool
	_ = w.e.pool.QueryRow(t.Context(), `SELECT count(*), bool_or(metadata::text ILIKE '%folha de pagamento%') FROM audit_logs WHERE action = 'mcp.call'`).Scan(&n, &leaked)
	if n < 5 || leaked {
		t.Fatalf("auditoria: %d chamadas, vazou conteúdo=%v", n, leaked)
	}
}

func TestAIAccessRateLimitOriginAndMFAReset(t *testing.T) {
	w := setupWorld(t)
	expect(t, w.admin.do("PUT", "/api/v1/ai/settings", map[string]any{"enabled": true, "maskDocuments": true}), 200)
	tok := w.mkToken(w.ana, "ana")

	// Origem de navegador não autorizada é recusada (proteção contra DNS rebinding).
	body := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	req, _ := http.NewRequest("POST", w.e.srv.URL+"/mcp", body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Origin", "https://site-malicioso.example")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatalf("origem estranha = %d", res.StatusCode)
	}

	// Limite por token.
	w.e.svcs.AI.SetRateLimit(3)
	for i := 0; i < 3; i++ {
		if st, _ := w.mcpCall(tok, "tools/list", nil); st != 200 {
			t.Fatalf("chamada %d = %d", i, st)
		}
	}
	if st, _ := w.mcpCall(tok, "tools/list", nil); st != 429 {
		t.Fatalf("acima do limite deveria ser 429, foi %d", st)
	}
	w.e.svcs.AI.SetRateLimit(60)

	// Redefinir o autenticador da pessoa revoga os tokens dela.
	var uid string
	_ = w.e.pool.QueryRow(t.Context(), `SELECT user_id::text FROM api_tokens LIMIT 1`).Scan(&uid)
	expect(t, w.admin.do("POST", "/api/v1/users/"+uid+"/invite", nil), 200)
	if st, _ := w.mcpCall(tok, "tools/list", nil); st != 401 {
		t.Fatalf("depois do reinvite = %d", st)
	}
}
