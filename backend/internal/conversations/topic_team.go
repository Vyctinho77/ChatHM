package conversations

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/segments"
)

// callTopicTeam: conversa JÁ em andamento (alguém atende ou ela já está numa fila) em que o cliente passa a falar do assunto de
// outra equipe ("nota fiscal", "folha de pagamento"). A conversa não troca de dono nem de fila ("nada é redistribuído por palavras
// soltas"), mas a equipe do assunto entra como participante e passa a ver a conversa em "Minhas".
//
// Regras: só mensagem do cliente em conversa 1:1 aberta; só quando UMA equipe vence a classificação; o segmento precisa estar ativo, ter
// equipe e permitir a chamada automática (autoInclude); cada assunto chama a equipe no máximo uma vez por conversa (quem tirar a equipe
// à mão não a vê voltar). O responsável atual nunca vira participante.
func (s *Service) callTopicTeam(ctx context.Context, tx pgx.Tx, convID, text string, curSeg, assignee *string, before *userSet) error {
	kws, err := segments.LoadKeywords(ctx, tx)
	if err != nil {
		return err
	}
	res := segments.Classify(text, kws)
	if res.Reason != segments.ReasonKeyword || (curSeg != nil && *curSeg == res.SegmentID) {
		return nil
	}
	var teamID *string
	var auto bool
	var segName string
	err = tx.QueryRow(ctx, `SELECT team_id::text, auto_include, name FROM segments WHERE id = $1 AND is_active`, res.SegmentID).Scan(&teamID, &auto, &segName)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if teamID == nil || !auto {
		return nil
	}
	tag, err := tx.Exec(ctx, `INSERT INTO conversation_segment_calls (conversation_id, segment_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, convID, res.SegmentID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil // esta equipe já foi chamada para esta conversa
	}
	rows, err := tx.Query(ctx, `
		SELECT tm.user_id::text FROM team_members tm
		  JOIN users u ON u.id = tm.user_id AND u.is_active
		 WHERE tm.team_id = $1
		   AND ($2::uuid IS NULL OR tm.user_id <> $2::uuid)
		   AND NOT EXISTS (SELECT 1 FROM conversation_participants cp WHERE cp.conversation_id = $3 AND cp.user_id = tm.user_id)`, *teamID, assignee, convID)
	if err != nil {
		return err
	}
	var users []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		users = append(users, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(users) == 0 {
		return nil
	}
	if *before == nil { // quem enxerga a conversa agora, para avisar só quem passa a enxergar
		if *before, err = s.visibleUsers(ctx, tx, convID); err != nil {
			return err
		}
	}
	for _, id := range users {
		if _, err := tx.Exec(ctx, `INSERT INTO conversation_participants (conversation_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, convID, id); err != nil {
			return err
		}
	}
	phrase := ""
	if len(res.Matches) > 0 {
		phrase = res.Matches[0].Phrase
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "conversation.team_called", TargetType: "conversation", TargetID: convID,
		Metadata: map[string]any{"segment": segName, "segmentId": res.SegmentID, "phrase": phrase, "people": len(users)}})
}
