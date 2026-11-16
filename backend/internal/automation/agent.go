package automation

import "context"

// agentName: o nome do agente que pediu o envio (o nome do token), para a mensagem dizer QUAL agente falou. Vazio se o token não existe mais.
func (s *Service) agentName(ctx context.Context, tokenID string) string {
	if tokenID == "" {
		return ""
	}
	var name string
	if err := s.db.QueryRow(ctx, `SELECT name FROM api_tokens WHERE id = $1`, tokenID).Scan(&name); err != nil {
		return ""
	}
	return name
}
