package media

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
)

func TestCleanFileName(t *testing.T) {
	for in, want := range map[string]string{
		"../../etc/passwd":    "passwd",
		`C:\Users\x\nota.pdf`: "nota.pdf",
		"a\x00b\nc.pdf":       "abc.pdf",
		"":                    "arquivo",
		"..":                  "arquivo",
	} {
		if got := CleanFileName(in); got != want {
			t.Errorf("CleanFileName(%q) = %q, queria %q", in, got, want)
		}
	}
	if got := CleanFileName(strings.Repeat("a", 300) + ".pdf"); len([]rune(got)) > 120 || !strings.HasSuffix(got, ".pdf") {
		t.Errorf("nome longo: %d %q", len(got), got[len(got)-6:])
	}
}

// clamd falso: responde ao INSTREAM com a resposta pedida.
func fakeClamd(t *testing.T, reply string) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				cmd := make([]byte, 10)
				_, _ = io.ReadFull(c, cmd)
				for {
					var n [4]byte
					if _, err := io.ReadFull(c, n[:]); err != nil {
						return
					}
					sz := binary.BigEndian.Uint32(n[:])
					if sz == 0 {
						break
					}
					_, _ = io.CopyN(io.Discard, c, int64(sz))
				}
				_, _ = c.Write([]byte(reply + "\x00"))
			}()
		}
	}()
	return l.Addr().String()
}

func TestScanner(t *testing.T) {
	ctx := context.Background()
	data := make([]byte, 200_000)
	if err := NewScanner(fakeClamd(t, "stream: OK")).Scan(ctx, data); err != nil {
		t.Fatalf("limpo: %v", err)
	}
	var rej *RejectError
	if err := NewScanner(fakeClamd(t, "stream: Eicar-Test-Signature FOUND")).Scan(ctx, data); !errors.As(err, &rej) {
		t.Fatalf("infectado deveria ser recusado: %v", err)
	}
	// Falha fechada: antivírus fora do ar não deixa passar.
	if err := NewScanner("127.0.0.1:1").Scan(ctx, data); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("antivírus inacessível: %v", err)
	}
}

func TestProcessRunsScanner(t *testing.T) {
	s := NewService(1<<20, NewScanner(fakeClamd(t, "stream: Win.Test.EICAR FOUND")))
	var rej *RejectError
	if _, err := s.Process(context.Background(), Document, []byte("%PDF-1.4"), "a.pdf", false); !errors.As(err, &rej) {
		t.Fatalf("esperava bloqueio: %v", err)
	}
	s = NewService(1<<20, NewScanner(fakeClamd(t, "stream: OK")))
	p, err := s.Process(context.Background(), Document, []byte("%PDF-1.4"), "a.pdf", false)
	if err != nil || p.ScanStatus != "clean" {
		t.Fatalf("%v %+v", err, p.ScanStatus)
	}
}
