// Command admin executa tarefas administrativas fora da interface web.
//
//	go run ./cmd/admin create-admin -email admin@escritorio.com.br -name "Fulana"
//	go run ./cmd/admin reset-password -email atendente@escritorio.com.br
//	go run ./cmd/admin reset-mfa -email admin@escritorio.com.br
//
// A senha é lida do terminal sem eco (ou de stdin, uma linha, quando não interativo).
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/vyctinho77/chathm/backend/internal/app"
	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/config"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
	"github.com/vyctinho77/chathm/backend/internal/users"
	"github.com/vyctinho77/chathm/backend/migrations"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("uso: admin [create-admin|reset-password|reset-mfa] -email ...")
	}
	cmd := args[0]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	email := fs.String("email", "", "e-mail do usuário")
	name := fs.String("name", "", "nome (create-admin)")
	_ = fs.Parse(args[1:])
	if *email == "" {
		return errors.New("-email é obrigatório")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	if err := migrations.Up(ctx, cfg.DatabaseURL); err != nil {
		return err
	}
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	svcs, err := app.NewServices(cfg, pool, auth.NewHasher())
	if err != nil {
		return err
	}

	switch cmd {
	case "create-admin":
		if *name == "" {
			return errors.New("-name é obrigatório")
		}
		pw, err := readPassword()
		if err != nil {
			return err
		}
		u, err := svcs.Users.Create(ctx, "", "", users.CreateInput{
			Email: *email, Name: *name, Password: pw, Role: auth.RoleAdmin,
		})
		if err != nil {
			return describe(err)
		}
		fmt.Printf("Administrador criado: %s (%s)\n", u.Email, u.ID)
		if cfg.RequireAdminMFA {
			fmt.Println("No primeiro login será exigida a configuração da verificação em duas etapas.")
		}
	case "reset-password":
		id, err := svcs.Users.FindIDByEmail(ctx, *email)
		if err != nil {
			return err
		}
		pw, err := readPassword()
		if err != nil {
			return err
		}
		if err := svcs.Users.ResetPassword(ctx, "", "", id, pw); err != nil {
			return describe(err)
		}
		fmt.Println("Senha redefinida; sessões do usuário encerradas.")
	case "reset-mfa":
		id, err := svcs.Users.FindIDByEmail(ctx, *email)
		if err != nil {
			return err
		}
		if err := auth.ResetMFA(ctx, pool, id); err != nil {
			return err
		}
		fmt.Println("MFA desativado; sessões encerradas. O usuário deverá configurá-lo novamente.")
	default:
		return fmt.Errorf("comando desconhecido: %s", cmd)
	}
	return nil
}

func readPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", errors.New("informe a senha via stdin")
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	fmt.Fprint(os.Stderr, "Senha: ")
	a, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	fmt.Fprint(os.Stderr, "Confirme a senha: ")
	b, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	if string(a) != string(b) {
		return "", errors.New("as senhas não conferem")
	}
	return string(a), nil
}

func describe(err error) error {
	var fe httpx.FieldErrors
	if errors.As(err, &fe) {
		var parts []string
		for k, v := range fe {
			parts = append(parts, k+": "+v)
		}
		return errors.New(strings.Join(parts, "; "))
	}
	return err
}
