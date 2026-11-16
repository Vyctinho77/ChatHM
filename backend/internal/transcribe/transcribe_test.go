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

func TestWaitsForFreeMemoryAndGivesUpIfItNeverComes(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "model.bin")
	_ = os.WriteFile(model, []byte("x"), 0o600)
	s := New(Config{Bin: "sh", Model: model, MinFreeMB: 1000})
	s.memPoll, s.memMax = 5*time.Millisecond, 60*time.Millisecond

	// Memória volta antes do prazo: segue.
	calls := 0
	s.memFree = func() (int, bool) { calls++; return map[bool]int{true: 2000, false: 300}[calls >= 3], true }
	if err := s.waitForMemory(context.Background()); err != nil || calls < 3 {
		t.Fatalf("deveria esperar a memória voltar: %v (%d consultas)", err, calls)
	}
	// Nunca volta: desiste com o erro certo, sem travar.
	s.memFree = func() (int, bool) { return 100, true }
	if err := s.waitForMemory(context.Background()); err != ErrLowMemory {
		t.Fatalf("esperava ErrLowMemory: %v", err)
	}
	// Não saber a memória (fora do Linux) ou desligar a proteção não bloqueia.
	s.memFree = func() (int, bool) { return 0, false }
	if err := s.waitForMemory(context.Background()); err != nil {
		t.Fatalf("sem leitura não bloqueia: %v", err)
	}
	s.memFree = func() (int, bool) { return 1, true }
	s.cfg.MinFreeMB = -1
	if err := s.waitForMemory(context.Background()); err != nil {
		t.Fatalf("proteção desligada não bloqueia: %v", err)
	}
	// Cancelar enquanto espera devolve o erro do contexto.
	s.cfg.MinFreeMB = 1000
	s.memMax = time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := s.waitForMemory(ctx); err == nil || err == ErrLowMemory {
		t.Fatalf("deveria devolver o erro do contexto: %v", err)
	}
}

func TestAvailableMBReadsThisMachine(t *testing.T) {
	if _, err := os.Stat("/proc/meminfo"); err != nil {
		t.Skip("sem /proc/meminfo")
	}
	if mb, ok := availableMB(); !ok || mb <= 0 {
		t.Fatalf("deveria ler a memória disponível: %d %v", mb, ok)
	}
}
