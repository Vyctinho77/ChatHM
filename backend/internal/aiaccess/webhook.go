package aiaccess

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/conversations"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
	"github.com/vyctinho77/chathm/backend/internal/platform/secretbox"
)

// Aviso automático: quando o cliente responde, o ChatHM chama um endereço escolhido pelo administrador (webhook), sem esperar o
// agente de IA perguntar. Pensado para quem roda o agente fora de uma conversa (n8n, tarefa agendada, servidor próprio).
//
// O que sai, e o que não sai:
//   - por padrão só vai QUEM respondeu, em qual conversa e fila e quantas respostas; o texto da mensagem não vai (o agente lê pelo MCP,
//     com todas as proteções de privacidade). O administrador pode incluir um trecho, já com dados sensíveis ocultos;
//   - assinado com HMAC-SHA256 (X-ChatHM-Signature: t=<unix>,v1=<hex>, sobre "<t>.<corpo>"); o segredo é gerado aqui, fica cifrado e é mostrado uma vez;
//   - só https e só para endereços públicos (nada de rede interna), sem seguir redirecionamentos;
//   - com reenvio (10 s, 1 min, 5 min, 30 min, 2 h) e respostas em rajada juntas num só aviso; desliga sozinho depois de muitas falhas seguidas.

const (
	webhookDelay        = 5 * time.Second // espera para juntar uma rajada de respostas num só aviso
	webhookMaxAttempts  = 6
	webhookDisableAfter = 20 // falhas seguidas (de avisos esgotados) até desligar sozinho
	webhookTimeout      = 6 * time.Second
	webhookPreviewRunes = 160
	webhookAIWindow     = "30 days"
)

var webhookBackoff = []time.Duration{10 * time.Second, time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour}

type WebhookConfig struct {
	Enabled             bool       `json:"enabled"`
	URL                 string     `json:"url"`
	OnlyAIReplies       bool       `json:"onlyAiReplies"`
	IncludePreview      bool       `json:"includePreview"`
	HasSecret           bool       `json:"hasSecret"`
	ConsecutiveFailures int        `json:"consecutiveFailures"`
	LastDeliveryAt      *time.Time `json:"lastDeliveryAt,omitempty"`
	LastStatus          string     `json:"lastStatus,omitempty"`
	LastError           string     `json:"lastError,omitempty"`
	DisabledReason      string     `json:"disabledReason,omitempty"`
	Pending             int        `json:"pending"`
}

type WebhookInput struct {
	Enabled        bool   `json:"enabled"`
	URL            string `json:"url"`
	OnlyAIReplies  bool   `json:"onlyAiReplies"`
	IncludePreview bool   `json:"includePreview"`
}

// SetBox liga a cifragem do segredo do webhook.
func (s *Service) SetBox(b *secretbox.Box) { s.box = b }

// AllowPrivateWebhooks libera http e endereços internos (só para testes e desenvolvimento).
func (s *Service) AllowPrivateWebhooks() {
	s.webhookPrivate = true
	s.webhookClient = newWebhookClient(true)
}

// --- configuração -----------------------------------------------------------------

func (s *Service) Webhook(ctx context.Context) (WebhookConfig, error) {
	var c WebhookConfig
	var enc []byte
	err := s.db.QueryRow(ctx, `
		SELECT enabled, url, secret_enc, only_ai_replies, include_preview, consecutive_failures, last_delivery_at, last_status, last_error, disabled_reason,
		       (SELECT count(*) FROM ai_webhook_deliveries WHERE status = 'pending')
		  FROM ai_webhook WHERE id`).
		Scan(&c.Enabled, &c.URL, &enc, &c.OnlyAIReplies, &c.IncludePreview, &c.ConsecutiveFailures, &c.LastDeliveryAt, &c.LastStatus, &c.LastError, &c.DisabledReason, &c.Pending)
	if errors.Is(err, pgx.ErrNoRows) {
		return WebhookConfig{OnlyAIReplies: true}, nil
	}
	c.HasSecret = len(enc) > 0
	return c, err
}

func newSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "whsec_" + base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *Service) sealSecret(secret string) ([]byte, error) {
	if s.box == nil {
		return nil, errors.New("criptografia indisponível")
	}
	return s.box.Seal([]byte(secret), []byte("ai_webhook"))
}

func (s *Service) webhookSecret(ctx context.Context) (string, error) {
	var enc []byte
	if err := s.db.QueryRow(ctx, `SELECT secret_enc FROM ai_webhook WHERE id`).Scan(&enc); err != nil {
		return "", err
	}
	if len(enc) == 0 || s.box == nil {
		return "", errors.New("o webhook não tem segredo")
	}
	b, err := s.box.Open(enc, []byte("ai_webhook"))
	if err != nil {
		return "", errors.New("não foi possível ler o segredo (ENCRYPTION_KEY mudou?)")
	}
	return string(b), nil
}

// validateWebhookURL aceita só https com endereço público (em testes, também http e endereços internos).
func validateWebhookURL(raw string, allowPrivate bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || utf8.RuneCountInString(raw) > 500 {
		return "", errors.New("Informe o endereço do webhook (até 500 caracteres).")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", errors.New("Endereço inválido. Exemplo: https://meu-servidor.com.br/chathm.")
	}
	if u.Scheme != "https" && !(allowPrivate && u.Scheme == "http") {
		return "", errors.New("Use um endereço https.")
	}
	if u.User != nil || u.Fragment != "" {
		return "", errors.New("O endereço não pode ter usuário, senha nem #.")
	}
	host := u.Hostname()
	if ip, err := netip.ParseAddr(host); err == nil && !allowPrivate && blockedIP(ip) {
		return "", errors.New("O endereço aponta para uma rede interna; use um endereço público.")
	}
	if strings.EqualFold(host, "localhost") && !allowPrivate {
		return "", errors.New("O endereço aponta para a própria máquina; use um endereço público.")
	}
	return u.String(), nil
}

// blockedIP: rede interna, loopback, link-local (inclui a metadata de nuvem 169.254.169.254), CGNAT, multicast e não especificado.
func blockedIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	cgnat := netip.MustParsePrefix("100.64.0.0/10")
	return cgnat.Contains(ip)
}

// newWebhookClient: o bloqueio é no momento da conexão (depois de resolver o nome), então um DNS que aponte para dentro também é barrado.
func newWebhookClient(allowPrivate bool) *http.Client {
	d := &net.Dialer{Timeout: 4 * time.Second}
	if !allowPrivate {
		d.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip, err := netip.ParseAddr(host)
			if err != nil || blockedIP(ip) {
				return errors.New("destino interno bloqueado")
			}
			return nil
		}
	}
	return &http.Client{
		Timeout:       webhookTimeout,
		Transport:     &http.Transport{DialContext: d.DialContext, DisableKeepAlives: true, Proxy: nil},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// SetWebhook salva a configuração. O segredo é gerado na primeira vez e devolvido UMA vez (secret vazio nas demais).
func (s *Service) SetWebhook(ctx context.Context, actorID, ip string, in WebhookInput) (WebhookConfig, string, error) {
	target := strings.TrimSpace(in.URL)
	if in.Enabled || target != "" {
		v, err := validateWebhookURL(target, s.webhookPrivate)
		if err != nil {
			return WebhookConfig{}, "", httpx.FieldErrors{"url": err.Error()}
		}
		target = v
	}
	var secret string
	var enc []byte
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var have []byte
		if err := tx.QueryRow(ctx, `SELECT secret_enc FROM ai_webhook WHERE id FOR UPDATE`).Scan(&have); err != nil {
			return err
		}
		if len(have) == 0 && (in.Enabled || target != "") {
			sec, err := newSecret()
			if err != nil {
				return err
			}
			if enc, err = s.sealSecret(sec); err != nil {
				return err
			}
			secret = sec
		}
		if _, err := tx.Exec(ctx, `
			UPDATE ai_webhook SET enabled = $1, url = $2, only_ai_replies = $3, include_preview = $4,
			       secret_enc = coalesce($5, secret_enc), updated_at = now(), updated_by = NULLIF($6, '')::uuid,
			       -- religar zera a contagem de falhas e o motivo do desligamento automático
			       consecutive_failures = CASE WHEN $1 THEN 0 ELSE consecutive_failures END,
			       disabled_reason = CASE WHEN $1 THEN '' ELSE disabled_reason END
			 WHERE id`, in.Enabled, target, in.OnlyAIReplies, in.IncludePreview, enc, actorID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "ai.webhook.update", TargetType: "ai_webhook", IP: ip,
			Metadata: map[string]any{"enabled": in.Enabled, "onlyAiReplies": in.OnlyAIReplies, "includePreview": in.IncludePreview, "host": hostOf(target)}})
	})
	if err != nil {
		return WebhookConfig{}, "", err
	}
	cfg, err := s.Webhook(ctx)
	return cfg, secret, err
}

