package transcribe

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClean(t *testing.T) {
	in := " [BLANK_AUDIO]\n Olá, bom dia.  \n\n(música) Preciso do   boleto.\n[Silêncio]"
	if got := clean(in); got != "Olá, bom dia. Preciso do boleto." {
		t.Fatalf("%q", got)
	}
	if clean("[BLANK_AUDIO]\n") != "" {
		t.Fatal("só silêncio vira vazio")
	}
}

func TestNewIsOffWithoutBinaryOrModel(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "model.bin")
	_ = os.WriteFile(model, []byte("x"), 0o600)
	if New(Config{}) != nil || New(Config{Bin: "/nao/existe/whisper-cli", Model: model}) != nil || New(Config{Bin: "sh", Model: filepath.Join(dir, "nao.bin")}) != nil {
		t.Fatal("sem programa ou sem modelo o recurso deve ficar desligado")
	}
	if New(Config{Bin: "sh", Model: model}) == nil {
		t.Fatal("com programa e modelo deve ligar")
	}
}

// Um whisper-cli de mentira (script) confere o caminho completo: ffmpeg de verdade converte, o "whisper" recebe WAV 16 kHz mono e imprime o texto.
func TestTranscribeWithFakeWhisper(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg não instalado")
	}
	dir := t.TempDir()
	model := filepath.Join(dir, "model.bin")
	_ = os.WriteFile(model, []byte("x"), 0o600)
	args := filepath.Join(dir, "args.txt")
	fake := filepath.Join(dir, "whisper-cli")
	// Anota os argumentos, confere que o arquivo é um WAV ("RIFF") e imprime duas linhas, como o whisper faz.
	script := "#!/bin/sh\necho \"$@\" > " + args + "\nwhile [ $# -gt 0 ]; do [ \"$1\" = -f ] && f=\"$2\"; shift; done\nhead -c 4 \"$f\" | grep -q RIFF || exit 3\nprintf ' [BLANK_AUDIO]\\n Já paguei o boleto,\\n  obrigado.\\n'\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	// 1 s de tom, em WAV (qualquer formato que o ffmpeg leia serve de áudio de entrada).
	src := filepath.Join(dir, "tone.wav")
	if out, err := exec.Command("ffmpeg", "-nostdin", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", src).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	audio, _ := os.ReadFile(src)

	s := New(Config{Bin: fake, Model: model, Lang: "pt", Threads: 3, MaxSeconds: 30})
	if s == nil {
		t.Fatal("deveria ligar")
	}
	got, err := s.Transcribe(context.Background(), audio, 1)
	if err != nil || got != "Já paguei o boleto, obrigado." {
		t.Fatalf("%q %v", got, err)
	}
	b, _ := os.ReadFile(args)
	for _, want := range []string{"-m " + model, "-l pt", "-t 3", "-nt", "-np"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("argumentos %q sem %q", b, want)
		}
	}
	if _, err := s.Transcribe(context.Background(), audio, 31); err != ErrTooLong {
		t.Fatalf("áudio longo demais: %v", err)
	}
	// Lixo que o ffmpeg não entende dá erro (não trava nem devolve texto).
	if _, err := s.Transcribe(context.Background(), []byte("isto não é áudio"), 1); err == nil {
		t.Fatal("áudio inválido deveria falhar")
	}
	// Cancelado enquanto espera a vez: devolve o erro do contexto.
	s.sem <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := s.Transcribe(ctx, audio, 1); err == nil {
		t.Fatal("deveria desistir ao cancelar")
	}
	<-s.sem
}
