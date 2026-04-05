# Sentinel API

API REST em Go para gerenciamento de usuários SSH e V2Ray/Xray em servidores Linux, com monitoramento em tempo real, cronjobs automáticos e recursos do sistema.

## Índice

- [Requisitos](#requisitos)
- [Instalação](#instalação)
- [Configuração](#configuração)
- [Autenticação](#autenticação)
- [Rate Limiting](#rate-limiting)
- [Referência da API](#referência-da-api)
  - [Health Check](#health-check)
  - [Monitoramento](#monitoramento)
  - [SSH Users](#ssh-users)
  - [V2Ray Users](#v2ray-users)
- [Schemas](#schemas)
- [Códigos de Erro](#códigos-de-erro)
- [Exemplos com cURL](#exemplos-com-curl)
- [Build](#build)
- [Segurança](#segurança)

---

## Requisitos

| Requisito | Versão |
|-----------|--------|
| Go | 1.21+ |
| OS (deploy) | Linux (amd64 ou arm64) |
| V2Ray/Xray | Qualquer versão com config JSON |

## Instalação

```bash
git clone https://github.com/TelksBr/Sentinel-Api.git
cd Sentinel-Api
go mod tidy
```

## Configuração

### Variável de Ambiente Obrigatória

```bash
export API_SENTINEL_KEY="sua-chave-secreta"
```

### Iniciar Servidor

```bash
# Porta padrão (8080)
./api-v2

# Porta customizada via flag
./api-v2 -port 3000

# Porta customizada via argumento posicional (retrocompatível)
./api-v2 3000

# Com TLS
./api-v2 -port 443 -tls-cert /path/cert.pem -tls-key /path/key.pem
```

### Flags Disponíveis

| Flag | Tipo | Padrão | Descrição |
|------|------|--------|-----------|
| `-port` | int | `8080` | Porta do servidor HTTP |
| `-tls-cert` | string | `""` | Caminho para certificado TLS |
| `-tls-key` | string | `""` | Caminho para chave privada TLS |

---

## Autenticação

Todas as rotas protegidas exigem o header `Authorization` com token Bearer:

```
Authorization: Bearer <API_SENTINEL_KEY>
```

**Rotas públicas** (sem autenticação):
- `GET /` — Health check
- `GET /onlines` — Contadores de usuários online
- `GET /system/resources` — Recursos do sistema (CPU/RAM)

## Rate Limiting

Todas as rotas possuem rate limiting global de **60 requisições por minuto por IP**.

Quando excedido, retorna `429 Too Many Requests`:

```json
{
  "error": true,
  "message": "Rate limit excedido. Tente novamente mais tarde."
}
```

---

## Referência da API

**Base URL:** `http://localhost:8080`

### Health Check

#### `GET /`

Verifica se a API está ativa.

**Autenticação:** Não

**Resposta `200`:**
```json
{
  "message": "🟢 API running !"
}
```

---

### Monitoramento

#### `GET /onlines`

Retorna contadores de usuários SSH, V2Ray e DT-Proto online. Dados em cache com polling em background (SSH: 2min, V2Ray: 90s, DT-Proto: 1min).

**Autenticação:** Não

**Resposta `200`:**
```json
{
  "ssh_users": 5,
  "v2ray_users": 12,
  "dt_proto_users": 8,
  "total_users": 25
}
```

---

#### `GET /users/online`

Retorna lista detalhada de usuários online com identificadores.

**Autenticação:** Sim

**Resposta `200`:**
```json
{
  "ssh_users": [
    { "username": "usuario1" }
  ],
  "v2ray_users": [
    {
      "email": "v2ray_550e8400@gmail.com",
      "uuid": "550e8400-e29b-41d4-a716-446655440000",
      "last_connection": "2025-01-15T10:30:00Z"
    }
  ],
  "dt_proto_users": [
    { "id": "user123" },
    { "id": "user456" }
  ],
  "total_ssh": 1,
  "total_v2ray": 1,
  "total_dt_proto": 2,
  "total_users": 4
}
```

---

#### `GET /system/resources`

Retorna uso de CPU e memória RAM do servidor. Lê diretamente de `/proc/meminfo` e `/proc/stat`.

**Autenticação:** Não

**Resposta `200`:**
```json
{
  "memory": {
    "total": 16777216,
    "available": 8388608,
    "used": 8388608,
    "free": 4194304,
    "usage_percent": 50.0
  },
  "cpu": {
    "usage_percent": 15.5,
    "user": 1234,
    "nice": 56,
    "system": 789,
    "idle": 45678,
    "iowait": 123,
    "irq": 45,
    "softirq": 67,
    "steal": 0
  }
}
```

> Valores de memória em **KB**. CPU calculado com duas leituras separadas para precisão.

---

### SSH Users

#### `POST /ssh_user`

Cria um ou mais usuários SSH no sistema Linux.

**Autenticação:** Sim

**Request Body** — `application/json` — Array de objetos:

| Campo | Tipo | Obrigatório | Validação | Descrição |
|-------|------|-------------|-----------|-----------|
| `username` | string | Sim | 3-32 chars, alfanumérico | Nome do usuário |
| `password` | string | Sim | min 4 chars | Senha |
| `limit` | int | Não | min 0 | Limite de conexões (0 = ilimitado) |
| `validate` | int | Sim | min 1 | Dias de validade |
| `is_test` | bool | Não | — | Se é usuário de teste |
| `time` | int | Não | min 0 | Horas para remoção automática |

**Request:**
```json
[
  {
    "username": "usuario1",
    "password": "senha123456",
    "limit": 0,
    "validate": 30,
    "is_test": false,
    "time": 0
  }
]
```

**Resposta `200`:**
```json
{
  "error": false,
  "message": "All users created successfully",
  "details": [
    {
      "username": "usuario1",
      "success": true,
      "message": "User created successfully"
    }
  ]
}
```

**Resposta `200` (parcial):**
```json
{
  "error": true,
  "message": "Some users failed to be created",
  "details": [
    { "username": "usuario1", "success": true, "message": "User created successfully" },
    { "username": "root", "success": false, "message": "Reserved username cannot be used: root" }
  ]
}
```

---

#### `POST /ssh_user/test`

Cria um usuário SSH temporário com remoção automática agendada.

**Autenticação:** Sim

**Request Body:**

| Campo | Tipo | Obrigatório | Validação | Descrição |
|-------|------|-------------|-----------|-----------|
| `username` | string | Sim | 3-32 chars, alfanumérico | Nome do usuário |
| `password` | string | Sim | min 4 chars | Senha |
| `time` | int | Sim | min 1 | Horas até remoção automática (máx 72) |

**Request:**
```json
{
  "username": "teste1",
  "password": "senha123456",
  "time": 2
}
```

**Resposta `200`:**
```json
{
  "error": false,
  "message": "User created successfully",
  "details": [
    { "username": "teste1", "success": true, "message": "User created successfully" }
  ]
}
```

---

#### `PUT /ssh_user/:username`

Atualiza senha e/ou validade de um usuário SSH. Aceita ambos os campos na mesma requisição.

**Autenticação:** Sim

**Path params:** `username` — nome do usuário

**Request Body:**

| Campo | Tipo | Obrigatório | Validação | Descrição |
|-------|------|-------------|-----------|-----------|
| `password` | string | Não | min 4 chars | Nova senha |
| `validate` | int | Não | min 1 | Novos dias de validade |

> Envie um ou ambos. Se ambos forem enviados, a API executa as duas operações e retorna array de resultados.

**Request (apenas senha):**
```json
{ "password": "nova_senha123" }
```

**Resposta `200`:**
```json
{ "username": "usuario1", "success": true, "message": "Password updated successfully" }
```

**Request (ambos):**
```json
{ "password": "nova_senha123", "validate": 90 }
```

**Resposta `200`:**
```json
[
  { "username": "usuario1", "success": true, "message": "Password updated successfully" },
  { "username": "usuario1", "success": true, "message": "Expiration date updated successfully" }
]
```

---

#### `PUT /ssh_user/disable/:username`

Desabilita um usuário SSH (bloqueia login, define shell como nologin).

**Autenticação:** Sim

**Resposta `200`:**
```json
{ "username": "usuario1", "success": true, "message": "User disabled successfully" }
```

---

#### `PUT /ssh_user/enable/:username`

Reabilita um usuário SSH desabilitado.

**Autenticação:** Sim

**Request Body (opcional):**

| Campo | Tipo | Obrigatório | Validação | Descrição |
|-------|------|-------------|-----------|-----------|
| `days` | int | Não | min 1 | Dias de validade ao reabilitar |

**Resposta `200`:**
```json
{ "username": "usuario1", "success": true, "message": "User enabled successfully" }
```

---

#### `POST /ssh_user/delete`

Deleta um ou mais usuários SSH (mata processos, remove do sistema).

**Autenticação:** Sim

**Request Body** — Array de strings (usernames):
```json
["usuario1", "usuario2"]
```

**Resposta `200`:**
```json
{
  "error": false,
  "message": "All users deleted successfully",
  "details": [
    { "username": "usuario1", "success": true, "message": "User deleted successfully" },
    { "username": "usuario2", "success": true, "message": "User deleted successfully" }
  ]
}
```

---

#### `POST /ssh_user/delete_all`

Deleta todos os usuários SSH (exceto usuários de sistema reservados).

**Autenticação:** Sim

**Request Body:** Nenhum

**Resposta `200`:**
```json
{
  "error": false,
  "message": "All users deleted successfully",
  "details": [...],
  "total_before": 10,
  "total_deleted": 10,
  "total_after": 0
}
```

---

### V2Ray Users

#### `POST /v2ray_user`

Cria um ou mais usuários V2Ray/Xray. Insere o client em todos os inbounds da config.

**Autenticação:** Sim

**Request Body** — Array de objetos:

| Campo | Tipo | Obrigatório | Validação | Descrição |
|-------|------|-------------|-----------|-----------|
| `uuid` | string | Sim | UUID v4 | UUID único do usuário |
| `expiration_date` | string | Sim | ISO 8601 / RFC 3339 | Data de expiração |

**Request:**
```json
[
  {
    "uuid": "550e8400-e29b-41d4-a716-446655440000",
    "expiration_date": "2026-12-31T23:59:59Z"
  }
]
```

**Resposta `200`:**
```json
{
  "error": false,
  "message": "Usuarios criados com sucesso",
  "users": [
    {
      "uuid": "550e8400-e29b-41d4-a716-446655440000",
      "email": "v2ray_550e8400@gmail.com",
      "expiration_date": "2026-12-31T23:59:59Z"
    }
  ]
}
```

---

#### `POST /v2ray/test`

Cria usuário V2Ray de teste com remoção automática agendada.

**Autenticação:** Sim

**Request Body:** Mesmo formato de `POST /v2ray_user` (array).

---

#### `PUT /v2ray_user/:uuid`

Atualiza a validade de um usuário V2Ray.

**Autenticação:** Sim

**Path params:** `uuid` — UUID v4 do usuário

**Request Body:**

| Campo | Tipo | Obrigatório | Validação | Descrição |
|-------|------|-------------|-----------|-----------|
| `validate` | int | Sim | min 1 | Novos dias de validade |

**Request:**
```json
{ "validate": 60 }
```

**Resposta `200`:**
```json
{
  "uuid": "550e8400-e29b-41d4-a716-446655440000",
  "email": "v2ray_550e8400@gmail.com",
  "expiration_date": "2026-12-31T23:59:59Z",
  "success": true,
  "message": "Validade atualizada com sucesso"
}
```

---

#### `PUT /v2ray_user/disable/:uuid`

Desabilita um usuário V2Ray (remove de todos os inbounds).

**Autenticação:** Sim

**Resposta `200`:**
```json
{
  "uuid": "550e8400-e29b-41d4-a716-446655440000",
  "success": true,
  "message": "Usuário desabilitado com sucesso"
}
```

---

#### `PUT /v2ray_user/enable/:uuid`

Reabilita um usuário V2Ray.

**Autenticação:** Sim

**Request Body (opcional):**

| Campo | Tipo | Obrigatório | Validação | Descrição |
|-------|------|-------------|-----------|-----------|
| `expiration_date` | string | Não | ISO 8601 | Nova data de expiração |

---

#### `POST /v2ray_user/delete`

Deleta um ou mais usuários V2Ray.

**Autenticação:** Sim

**Request Body:**
```json
{
  "uuids": ["550e8400-e29b-41d4-a716-446655440000"]
}
```

**Resposta `200`:**
```json
{
  "error": false,
  "message": "Usuarios deletados com sucesso",
  "users": [
    {
      "uuid": "550e8400-e29b-41d4-a716-446655440000",
      "email": "v2ray_550e8400@gmail.com",
      "expiration_date": "2026-12-31T23:59:59Z"
    }
  ]
}
```

---

#### `POST /v2ray_user/delete_all`

Deleta todos os usuários V2Ray.

**Autenticação:** Sim

**Request Body:** Nenhum

---

## Schemas

### OnlineUsersResponse

```json
{
  "ssh_users": "int",
  "v2ray_users": "int",
  "dt_proto_users": "int",
  "total_users": "int"
}
```

### DTProtoUserOnline

```json
{
  "id": "string"
}
```

### DetailedUsersResponse

```json
{
  "ssh_users": ["SSHUserOnline"],
  "v2ray_users": ["V2RayUserOnline"],
  "dt_proto_users": ["DTProtoUserOnline"],
  "total_ssh": "int",
  "total_v2ray": "int",
  "total_dt_proto": "int",
  "total_users": "int"
}
```

### SSHUserResponse

```json
{
  "username": "string",
  "success": "boolean",
  "message": "string"
}
```

### SSHUserCreateResponse

```json
{
  "error": "boolean",
  "message": "string",
  "details": ["SSHUserResponse"],
  "total_before": "int (opcional)",
  "total_deleted": "int (opcional)",
  "total_after": "int (opcional)"
}
```

### V2RayUserResponse

```json
{
  "uuid": "string (UUID v4)",
  "email": "string",
  "expiration_date": "string (ISO 8601)",
  "success": "boolean",
  "message": "string"
}
```

### V2RayUserCreateResponse

```json
{
  "error": "boolean",
  "message": "string",
  "users": ["V2RayUserResponse"],
  "total_before": "int (opcional)",
  "total_deleted": "int (opcional)",
  "total_after": "int (opcional)"
}
```

### ErrorResponse

```json
{
  "error": true,
  "message": "string"
}
```

### ValidationErrorResponse

```json
{
  "error": true,
  "message": "string",
  "details": [
    {
      "field": "string",
      "tag": "string",
      "value": "string",
      "message": "string"
    }
  ]
}
```

---

## Códigos de Erro

| HTTP | Cenário | Exemplo de `message` |
|------|---------|----------------------|
| `200` | Sucesso | `All users created successfully` |
| `200` | Sucesso parcial (`error: true`) | `Some users failed to be created` |
| `400` | Validação / dados inválidos | `Dados de usuário inválidos` |
| `400` | Usuário não encontrado | `User not found` |
| `400` | Username reservado | `Reserved username cannot be used: root` |
| `401` | Token ausente | `Token de autorização não fornecido` |
| `401` | Token inválido | `Token de autorização inválido` |
| `429` | Rate limit excedido | `Rate limit excedido. Tente novamente mais tarde.` |

> **Nota:** Operações em lote (create, delete) retornam `200` mesmo com falhas parciais. O campo `error: true` indica que nem todas as operações tiveram sucesso. Verifique `details[].success` individualmente.

---

## Exemplos com cURL

### Criar usuário SSH

```bash
curl -X POST http://localhost:8080/ssh_user \
  -H "Authorization: Bearer SUA_CHAVE" \
  -H "Content-Type: application/json" \
  -d '[{"username":"joao","password":"senha123","limit":0,"validate":30,"is_test":false,"time":0}]'
```

### Atualizar senha e validade

```bash
curl -X PUT http://localhost:8080/ssh_user/joao \
  -H "Authorization: Bearer SUA_CHAVE" \
  -H "Content-Type: application/json" \
  -d '{"password":"nova_senha","validate":60}'
```

### Deletar usuários SSH

```bash
curl -X POST http://localhost:8080/ssh_user/delete \
  -H "Authorization: Bearer SUA_CHAVE" \
  -H "Content-Type: application/json" \
  -d '["joao","maria"]'
```

### Criar usuário V2Ray

```bash
curl -X POST http://localhost:8080/v2ray_user \
  -H "Authorization: Bearer SUA_CHAVE" \
  -H "Content-Type: application/json" \
  -d '[{"uuid":"550e8400-e29b-41d4-a716-446655440000","expiration_date":"2026-12-31T23:59:59Z"}]'
```

### Ver usuários online

```bash
curl http://localhost:8080/onlines
```

### Ver recursos do sistema

```bash
curl http://localhost:8080/system/resources
```

---

## Build

### Desenvolvimento

```bash
go run ./cmd/api
```

### Produção (Linux)

```bash
# x64
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o build/api-v2_x64 ./cmd/api

# ARM64
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o build/api-v2_arm64 ./cmd/api

# Estático (sem dependências)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags="-s -w" -o build/api-v2_static ./cmd/api
```

### Via Makefile

```bash
make build-x64     # Linux x64
make build-arm64   # Linux ARM64
make build-all     # Todas as plataformas
make build-static  # Binário estático
make test          # Testes
```

### Testes

```bash
go test ./...
```

---

## Segurança

- **Autenticação** via Bearer token com comparação constant-time (imune a timing attacks)
- **Rate limiting** global: 60 req/min por IP
- **Sanitização** de usernames com validação POSIX (proteção contra command injection)
- **Nomes reservados** bloqueados (root, admin, sshd, www-data, etc.)
- **Não usa `bash -c`** para executar comandos — usa `exec.Command` direto com argumentos separados
- **Mutex** no acesso a arquivos de cronjobs para evitar race conditions
- **Atomic writes** na limpeza de logs V2Ray (tmp + rename)
- **TLS nativo** disponível via flags de certificado

---

## Estrutura do Projeto

```
cmd/api/main.go              # Entrypoint
internal/
  handlers/                  # Controllers HTTP (Gin)
    ssh_handlers.go          # Handlers SSH
    v2ray_handlers.go        # Handlers V2Ray
    monitor_handlers.go      # Handlers de monitoramento
    error_handlers.go        # Handler de erros de validação
  services/                  # Lógica de negócio
    ssh_service.go           # Gerenciamento de usuários SSH (Linux)
    v2ray_service.go         # Gerenciamento de usuários V2Ray/Xray
    monitor_service.go       # Monitoramento em tempo real
    interfaces.go            # Interfaces dos serviços
  middleware/
    auth.go                  # Autenticação Bearer token
    ratelimit.go             # Rate limiting por IP
  models/                    # Structs e validação
    ssh_user.go              # Modelos SSH
    v2ray_user.go            # Modelos V2Ray
    monitor.go               # Modelos de monitoramento
    response.go              # Respostas padronizadas
    config.go                # Configuração V2Ray
  routes/routes.go           # Definição de rotas
  cron/                      # Cronjobs automáticos
    cronjob_service.go       # Serviço de agendamento
    cronjob_models.go        # Modelos de cronjob
  utils/ssh_utils.go         # Utilitários SSH (comandos Linux)
```

## Cronjobs Automáticos

| Job | Intervalo | Descrição |
|-----|-----------|-----------|
| Remoção de testes SSH | 5 min | Remove usuários teste expirados |
| Remoção de V2Ray expirados | 1 hora | Remove usuários V2Ray com data expirada |

---

## Licença

MIT
