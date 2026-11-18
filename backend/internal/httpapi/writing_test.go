package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// Medição do corretor: qualquer pessoa logada envia contadores por palavra; só o administrador lê; nada de texto nem de quem digitou.
func TestWritingStatsRecordAndReport(t *testing.T) {
	w := setupWorld(t)
	post := func(c *client, entries any) resp {
		return c.do("POST", "/api/v1/writing/stats", map[string]any{"entries": entries})
	}
	e := func(layer, from, to string, applied, undone int) map[string]any {
		return map[string]any{"layer": layer, "from": from, "to": to, "applied": applied, "undone": undone}
	}

	// Pessoas diferentes enviam; os contadores se somam (e a caixa das palavras não importa).
	expect(t, post(w.ana, []map[string]any{e("curated", "Voce", "você", 40, 1), e("typo_tr", "tmepo", "tempo", 6, 0), e("extended", "marcio", "márcio", 8, 6)}), 204)
	expect(t, post(w.bia, []map[string]any{e("curated", "voce", "Você", 10, 0), e("extended", "marcio", "márcio", 2, 3)}), 204)

	// Só administrador lê o relatório.
	expect(t, w.ana.do("GET", "/api/v1/writing/stats", nil), 403)
	r := w.admin.do("GET", "/api/v1/writing/stats?min=5", nil)
	expect(t, r, 200)
	layers := map[string]map[string]any{}
	for _, x := range r.body["layers"].([]any) {
		m := x.(map[string]any)
		layers[m["layer"].(string)] = m
	}
	if layers["curated"]["applied"].(float64) != 50 || layers["curated"]["undone"].(float64) != 1 || layers["curated"]["words"].(float64) != 1 {
		t.Fatalf("camada curated (soma das duas pessoas): %v", layers["curated"])
	}
	if layers["extended"]["undoRate"].(float64) != 0.9 {
		t.Fatalf("taxa de desfeitas de extended: %v", layers["extended"])
	}
	// "marcio": 10 aplicadas, 9 desfeitas: a candidata a sair da lista.
	sus := r.body["suspicious"].([]any)
	if len(sus) == 0 || sus[0].(map[string]any)["from"] != "marcio" || sus[0].(map[string]any)["undone"].(float64) != 9 {
		t.Fatalf("suspeitas: %v", sus)
	}
	// Com min alto, quem tem poucas aplicações some da lista de suspeitas.
	if s := w.admin.do("GET", "/api/v1/writing/stats?min=60", nil).body["suspicious"].([]any); len(s) != 0 {
		t.Fatalf("min=60: %v", s)
	}
	// Sem login não envia nem lê.
	anon := w.e.client()
	expect(t, anon.do("POST", "/api/v1/writing/stats", map[string]any{"entries": []map[string]any{e("curated", "voce", "você", 1, 0)}}), 401)
	expect(t, anon.do("GET", "/api/v1/writing/stats", nil), 401)
	// O que ninguém desfez aparece em "popular", a mais usada primeiro.
	pop := r.body["popular"].([]any)
	if len(pop) != 1 || pop[0].(map[string]any)["from"] != "tmepo" {
		// "voce" foi desfeita uma vez, então não é "popular".
		t.Fatalf("populares: %v", pop)
	}
	// Palavras guardadas em minúsculas.
	if n := scalar[int](t, w, `SELECT count(*) FROM writing_stats WHERE from_word <> lower(from_word) OR to_word <> lower(to_word)`); n != 0 {
		t.Fatalf("palavras devem ser guardadas em minúsculas: %d", n)
	}
}

func TestWritingStatsValidationAndPrivacy(t *testing.T) {
	w := setupWorld(t)
	ok := map[string]any{"layer": "curated", "from": "voce", "to": "você", "applied": 1, "undone": 0}
	with := func(k string, v any) map[string]any {
		m := map[string]any{}
		for a, b := range ok {
			m[a] = b
		}
		m[k] = v
		return m
	}
	post := func(entries any) resp {
		return w.ana.do("POST", "/api/v1/writing/stats", map[string]any{"entries": entries})
	}

	for name, entries := range map[string]any{
		"camada desconhecida":     []map[string]any{with("layer", "inventada")},
		"frase em vez de palavra": []map[string]any{with("from", "oi tudo bem")},
		"número":                  []map[string]any{with("from", "voce2")},
		"palavra longa demais":    []map[string]any{with("to", strings.Repeat("a", 41))},
		"vazia":                   []map[string]any{with("from", "")},
		"contador negativo":       []map[string]any{with("applied", -1)},
		"contador enorme":         []map[string]any{with("applied", 1001)},
		"tudo zero":               []map[string]any{with("applied", 0)},
		"nenhum item":             []map[string]any{},
		"itens demais (201)": func() []map[string]any {
			l := make([]map[string]any, 201)
			for i := range l {
				l[i] = ok
			}
			return l
		}(),
		"um ruim recusa o lote": []map[string]any{ok, with("layer", "x")},
	} {
		if r := post(entries); r.status != http.StatusUnprocessableEntity {
			t.Errorf("%s: status %d (esperava 422)", name, r.status)
		}
	}
	if n := scalar[int](t, w, `SELECT count(*) FROM writing_stats`); n != 0 {
		t.Fatalf("nada inválido pode ter sido guardado (nem parte do lote): %d", n)
	}
	// 200 itens é o limite aceito.
	many := make([]map[string]any, 200)
	for i := range many {
		many[i] = with("from", "voce"+strings.Repeat("a", i%30))
		many[i]["to"] = "você"
	}
	expect(t, post(many), 204)

	// Campo a mais (por exemplo, texto da mensagem ou o usuário) é recusado, não ignorado.
	if r := w.ana.do("POST", "/api/v1/writing/stats", map[string]any{"entries": []map[string]any{with("text", "Oi, meu cpf é 123")}}); r.status != http.StatusBadRequest {
		t.Fatalf("campo desconhecido: %d", r.status)
	}
	if r := w.ana.do("POST", "/api/v1/writing/stats", map[string]any{"entries": []map[string]any{ok}, "userId": "x"}); r.status != http.StatusBadRequest {
		t.Fatalf("campo desconhecido na raiz: %d", r.status)
	}
	// A tabela não tem onde guardar quem digitou nem o texto.
	cols := scalar[string](t, w, `SELECT string_agg(column_name, ',' ORDER BY column_name) FROM information_schema.columns WHERE table_name = 'writing_stats'`)
	if cols != "applied,first_seen,from_word,layer,to_word,undone,updated_at" {
		t.Fatalf("colunas de writing_stats: %s", cols)
	}
}