func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Host
	}
	return ""
}

// RotateWebhookSecret troca o segredo e o devolve (uma única vez). Quem valida a assinatura precisa atualizar.
func (s *Service) RotateWebhookSecret(ctx context.Context, actorID, ip string) (string, error) {
	sec, err := newSecret()
	if err != nil {
		return "", err
	}
	enc, err := s.sealSecret(sec)
	if err != nil {
		return "", err
	}
	err = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE ai_webhook SET secret_enc = $1, updated_at = now(), updated_by = NULLIF($2, '')::uuid WHERE id`, enc, actorID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "ai.webhook.rotate_secret", TargetType: "ai_webhook", IP: ip})
	})
	return sec, err
}

// --- entrega ----------------------------------------------------------------------

// sign monta o cabeçalho de assinatura: t=<unix>,v1=<hex(hmac_sha256(segredo, "<t>.<corpo>"))>.
func sign(secret string, ts int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.", ts)
	mac.Write(body)
	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

// VerifyWebhookSignature é o que o receptor faz (exposto para testes e documentação): confere o hash e a tolerância de tempo.
func VerifyWebhookSignature(secret, header string, body []byte, now time.Time, tolerance time.Duration) bool {
	var t int64
	var v1 string
	for _, part := range strings.Split(header, ",") {
		k, val, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch k {
		case "t":
			fmt.Sscanf(val, "%d", &t)
		case "v1":
			v1 = val
		}
	}
	if t == 0 || v1 == "" {
		return false
	}
	if d := now.Sub(time.Unix(t, 0)); d > tolerance || d < -tolerance {
		return false
	}
	return hmac.Equal([]byte(sign(secret, t, body)), []byte(fmt.Sprintf("t=%d,v1=%s", t, v1)))
}

type webhookDelivery struct {
	ID, ConversationID, LastMessageID string
	Count                             int
	FirstAt                           time.Time
	Attempts                          int
}

func (s *Service) post(ctx context.Context, target, secret, event, deliveryID string, body []byte) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ChatHM-Webhook/1")
	req.Header.Set("X-ChatHM-Event", event)
	req.Header.Set("X-ChatHM-Delivery", deliveryID)
	req.Header.Set("X-ChatHM-Signature", sign(secret, time.Now().Unix(), body))
	resp, err := s.webhookClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode/100 != 2 {
		return resp.StatusCode, fmt.Errorf("o receptor respondeu %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

// webhookError reduz o erro a algo curto e sem o endereço completo (que pode ter parâmetros).
func webhookError(err error) string {
	switch {
	case err == nil:
		return ""
	case strings.Contains(err.Error(), "destino interno bloqueado"):
		return "o endereço aponta para uma rede interna (bloqueado)"
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "Client.Timeout") || strings.Contains(err.Error(), "timeout"):
		return "o receptor não respondeu a tempo"
	case strings.Contains(err.Error(), "o receptor respondeu"):
		return err.Error()
	}
	return "não foi possível conectar ao receptor"
}

// OnInbound registra o aviso de uma mensagem recebida (chamado depois que a mensagem foi gravada). Nunca devolve erro: um problema aqui
// não pode atrapalhar o recebimento de mensagens.
func (s *Service) OnInbound(ctx context.Context, ev conversations.InboundEvent) {
	defer func() { _ = recover() }()
	if ev.Group || ev.ConversationID == "" || ev.MessageID == "" {
		return
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO ai_webhook_deliveries (conversation_id, first_message_id, last_message_id, next_attempt_at)
		SELECT $1::uuid, $2::uuid, $2::uuid, now() + $3::interval
		 WHERE (SELECT w.enabled AND w.url <> '' AND coalesce((SELECT a.enabled FROM ai_settings a WHERE a.id), false) FROM ai_webhook w WHERE w.id)
		   AND (NOT (SELECT only_ai_replies FROM ai_webhook WHERE id) OR EXISTS (
		        SELECT 1 FROM messages m WHERE m.conversation_id = $1::uuid AND m.direction = 'out' AND m.payload->>'automated' = 'true'
		           AND m.sent_at > now() - $4::interval))
		ON CONFLICT (conversation_id) WHERE status = 'pending'
		DO UPDATE SET last_message_id = $2::uuid, message_count = ai_webhook_deliveries.message_count + 1`,
		ev.ConversationID, ev.MessageID, webhookDelay.String(), webhookAIWindow)
	_ = err
}

