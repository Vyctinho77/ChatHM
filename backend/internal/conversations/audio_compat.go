package conversations

import (
	"context"
	"strings"

	"github.com/vyctinho77/chathm/backend/internal/media"
)

// AudioM4A devolve o áudio de um anexo em AAC/MP4 (.m4a), para aparelhos que não tocam Ogg/Opus. Só converte áudio; o arquivo
// guardado não muda. O SHA-256 ganha um sufixo para o ETag da versão convertida não se confundir com o do original.
func (s *Service) AudioM4A(ctx context.Context, a AttachmentFile) (AttachmentFile, error) {
	if s.media == nil {
		return AttachmentFile{}, media.ErrUnavailable
	}
	if !strings.HasPrefix(a.MimeType, "audio/") {
		return AttachmentFile{}, media.ErrUnavailable
	}
	key := ""
	if a.SHA256 != "" {
		key = a.SHA256 + "-m4a"
	}
	data, err := s.media.ToAAC(ctx, a.Data, key)
	if err != nil {
		return AttachmentFile{}, err
	}
	return AttachmentFile{Data: data, MimeType: "audio/mp4", FileName: "audio.m4a", SHA256: key}, nil
}
