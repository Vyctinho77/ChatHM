package conversations

import "context"

// Refresh avisa as telas que a conversa mudou (etiqueta, fila, nota) por uma via que não passa pelo serviço de conversas, como a passagem
// da IA para um humano.
func (s *Service) Refresh(ctx context.Context, convID string) { s.publishState(ctx, convID, nil) }