// buildEvent monta o aviso com os dados de AGORA da conversa (o que mudou entre o aviso e o envio já vem atualizado).
func (s *Service) buildEvent(ctx context.Context, d webhookDelivery, includePreview bool) (map[string]any, error) {
	var status, segKey, segName, contactID, contactName, kind string
	var assignee *string
	var replyToAI bool
	var lastAt time.Time
	err := s.db.QueryRow(ctx, `
		SELECT c.status, coalesce(sg.key, 'geral'), coalesce(sg.name::text, 'Não atribuídos'), ct.id::text, ct.name, u.name::text,
		       coalesce(m.kind, 'text'), coalesce(m.sent_at, now()),
		       EXISTS (SELECT 1 FROM messages x WHERE x.conversation_id = c.id AND x.direction = 'out' AND x.payload->>'automated' = 'true' AND x.sent_at > now() - $2::interval)
		  FROM conversations c
		  JOIN contacts ct ON ct.id = c.contact_id
		  LEFT JOIN segments sg ON sg.id = c.segment_id
		  LEFT JOIN users u ON u.id = c.assignee_id
		  LEFT JOIN messages m ON m.id = $3::uuid
		 WHERE c.id = $1`, d.ConversationID, webhookAIWindow, nullUUID(d.LastMessageID)).
		Scan(&status, &segKey, &segName, &contactID, &contactName, &assignee, &kind, &lastAt, &replyToAI)
	if err != nil {
		return nil, err
	}
	st, err := s.Settings(ctx)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(contactName)
	if st.StrictPrivacy {
		name = shortName(name)
	}
	ev := map[string]any{
		"type": "message.received", "id": d.ID, "createdAt": time.Now().UTC().Format(time.RFC3339),
		"conversation": map[string]any{"id": d.ConversationID, "status": statusPT[status], "queue": map[string]any{"key": segKey, "name": segName}},
		"contact":      map[string]any{"id": contactID, "name": name},
		"messages":     map[string]any{"count": d.Count, "firstAt": d.FirstAt.UTC().Format(time.RFC3339), "lastAt": lastAt.UTC().Format(time.RFC3339), "lastMessageId": d.LastMessageID, "lastKind": kind},
		"replyToAi":    replyToAI,
		"next":         "Use a ferramenta list_pending_replies do MCP para ler as respostas e mark_replies para marcar o que já foi tratado.",
	}
	if assignee != nil {
		ev["conversation"].(map[string]any)["assignee"] = *assignee
	}
	if includePreview {
		var body, caption string
		_ = s.db.QueryRow(ctx, `SELECT coalesce(body, ''), coalesce(payload->>'caption', '') FROM messages WHERE id = $1::uuid`, nullUUID(d.LastMessageID)).Scan(&body, &caption)
		text := strings.TrimSpace(body)
		if text == "" {
			text = strings.TrimSpace(caption)
		}
		text = Masker{Documents: true, Phones: true, Emails: true, Addresses: true}.Text(text) // quem recebe é um sistema externo: tudo oculto
		if r := []rune(text); len(r) > webhookPreviewRunes {
			text = string(r[:webhookPreviewRunes]) + "…"
		}
		if text != "" {
			ev["preview"] = text
		}
	}
	return ev, nil
}

