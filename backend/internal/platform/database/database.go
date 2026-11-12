// Package database encapsula a conexão com o PostgreSQL.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// O pooler "Session" do Supabase aceita no máximo 15 conexões ao mesmo tempo (pool_size) para todo o projeto. Passar disso faz
// as conexões extras serem recusadas (EMAXCONNSESSION) ou ficarem presas, e as requisições esperam dezenas de segundos antes
// de falhar. O padrão fica abaixo do limite, com folga para a troca de versão (o container novo sobe enquanto o antigo fecha),
// migrações e ferramentas de administração.
const (
	DefaultMaxConns = 10
	DefaultMinConns = 2
)

type Options struct {
	MaxConns int32 // 0 = DefaultMaxConns
	MinConns int32 // 0 = DefaultMinConns (nunca acima de MaxConns)
}

func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	return ConnectWith(ctx, url, Options{})
}

func ConnectWith(ctx context.Context, url string, o Options) (*pgxpool.Pool, error) {
	cfg, err := poolConfig(url, o)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("conectando ao banco: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping no banco: %w", err)
	}
	return pool, nil
}

// poolConfig monta a configuração do pool (sem conectar), com os limites seguros.
func poolConfig(url string, o Options) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("DATABASE_URL inválido: %w", err)
	}
	cfg.MaxConns = o.MaxConns
	if cfg.MaxConns <= 0 {
		cfg.MaxConns = DefaultMaxConns
	}
	cfg.MaxConnIdleTime = 5 * time.Minute
	// Abrir conexão (TLS + autenticação) com um banco remoto leva segundos: mantém
	// algumas sempre prontas para o primeiro envio depois de uma pausa não pagar isso.
	cfg.MinConns = o.MinConns
	if cfg.MinConns <= 0 {
		cfg.MinConns = DefaultMinConns
	}
	cfg.MinConns = min(cfg.MinConns, cfg.MaxConns)
	// Uma conexão que não abre (pooler cheio, rede) falha em segundos, em vez de segurar a requisição.
	cfg.ConnConfig.ConnectTimeout = 10 * time.Second
	return cfg, nil
}
