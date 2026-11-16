package automation

import (
	"context"
	"fmt"
	"time"
)

// PreflightInput: "esse envio passaria agora?". Mode é "automatic" (send_automatic_message, exige Category) ou "attendant" (send_as_attendant).
type PreflightInput struct {
	ContactID string
	Mode      string
	Category  string
}

// PreflightResult é só informação: nada é enfileirado, enviado nem gravado. A resposta vale para este instante; o envio real confere tudo de novo.
type PreflightResult struct {
	Mode      string    `json:"mode"`
	WouldSend bool      `json:"wouldSend"` // nenhum bloqueio agora
	Blockers  []Refusal `json:"blockers"`  // o que recusaria o envio (mesmos códigos das recusas reais)
	Notes     []string  `json:"notes"`     // não bloqueiam, mas importam (ex.: fora da janela: o envio espera)
	Contact   struct {
		LastCustomerMessageAt *time.Time `json:"lastCustomerMessageAt,omitempty"`
		HoursSinceCustomer    *float64   `json:"hoursSinceCustomerMessage,omitempty"`
	} `json:"contact"`
	Window *struct {
		OpenNow      bool       `json:"openNow"`
		NextOpensAt  *time.Time `json:"nextOpensAt,omitempty"`
		Timezone     string     `json:"timezone"`
		CategoryName string     `json:"category"`
	} `json:"window,omitempty"`
	Limits map[string]any `json:"limits,omitempty"`
}

// Preflight responde se um envio passaria hoje, sem fazê-lo. Reaproveita as mesmas regras do envio real.
func (s *Service) Preflight(ctx context.Context, in PreflightInput) (PreflightResult, error) {
	res := PreflightResult{Mode: in.Mode, Blockers: []Refusal{}, Notes: []string{}}
	block := func(r *Refusal) { res.Blockers = append(res.Blockers, *r) }

	st, err := s.Settings(ctx)
	if err != nil {
		return res, err
	}
	if st.Paused {
		msg := "Os envios da IA estão pausados pelo administrador."
		if st.PausedReason != "" {
			msg += " Motivo: " + st.PausedReason
		}
		block(refuse("paused", "%s", msg))
	}

	// O que se sabe do cliente, valha a regra ou não: quando ele escreveu pela última vez.
	var last *time.Time
	if err := s.db.QueryRow(ctx, `
		SELECT max(m.sent_at) FROM messages m JOIN conversations c ON c.id = m.conversation_id
		 WHERE c.contact_id = $1 AND m.direction = 'in' AND coalesce(m.payload->>'historical', '') <> 'true'`, in.ContactID).Scan(&last); err == nil && last != nil {
		res.Contact.LastCustomerMessageAt = last
		h := s.now().Sub(*last).Hours()
		res.Contact.HoursSinceCustomer = &h
	}

	switch in.Mode {
	case "attendant":
		on, err := s.attendOn(ctx)
		if err != nil {
			return res, err
		}
		if !on {
			block(refuse("attend_off", "O atendente autônomo está desligado pelo administrador."))
		}
		var optOut, group bool
		if err := s.db.QueryRow(ctx, `SELECT automation_opt_out_at IS NOT NULL, kind = 'group' FROM contacts WHERE id = $1`, in.ContactID).Scan(&optOut, &group); err != nil {
			block(refuse("contact", "Contato não encontrado."))
			break
		}
		if group {
			block(refuse("group", "A IA não envia mensagens para grupos."))
		}
		if optOut {
			block(refuse("opt_out", "Este cliente pediu para não receber mensagens automáticas (PARAR)."))
		}
		recent, today, err := s.attendRates(ctx)
		if err != nil {
			return res, err
		}
		res.Limits = map[string]any{
			"last10min": map[string]int{"used": recent, "max": attendPer10Min},
			"last24h":   map[string]int{"used": today, "max": attendPerDay},
		}
		if recent >= attendPer10Min || today >= attendPerDay {
			block(refuse("rate", "Limite de velocidade atingido (%d mensagens em 10 minutos, %d por dia). Tente de novo em alguns minutos.", attendPer10Min, attendPerDay))
		}
	case "automatic":
		on, err := s.masterOn(ctx)
		if err != nil {
			return res, err
		}
		if !on {
			block(refuse("automation_off", "Os envios automáticos estão desligados pelo administrador."))
		}
		cat, err := s.category(ctx, in.Category)
		if err != nil {
			block(refuse("category_off", "Categoria desconhecida %q. Use automation_status para ver as liberadas.", in.Category))
			break
		}
		if !cat.Enabled {
			block(refuse("category_off", "A categoria %q não está liberada pelo administrador.", cat.Key))
		}
		if _, ref, err := s.guardContact(ctx, in.ContactID, st); err != nil {
			return res, err
		} else if ref != nil {
			block(ref)
		}
		if ref, err := s.clientLimits(ctx, s.db, in.ContactID, cat, st, ""); err != nil {
			return res, err
		} else if ref != nil {
			block(ref)
		}
		res.Limits = map[string]any{"dialogHours": st.DialogHours, "clientDailyMax": st.ClientDaily, "minIntervalDays": cat.MinIntervalDays}
		now := s.now()
		open := s.inWindow(cat, st, now)
		w := &struct {
			OpenNow      bool       `json:"openNow"`
			NextOpensAt  *time.Time `json:"nextOpensAt,omitempty"`
			Timezone     string     `json:"timezone"`
			CategoryName string     `json:"category"`
		}{OpenNow: open, Timezone: st.Timezone, CategoryName: cat.Label}
		if !open {
			// Passos de 15 minutos até achar uma janela aberta; depois volta minuto a minuto até o instante exato em que ela abre.
			for t := now.Truncate(time.Minute); t.Before(now.Add(8 * 24 * time.Hour)); t = t.Add(15 * time.Minute) {
				if !s.inWindow(cat, st, t) {
					continue
				}
				from := t.Add(-14 * time.Minute)
				if from.Before(now) {
					from = now.Truncate(time.Minute).Add(time.Minute)
				}
				open := t
				for u := from; u.Before(t); u = u.Add(time.Minute) {
					if s.inWindow(cat, st, u) {
						open = u
						break
					}
				}
				w.NextOpensAt = &open
				break
			}
			res.Notes = append(res.Notes, fmt.Sprintf("Fora do horário de %q agora: o envio não é recusado, mas espera na fila até a janela abrir.", cat.Label))
		}
		res.Window = w
	default:
		return res, refuse("mode", "mode deve ser automatic ou attendant.")
	}
	res.WouldSend = len(res.Blockers) == 0
	return res, nil
}
