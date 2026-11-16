package aiaccess

import (
	"context"

	"github.com/vyctinho77/chathm/backend/internal/conversations"
)

// addTranscripts devolve às mensagens de voz a transcrição (a equipe não a vê, só os agentes de IA): o serviço de conversas a tira das
// mensagens que mostra às telas. As mensagens já foram obtidas pelo serviço, então a visibilidade já está conferida.
func (m *MCP) addTranscripts(ctx context.Context, msgs []conversations.Message) {
	var ids []string
	for _, x := range msgs {
		if str(x["kind"]) == "audio" && str(x["direction"]) == "in" && x["deleted"] != true {
			ids = append(ids, str(x["id"]))
		}
	}
	ts, err := m.convs.Transcripts(ctx, ids)
	if err != nil || len(ts) == 0 {
		return
	}
	for _, x := range msgs {
		if t, ok := ts[str(x["id"])]; ok {
			x["transcriptState"] = t.State
			if t.Text != "" {
				x["transcript"] = t.Text
			}
		}
	}
}