func nullUUID(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *Service) claimDeliveries(ctx context.Context) ([]webhookDelivery, error) {
	// O próprio "next_attempt_at" vira a reserva: se o servidor cair no meio, a entrega volta sozinha em 2 minutos.
	rows, err := s.db.Query(ctx, `
		UPDATE ai_webhook_deliveries d SET attempts = d.attempts + 1, next_attempt_at = now() + interval '2 minutes'
		 WHERE d.id IN (SELECT id FROM ai_webhook_deliveries WHERE status = 'pending' AND next_attempt_at <= now()
		                 ORDER BY next_attempt_at LIMIT 10 FOR UPDATE SKIP LOCKED)
		RETURNING d.id::text, d.conversation_id::text, coalesce(d.last_message_id::text, ''), d.message_count, d.first_at, d.attempts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []webhookDelivery
	for rows.Next() {
		var d webhookDelivery
		if err := rows.Scan(&d.ID, &d.ConversationID, &d.LastMessageID, &d.Count, &d.FirstAt, &d.Attempts); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeliverDue entrega os avisos que venceram e devolve quantos saíram (usado pelo laço e pelos testes).
func (s *Service) DeliverDue(ctx context.Context) int {
	cfg, err := s.Webhook(ctx)
	if err != nil {
		return 0
	}
	if !cfg.Enabled || cfg.URL == "" {
		// Desligado: o que estava pendente não deve sair depois (religar não despeja avisos velhos).
		_, _ = s.db.Exec(ctx, `UPDATE ai_webhook_deliveries SET status = 'failed', last_error = 'webhook desligado' WHERE status = 'pending'`)
		return 0
	}
	secret, err := s.webhookSecret(ctx)
	if err != nil {
		return 0
	}
	list, err := s.claimDeliveries(ctx)
	if err != nil {
		return 0
	}
	sent := 0
	for _, d := range list {
		ev, err := s.buildEvent(ctx, d, cfg.IncludePreview)
		if errors.Is(err, pgx.ErrNoRows) { // a conversa foi apagada
			_, _ = s.db.Exec(ctx, `UPDATE ai_webhook_deliveries SET status = 'failed', last_error = 'conversa removida' WHERE id = $1`, d.ID)
			continue
		}
		var status int
		if err == nil {
			var body []byte
			if body, err = json.Marshal(ev); err == nil {
				cctx, cancel := context.WithTimeout(ctx, webhookTimeout+2*time.Second)
				status, err = s.post(cctx, cfg.URL, secret, "message.received", d.ID, body)
				cancel()
			}
		}
		s.finishDelivery(ctx, d, status, err)
		if err == nil {
			sent++
		}
	}
	return sent
}

func (s *Service) finishDelivery(ctx context.Context, d webhookDelivery, status int, err error) {
	if err == nil {
		_, _ = s.db.Exec(ctx, `UPDATE ai_webhook_deliveries SET status = 'sent', sent_at = now(), last_error = '' WHERE id = $1`, d.ID)
		_, _ = s.db.Exec(ctx, `UPDATE ai_webhook SET consecutive_failures = 0, last_delivery_at = now(), last_status = $1, last_error = '' WHERE id`, fmt.Sprint(status))
		return
	}
	msg := webhookError(err)
	if d.Attempts >= webhookMaxAttempts { // desiste deste aviso
		_, _ = s.db.Exec(ctx, `UPDATE ai_webhook_deliveries SET status = 'failed', last_error = $2 WHERE id = $1`, d.ID, msg)
		var n int
		_ = s.db.QueryRow(ctx, `UPDATE ai_webhook SET consecutive_failures = consecutive_failures + 1, last_delivery_at = now(), last_status = $1, last_error = $2
			 WHERE id RETURNING consecutive_failures`, statusText(status), msg).Scan(&n)
		if n >= webhookDisableAfter {
			_, _ = s.db.Exec(ctx, `UPDATE ai_webhook SET enabled = false, disabled_reason = $1 WHERE id`,
				fmt.Sprintf("Desligado sozinho depois de %d avisos seguidos sem entrega. Último erro: %s", n, msg))
		}
		return
	}
	wait := webhookBackoff[min(d.Attempts-1, len(webhookBackoff)-1)]
	_, _ = s.db.Exec(ctx, `UPDATE ai_webhook_deliveries SET next_attempt_at = now() + $2::interval, last_error = $3 WHERE id = $1`, d.ID, wait.String(), msg)
	_, _ = s.db.Exec(ctx, `UPDATE ai_webhook SET last_status = $1, last_error = $2 WHERE id`, statusText(status), msg)
}

func statusText(status int) string {
	if status == 0 {
		return "sem resposta"
	}
	return fmt.Sprint(status)
}

// RunWebhooks é o laço de entrega.
func (s *Service) RunWebhooks(ctx context.Context) {
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for n := 0; ; n++ {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.DeliverDue(ctx)
			if n%1200 == 0 { // de tempos em tempos, limpa o histórico de entregas antigas
				_, _ = s.db.Exec(ctx, `DELETE FROM ai_webhook_deliveries WHERE status <> 'pending' AND created_at < now() - interval '14 days'`)
			}
		}
	}
}

type WebhookTestResult struct {
	OK         bool   `json:"ok"`
	Status     int    `json:"status,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"durationMs"`
}

// TestWebhook envia um aviso de teste (type = webhook.test) e devolve o resultado na hora.
func (s *Service) TestWebhook(ctx context.Context) (WebhookTestResult, error) {
	cfg, err := s.Webhook(ctx)
	if err != nil {
		return WebhookTestResult{}, err
	}
	if cfg.URL == "" || !cfg.HasSecret {
		return WebhookTestResult{}, httpx.FieldErrors{"url": "Salve o endereço do webhook antes de testar."}
	}
	secret, err := s.webhookSecret(ctx)
	if err != nil {
		return WebhookTestResult{}, err
	}
	id := newID()
	body, _ := json.Marshal(map[string]any{"type": "webhook.test", "id": id, "createdAt": time.Now().UTC().Format(time.RFC3339),
		"message": "Teste do ChatHM: se você recebeu isto e a assinatura confere, o aviso automático está funcionando."})
	start := time.Now()
	cctx, cancel := context.WithTimeout(ctx, webhookTimeout+2*time.Second)
	defer cancel()
	status, err := s.post(cctx, cfg.URL, secret, "webhook.test", id, body)
	res := WebhookTestResult{OK: err == nil, Status: status, DurationMs: time.Since(start).Milliseconds(), Error: webhookError(err)}
	return res, nil
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "evt_" + hex.EncodeToString(b)
}
