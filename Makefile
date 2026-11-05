.DEFAULT_GOAL := help
SHELL := /bin/bash

-include .env
export

.PHONY: help setup up down logs api web migrate-up migrate-down migrate-status migrate-new test lint fmt build

help: ## Lista os comandos
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n",$$1,$$2}'

setup: ## Instala dependências e cria .env
	@test -f .env || cp .env.example .env
	pnpm install
	cd backend && go mod download

up: ## Sobe Postgres e MinIO
	docker compose up -d

down: ## Derruba os serviços
	docker compose down

logs: ## Logs dos serviços
	docker compose logs -f

api: ## Roda a API Go localmente
	cd backend && go run ./cmd/api

web: ## Roda o frontend (Vite)
	pnpm dev:web

migrate-up: ## Aplica migrações
	cd backend && go run ./cmd/migrate up

migrate-down: ## Reverte a última migração
	cd backend && go run ./cmd/migrate down

migrate-status: ## Status das migrações
	cd backend && go run ./cmd/migrate status

migrate-new: ## Cria migração: make migrate-new name=create_users
	cd backend && go run ./cmd/migrate create $(name)

test: ## Testes (Go + web)
	cd backend && go test -race ./...
	pnpm test

lint: ## Lint e typecheck
	cd backend && go vet ./...
	pnpm lint
	pnpm typecheck

fmt: ## Formata código Go
	cd backend && gofmt -w .

build: ## Build de produção
	cd backend && go build -o bin/ ./cmd/...
	pnpm build
