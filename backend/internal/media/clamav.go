package media

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"
)

// Scanner fala o protocolo INSTREAM do clamd (TCP). Falha fechada: se o antivírus não
// responde, o arquivo não é aceito.
type Scanner struct{ Addr string }

func NewScanner(addr string) *Scanner { return &Scanner{Addr: addr} }

func (s *Scanner) Scan(ctx context.Context, data []byte) error {
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("%w: antivírus inacessível", ErrUnavailable)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(60 * time.Second))
	if _, err := conn.Write([]byte("zINSTREAM\x00")); err != nil {
		return fmt.Errorf("%w: antivírus", ErrUnavailable)
	}
	const chunk = 64 << 10
	var hdr [4]byte
	for off := 0; off < len(data); off += chunk {
		end := min(off+chunk, len(data))
		binary.BigEndian.PutUint32(hdr[:], uint32(end-off))
		if _, err := conn.Write(hdr[:]); err != nil {
			return fmt.Errorf("%w: antivírus", ErrUnavailable)
		}
		if _, err := conn.Write(data[off:end]); err != nil {
			return fmt.Errorf("%w: antivírus", ErrUnavailable)
		}
	}
	binary.BigEndian.PutUint32(hdr[:], 0)
	if _, err := conn.Write(hdr[:]); err != nil {
		return fmt.Errorf("%w: antivírus", ErrUnavailable)
	}
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil && n == 0 {
		return fmt.Errorf("%w: antivírus", ErrUnavailable)
	}
	resp := strings.TrimRight(string(buf[:n]), "\x00\n ")
	switch {
	case strings.HasSuffix(resp, "OK"):
		return nil
	case strings.HasSuffix(resp, "FOUND"):
		return reject("O arquivo foi bloqueado pelo antivírus.")
	default:
		return fmt.Errorf("%w: resposta do antivírus: %s", ErrUnavailable, resp)
	}
}
