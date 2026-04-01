# 📚 API V2 - Guia Completo de Rotas

## 🔧 Configuração Inicial

### 1. Definir Variável de Ambiente
```bash
export API_ATLAS_KEY=sua-chave-api-aqui
```

### 2. Iniciar API
```bash
./build/api-v2 8080
```

### 3. Base URL
```
http://localhost:8080
```

### 4. Headers Obrigatórios
```
Authorization: Bearer sua-chave-api-aqui
Content-Type: application/json
```

---

## 🚀 Rotas da API

### 📍 **Health Check**
```http
GET /
```

**Resposta:**
```json
{
  "message": "🟢 API running !"
}
```

### 📊 **Usuários Online (Contadores)**
```http
GET /onlines
```

**Resposta:**
```json
{
  "ssh_users": 5,
  "v2ray_users": 12,
  "total_users": 17
}
```

**Características:**
- ✅ **Cache em memória** - nunca chama função direta
- ✅ **Polling em background** - SSH (2min), V2Ray (90s)
- ✅ **Thread-safe** - acesso concorrente seguro
- ✅ **Zero overhead** - resposta instantânea

### 📋 **Usuários Online (Lista Detalhada)**
```http
GET /users/online
```

**Resposta:**
```json
{
  "ssh_users": [
    {
      "username": "usuario1"
    },
    {
      "username": "usuario2"
    }
  ],
  "v2ray_users": [
    {
      "email": "v2ray_550e8400@gmail.com",
      "uuid": "550e8400-e29b-41d4-a716-446655440000",
      "last_connection": "2024-12-15T10:30:00Z"
    },
    {
      "email": "v2ray_123e4567@yahoo.com",
      "uuid": "123e4567-e89b-12d3-a456-426614174000",
      "last_connection": "2024-12-15T11:15:00Z"
    }
  ],
  "total_ssh": 2,
  "total_v2ray": 2,
  "total_users": 4
}
```

**Características:**
- ✅ **Lista completa** de usuários SSH e V2Ray online
- ✅ **Informações detalhadas** - username, IP, tempo de login
- ✅ **Cache em memória** - nunca chama função direta
- ✅ **Polling em background** - atualizações automáticas
- ✅ **Thread-safe** - acesso concorrente seguro

---

### 💻 **Recursos do Sistema (CPU e RAM)**
```http
GET /system/resources
```

**Resposta:**
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

**Características:**
- ✅ **Informações de memória** - Total, disponível, usado, livre em KB e percentual de uso
- ✅ **Informações de CPU** - Uso percentual e estatísticas detalhadas (user, system, idle, etc)
- ✅ **Leitura em tempo real** - Lê `/proc/meminfo` e `/proc/stat`
- ✅ **Cálculo preciso** - CPU calculado com duas leituras separadas

---

## 👤 **SSH Users**

### 1. **Criar Usuários SSH**
```http
POST /ssh_user
```

**Body:**
```json
[
  {
    "username": "usuario1",
    "password": "senha123456",
    "limit": 0,
    "validate": 30,
    "is_test": false,
    "time": 0
  },
  {
    "username": "usuario2", 
    "password": "outrasenha789",
    "limit": 0,
    "validate": 60,
    "is_test": false,
    "time": 0
  }
]
```

**Resposta de Sucesso:**
```json
{
  "error": false,
  "message": "All users created successfully",
  "details": [
    {
      "username": "usuario1",
      "success": true,
      "message": "User created successfully"
    },
    {
      "username": "usuario2",
      "success": true,
      "message": "User created successfully"
    }
  ]
}
```

**Resposta de Erro:**
```json
{
  "error": true,
  "message": "Some users failed to be created",
  "details": [
    {
      "username": "usuario1",
      "success": true,
      "message": "User created successfully"
    },
    {
      "username": "root",
      "success": false,
      "message": "Reserved username cannot be used: root"
    }
  ]
}
```

### 2. **Criar Usuário de Teste SSH**
```http
POST /ssh_user/test
```

**Body:**
```json
{
  "username": "teste1",
  "password": "senha123456",
  "time": 2
}
```

**Parâmetros:**
- **username**: Nome do usuário (3-32 caracteres, alfanumérico)
- **password**: Senha (mínimo 6 caracteres)
- **time**: Horas até remoção automática (máximo 72 horas)

**Resposta:**
```json
{
  "error": false,
  "message": "User created successfully",
  "details": [
    {
      "username": "teste1",
      "success": true,
      "message": "User created successfully"
    }
  ]
}
```

**Notas:**
- Usuário criado com **3 dias de validade** no sistema
- Removido automaticamente após o tempo especificado em `time` (máximo 72 horas)
- Agendamento realizado via arquivo JSON em `/root/dragoncore/temp/cronjobs.json`

### 3. **Atualizar Usuário SSH**
```http
PUT /ssh_user/{username}
```

#### 3.1. Atualizar Senha
**Body:**
```json
{
  "password": "nova_senha123"
}
```

**Resposta:**
```json
{
  "username": "usuario1",
  "success": true,
  "message": "Password updated successfully"
}
```

#### 3.2. Atualizar Validade
**Body:**
```json
{
  "validate": 90
}
```

**Resposta:**
```json
{
  "username": "usuario1",
  "success": true,
  "message": "Expiration date updated successfully"
}
```

### 4. **Desabilitar Usuário SSH**
```http
PUT /ssh_user/disable/{username}
```

**Exemplo:** `PUT /ssh_user/disable/usuario1`

**Resposta:**
```json
{
  "username": "usuario1",
  "success": true,
  "message": "User disabled successfully"
}
```

### 5. **Habilitar Usuário SSH**
```http
PUT /ssh_user/enable/{username}
```

**Exemplo:** `PUT /ssh_user/enable/usuario1`

**Body (opcional):**
```json
{
  "days": 30
}
```

**Resposta:**
```json
{
  "username": "usuario1",
  "success": true,
  "message": "User enabled successfully"
}
```

### 6. **Deletar Usuários SSH**
```http
POST /ssh_user/delete
```

**Body:**
```json
["usuario1", "usuario2", "teste1"]
```

**Resposta:**
```json
{
  "error": false,
  "message": "All users deleted successfully",
  "details": [
    {
      "username": "usuario1",
      "success": true,
      "message": "User deleted successfully"
    },
    {
      "username": "usuario2",
      "success": true,
      "message": "User deleted successfully"
    },
    {
      "username": "teste1",
      "success": true,
      "message": "User deleted successfully"
    }
  ]
}
```

---

## 🌐 **V2Ray Users**

### 1. **Criar Usuários V2Ray**
```http
POST /v2ray_user
```

**Body:**
```json
[
  {
    "uuid": "550e8400-e29b-41d4-a716-446655440000",
    "expiration_date": "2025-12-31T23:59:59Z"
  },
  {
    "uuid": "550e8400-e29b-41d4-a716-446655440001",
    "expiration_date": "2025-06-30T23:59:59Z"
  }
]
```

**Resposta de Sucesso:**
```json
{
  "error": false,
  "message": "Usuarios criados com sucesso",
  "users": [
    {
      "uuid": "550e8400-e29b-41d4-a716-446655440000",
      "email": "v2ray_550e8400@gmail.com",
      "expiration_date": "2025-12-31T23:59:59Z"
    },
    {
      "uuid": "550e8400-e29b-41d4-a716-446655440001",
      "email": "v2ray_550e8400@yahoo.com",
      "expiration_date": "2025-06-30T23:59:59Z"
    }
  ]
}
```

### 2. **Criar Usuário de Teste V2Ray**
```http
POST /v2ray/test
```

**Body:**
```json
[
  {
    "uuid": "550e8400-e29b-41d4-a716-446655440002",
    "expiration_date": "2024-12-20T23:59:59Z"
  }
]
```

**Resposta:**
```json
{
  "error": false,
  "message": "Usuarios criados com sucesso",
  "users": [
    {
      "uuid": "550e8400-e29b-41d4-a716-446655440002",
      "email": "v2ray_550e8400@outlook.com",
      "expiration_date": "2024-12-20T23:59:59Z"
    }
  ]
}
```

### 3. **Atualizar Validade V2Ray**
```http
PUT /v2ray_user/{uuid}
```

**Exemplo:** `PUT /v2ray_user/550e8400-e29b-41d4-a716-446655440000`

**Body:**
```json
{
  "validate": 60
}
```

**Resposta:**
```json
{
  "uuid": "550e8400-e29b-41d4-a716-446655440000",
  "email": "v2ray_550e8400@gmail.com",
  "expiration_date": "2025-12-31T23:59:59Z",
  "success": true,
  "message": "Validade atualizada com sucesso"
}
```

### 4. **Desabilitar Usuário V2Ray**
```http
PUT /v2ray_user/disable/{uuid}
```

**Exemplo:** `PUT /v2ray_user/disable/550e8400-e29b-41d4-a716-446655440000`

**Resposta:**
```json
{
  "uuid": "550e8400-e29b-41d4-a716-446655440000",
  "email": "v2ray_550e8400@gmail.com",
  "expiration_date": "",
  "success": true,
  "message": "Usuário desabilitado com sucesso"
}
```

### 5. **Habilitar Usuário V2Ray**
```http
PUT /v2ray_user/enable/{uuid}
```

**Exemplo:** `PUT /v2ray_user/enable/550e8400-e29b-41d4-a716-446655440000`

**Body (opcional):**
```json
{
  "expiration_date": "2025-12-31T23:59:59Z"
}
```

**Resposta:**
```json
{
  "uuid": "550e8400-e29b-41d4-a716-446655440000",
  "email": "v2ray_550e8400@gmail.com",
  "expiration_date": "2025-12-31T23:59:59Z",
  "success": true,
  "message": "Usuário habilitado com sucesso"
}
```

### 6. **Deletar Usuários V2Ray**
```http
POST /v2ray_user/delete
```

**Body:**
```json
{
  "uuids": [
    "550e8400-e29b-41d4-a716-446655440000",
    "550e8400-e29b-41d4-a716-446655440001"
  ]
}
```

**Resposta:**
```json
{
  "error": false,
  "message": "Usuarios deletados com sucesso",
  "users": [
    {
      "uuid": "550e8400-e29b-41d4-a716-446655440000",
      "email": "v2ray_550e8400@gmail.com",
      "expiration_date": "2025-12-31T23:59:59Z"
    },
    {
      "uuid": "550e8400-e29b-41d4-a716-446655440001",
      "email": "v2ray_550e8400@yahoo.com",
      "expiration_date": "2025-06-30T23:59:59Z"
    }
  ]
}
```

---

## ❌ **Respostas de Erro**

### 1. **Erro de Autenticação (401)**
```json
{
  "error": true,
  "message": "Token de autorização inválido"
}
```

### 2. **Erro de Validação (400)**
```json
{
  "error": true,
  "message": "Dados de usuário inválidos",
  "details": [
    {
      "field": "username",
      "tag": "required",
      "value": "",
      "message": "Username é obrigatório"
    },
    {
      "field": "password",
      "tag": "min",
      "value": "123",
      "message": "Password deve ter pelo menos 6 caracteres"
    }
  ]
}
```

### 3. **Erro de Usuário Não Encontrado (400)**
```json
{
  "username": "usuario_inexistente",
  "success": false,
  "message": "User not found"
}
```

### 4. **Erro de Username Reservado (400)**
```json
{
  "username": "root",
  "success": false,
  "message": "Reserved username cannot be used: root"
}
```

### 5. **Erro de UUID Inválido (400)**
```json
{
  "error": true,
  "message": "Dados de usuário V2Ray inválidos",
  "details": [
    {
      "field": "uuid",
      "tag": "uuid4",
      "value": "uuid-invalido",
      "message": "UUID deve estar no formato UUID4"
    }
  ]
}
```

### 6. **Erro de Data Inválida (400)**
```json
{
  "error": true,
  "message": "Dados de usuário V2Ray inválidos",
  "details": [
    {
      "field": "expiration_date",
      "tag": "validation",
      "value": "data-invalida",
      "message": "Data de expiração deve estar no formato ISO válido"
    }
  ]
}
```

---

## 📋 **Coleção Postman**

### Importar no Postman
1. Abra o Postman
2. Clique em "Import"
3. Cole o JSON da coleção abaixo:

```json
{
  "info": {
    "name": "API Atlas V2",
    "description": "Coleção completa da API Atlas V2",
    "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"
  },
  "variable": [
    {
      "key": "baseUrl",
      "value": "http://localhost:8080",
      "type": "string"
    },
    {
      "key": "apiKey",
      "value": "sua-chave-api-aqui",
      "type": "string"
    }
  ],
  "item": [
    {
      "name": "Health Check",
      "request": {
        "method": "GET",
        "header": [
          {
            "key": "Authorization",
            "value": "Bearer {{apiKey}}",
            "type": "text"
          }
        ],
        "url": {
          "raw": "{{baseUrl}}/",
          "host": ["{{baseUrl}}"],
          "path": [""]
        }
      }
    },
    {
      "name": "SSH Users",
      "item": [
        {
          "name": "Criar Usuários SSH",
          "request": {
            "method": "POST",
            "header": [
              {
                "key": "Authorization",
                "value": "Bearer {{apiKey}}",
                "type": "text"
              },
              {
                "key": "Content-Type",
                "value": "application/json",
                "type": "text"
              }
            ],
            "body": {
              "mode": "raw",
              "raw": "[\n  {\n    \"username\": \"usuario1\",\n    \"password\": \"senha123456\",\n    \"limit\": 0,\n    \"validate\": 30,\n    \"is_test\": false,\n    \"time\": 0\n  }\n]"
            },
            "url": {
              "raw": "{{baseUrl}}/ssh_user",
              "host": ["{{baseUrl}}"],
              "path": ["ssh_user"]
            }
          }
        },
        {
          "name": "Criar Usuário Teste SSH",
          "request": {
            "method": "POST",
            "header": [
              {
                "key": "Authorization",
                "value": "Bearer {{apiKey}}",
                "type": "text"
              },
              {
                "key": "Content-Type",
                "value": "application/json",
                "type": "text"
              }
            ],
            "body": {
              "mode": "raw",
              "raw": "{\n  \"username\": \"teste1\",\n  \"password\": \"senha123456\",\n  \"time\": 2\n}"
            },
            "url": {
              "raw": "{{baseUrl}}/ssh_user/test",
              "host": ["{{baseUrl}}"],
              "path": ["ssh_user", "test"]
            }
          }
        },
        {
          "name": "Atualizar Senha SSH",
          "request": {
            "method": "PUT",
            "header": [
              {
                "key": "Authorization",
                "value": "Bearer {{apiKey}}",
                "type": "text"
              },
              {
                "key": "Content-Type",
                "value": "application/json",
                "type": "text"
              }
            ],
            "body": {
              "mode": "raw",
              "raw": "{\n  \"password\": \"nova_senha123\"\n}"
            },
            "url": {
              "raw": "{{baseUrl}}/ssh_user/usuario1",
              "host": ["{{baseUrl}}"],
              "path": ["ssh_user", "usuario1"]
            }
          }
        },
        {
          "name": "Atualizar Validade SSH",
          "request": {
            "method": "PUT",
            "header": [
              {
                "key": "Authorization",
                "value": "Bearer {{apiKey}}",
                "type": "text"
              },
              {
                "key": "Content-Type",
                "value": "application/json",
                "type": "text"
              }
            ],
            "body": {
              "mode": "raw",
              "raw": "{\n  \"validate\": 90\n}"
            },
            "url": {
              "raw": "{{baseUrl}}/ssh_user/usuario1",
              "host": ["{{baseUrl}}"],
              "path": ["ssh_user", "usuario1"]
            }
          }
        },
        {
          "name": "Desabilitar Usuário SSH",
          "request": {
            "method": "PUT",
            "header": [
              {
                "key": "Authorization",
                "value": "Bearer {{apiKey}}",
                "type": "text"
              }
            ],
            "url": {
              "raw": "{{baseUrl}}/ssh_user/disable/usuario1",
              "host": ["{{baseUrl}}"],
              "path": ["ssh_user", "disable", "usuario1"]
            }
          }
        },
        {
          "name": "Habilitar Usuário SSH",
          "request": {
            "method": "PUT",
            "header": [
              {
                "key": "Authorization",
                "value": "Bearer {{apiKey}}",
                "type": "text"
              },
              {
                "key": "Content-Type",
                "value": "application/json",
                "type": "text"
              }
            ],
            "body": {
              "mode": "raw",
              "raw": "{\n  \"days\": 30\n}"
            },
            "url": {
              "raw": "{{baseUrl}}/ssh_user/enable/usuario1",
              "host": ["{{baseUrl}}"],
              "path": ["ssh_user", "enable", "usuario1"]
            }
          }
        },
        {
          "name": "Deletar Usuários SSH",
          "request": {
            "method": "POST",
            "header": [
              {
                "key": "Authorization",
                "value": "Bearer {{apiKey}}",
                "type": "text"
              },
              {
                "key": "Content-Type",
                "value": "application/json",
                "type": "text"
              }
            ],
            "body": {
              "mode": "raw",
              "raw": "[\"usuario1\", \"usuario2\"]"
            },
            "url": {
              "raw": "{{baseUrl}}/ssh_user/delete",
              "host": ["{{baseUrl}}"],
              "path": ["ssh_user", "delete"]
            }
          }
        }
      ]
    },
    {
      "name": "V2Ray Users",
      "item": [
        {
          "name": "Criar Usuários V2Ray",
          "request": {
            "method": "POST",
            "header": [
              {
                "key": "Authorization",
                "value": "Bearer {{apiKey}}",
                "type": "text"
              },
              {
                "key": "Content-Type",
                "value": "application/json",
                "type": "text"
              }
            ],
            "body": {
              "mode": "raw",
              "raw": "[\n  {\n    \"uuid\": \"550e8400-e29b-41d4-a716-446655440000\",\n    \"expiration_date\": \"2025-12-31T23:59:59Z\"\n  }\n]"
            },
            "url": {
              "raw": "{{baseUrl}}/v2ray_user",
              "host": ["{{baseUrl}}"],
              "path": ["v2ray_user"]
            }
          }
        },
        {
          "name": "Criar Usuário Teste V2Ray",
          "request": {
            "method": "POST",
            "header": [
              {
                "key": "Authorization",
                "value": "Bearer {{apiKey}}",
                "type": "text"
              },
              {
                "key": "Content-Type",
                "value": "application/json",
                "type": "text"
              }
            ],
            "body": {
              "mode": "raw",
              "raw": "[\n  {\n    \"uuid\": \"550e8400-e29b-41d4-a716-446655440001\",\n    \"expiration_date\": \"2024-12-20T23:59:59Z\"\n  }\n]"
            },
            "url": {
              "raw": "{{baseUrl}}/v2ray/test",
              "host": ["{{baseUrl}}"],
              "path": ["v2ray", "test"]
            }
          }
        },
        {
          "name": "Atualizar Validade V2Ray",
          "request": {
            "method": "PUT",
            "header": [
              {
                "key": "Authorization",
                "value": "Bearer {{apiKey}}",
                "type": "text"
              },
              {
                "key": "Content-Type",
                "value": "application/json",
                "type": "text"
              }
            ],
            "body": {
              "mode": "raw",
              "raw": "{\n  \"validate\": 60\n}"
            },
            "url": {
              "raw": "{{baseUrl}}/v2ray_user/550e8400-e29b-41d4-a716-446655440000",
              "host": ["{{baseUrl}}"],
              "path": ["v2ray_user", "550e8400-e29b-41d4-a716-446655440000"]
            }
          }
        },
        {
          "name": "Desabilitar Usuário V2Ray",
          "request": {
            "method": "PUT",
            "header": [
              {
                "key": "Authorization",
                "value": "Bearer {{apiKey}}",
                "type": "text"
              }
            ],
            "url": {
              "raw": "{{baseUrl}}/v2ray_user/disable/550e8400-e29b-41d4-a716-446655440000",
              "host": ["{{baseUrl}}"],
              "path": ["v2ray_user", "disable", "550e8400-e29b-41d4-a716-446655440000"]
            }
          }
        },
        {
          "name": "Habilitar Usuário V2Ray",
          "request": {
            "method": "PUT",
            "header": [
              {
                "key": "Authorization",
                "value": "Bearer {{apiKey}}",
                "type": "text"
              },
              {
                "key": "Content-Type",
                "value": "application/json",
                "type": "text"
              }
            ],
            "body": {
              "mode": "raw",
              "raw": "{\n  \"expiration_date\": \"2025-12-31T23:59:59Z\"\n}"
            },
            "url": {
              "raw": "{{baseUrl}}/v2ray_user/enable/550e8400-e29b-41d4-a716-446655440000",
              "host": ["{{baseUrl}}"],
              "path": ["v2ray_user", "enable", "550e8400-e29b-41d4-a716-446655440000"]
            }
          }
        },
        {
          "name": "Deletar Usuários V2Ray",
          "request": {
            "method": "POST",
            "header": [
              {
                "key": "Authorization",
                "value": "Bearer {{apiKey}}",
                "type": "text"
              },
              {
                "key": "Content-Type",
                "value": "application/json",
                "type": "text"
              }
            ],
            "body": {
              "mode": "raw",
              "raw": "{\n  \"uuids\": [\n    \"550e8400-e29b-41d4-a716-446655440000\",\n    \"550e8400-e29b-41d4-a716-446655440001\"\n  ]\n}"
            },
            "url": {
              "raw": "{{baseUrl}}/v2ray_user/delete",
              "host": ["{{baseUrl}}"],
              "path": ["v2ray_user", "delete"]
            }
          }
        }
      ]
    }
  ]
}
```

---

## 🧪 **Testando no Postman**

### 1. **Configurar Variáveis**
- `baseUrl`: `http://localhost:8080`
- `apiKey`: `sua-chave-api-aqui`

### 2. **Sequência de Testes Recomendada**
1. **Health Check** - Verificar se API está rodando
2. **Criar Usuários SSH** - Testar criação
3. **Criar Usuário Teste SSH** - Testar usuário temporário
4. **Atualizar Senha SSH** - Testar atualização
5. **Atualizar Validade SSH** - Testar extensão
6. **Desabilitar SSH** - Testar desabilitação
7. **Habilitar SSH** - Testar habilitação
8. **Criar Usuários V2Ray** - Testar criação V2Ray
9. **Criar Usuário Teste V2Ray** - Testar usuário temporário V2Ray
10. **Atualizar Validade V2Ray** - Testar atualização V2Ray
11. **Desabilitar V2Ray** - Testar desabilitação V2Ray
12. **Habilitar V2Ray** - Testar habilitação V2Ray
13. **Deletar Usuários SSH** - Testar deleção SSH
14. **Deletar Usuários V2Ray** - Testar deleção V2Ray

### 3. **Códigos de Status HTTP**

#### ✅ **Sucesso (2xx)**
- **200 OK**: Operação realizada com sucesso
  - Usuários criados, atualizados, habilitados/desabilitados
  - Consultas de usuários online
  - Health check

#### ❌ **Erro do Cliente (4xx)**
- **400 Bad Request**: Dados inválidos ou malformados
  - Validação de campos obrigatórios
  - Formato de UUID inválido
  - Data de expiração inválida
  - Username reservado
  - Lista vazia para deleção

- **401 Unauthorized**: Token de autenticação inválido
  - API key ausente ou incorreta
  - Formato do header Authorization inválido

- **404 Not Found**: Recurso não encontrado
  - Usuário SSH não encontrado
  - UUID V2Ray não encontrado

#### ⚠️ **Erro do Servidor (5xx)**
- **500 Internal Server Error**: Erro interno do servidor
  - Falha na comunicação com serviços externos
  - Erro de sistema operacional
  - Falha na execução de comandos

---

## 🔧 **Troubleshooting e Problemas Comuns**

### ❌ **Erro 401 - Token de autorização inválido**
**Problema:** API retorna erro de autenticação
**Soluções:**
- Verificar se a variável de ambiente `API_ATLAS_KEY` está definida
- Confirmar se o header `Authorization: Bearer sua-chave-api` está correto
- Verificar se não há espaços extras na chave da API

### ❌ **Erro 400 - Dados inválidos**
**Problema:** Validação de dados falha
**Soluções:**
- **SSH Users:**
  - Username: 3-32 caracteres, apenas alfanumérico
  - Password: mínimo 6 caracteres
  - Validate: número inteiro positivo (dias)
  - Evitar usernames reservados: root, admin, sshd, www-data, postgres, mysql, nginx, apache

- **V2Ray Users:**
  - UUID: formato UUID4 válido (ex: 550e8400-e29b-41d4-a716-446655440000)
  - Expiration Date: formato ISO 8601 (ex: 2025-12-31T23:59:59Z)

### ❌ **Erro 500 - Erro interno do servidor**
**Problema:** Falha na execução de comandos do sistema
**Soluções:**
- Verificar se o usuário tem permissões para executar comandos SSH/V2Ray
- Verificar se os serviços SSH e V2Ray estão rodando
- Verificar logs do sistema para mais detalhes

### ⚠️ **Usuários não aparecem online**
**Problema:** Usuários criados não aparecem na lista de online
**Soluções:**
- Aguardar até 2 minutos (SSH) ou 90 segundos (V2Ray) para atualização do cache
- Verificar se os usuários estão realmente conectados
- Verificar se os serviços de monitoramento estão rodando

### ⚠️ **Usuários de teste não são removidos**
**Problema:** Usuários de teste SSH não são removidos automaticamente
**Soluções:**
- Verificar se o cronjob está rodando (a cada 5 minutos)
- Verificar logs do sistema para erros no cronjob
- Verificar se o arquivo de cronjobs está sendo atualizado

### ⚠️ **Usuários V2Ray expirados não são removidos**
**Problema:** Usuários V2Ray expirados não são removidos automaticamente
**Soluções:**
- Verificar se o cronjob está rodando (a cada 1 hora)
- Verificar logs do sistema para erros no cronjob
- Verificar se o arquivo de cronjobs está sendo atualizado

### 📋 **Verificações de Sistema**
```bash
# Verificar se a API está rodando
curl -H "Authorization: Bearer sua-chave-api" http://localhost:8080/

# Verificar variável de ambiente
echo $API_ATLAS_KEY

# Verificar logs do sistema
tail -f /var/log/syslog | grep api-v2

# Verificar arquivo de cronjobs
cat /root/dragoncore/temp/cronjobs.json
```

---

## 📝 **Notas Importantes**

### Validações SSH
- **Username**: 3-32 caracteres, alfanumérico
- **Password**: Mínimo 6 caracteres
- **Validate**: Número inteiro positivo (dias)
- **Usernames reservados**: root, admin, sshd, www-data, postgres, mysql, nginx, apache

### Validações V2Ray
- **UUID**: Formato UUID4 válido
- **Expiration Date**: Formato ISO 8601 (RFC3339)

### Cronjobs Automáticos
- **Usuários teste SSH**: Removidos a cada 5 minutos (máximo 72 horas de teste, 3 dias de validade)
- **Usuários V2Ray expirados**: Removidos a cada 1 hora

### Logs
- **SSH**: `./logs/ssh_user_creation_errors.log`
- **Cronjobs**: Console da aplicação
- **Cronjobs**: `/root/dragoncore/temp/cronjobs.json`

---

**🎉 Pronto para testar no Postman! Todas as rotas estão documentadas com exemplos completos.**

---

## 📊 **Resumo das Rotas**

| Método | Rota | Descrição | Status |
|--------|------|-----------|--------|
| `GET` | `/` | Health check | ✅ |
| `GET` | `/onlines` | Contadores de usuários online | ✅ |
| `GET` | `/users/online` | Lista detalhada de usuários online | ✅ |
| `GET` | `/system/resources` | Recursos do sistema (CPU e RAM) | ✅ |
| `POST` | `/ssh_user` | Criar usuários SSH | ✅ |
| `POST` | `/ssh_user/test` | Criar usuário de teste SSH (3 dias) | ✅ |
| `PUT` | `/ssh_user/{username}` | Atualizar usuário SSH | ✅ |
| `PUT` | `/ssh_user/disable/{username}` | Desabilitar usuário SSH | ✅ |
| `PUT` | `/ssh_user/enable/{username}` | Habilitar usuário SSH | ✅ |
| `POST` | `/ssh_user/delete` | Deletar usuários SSH | ✅ |
| `POST` | `/v2ray_user` | Criar usuários V2Ray | ✅ |
| `POST` | `/v2ray/test` | Criar usuário de teste V2Ray | ✅ |
| `PUT` | `/v2ray_user/{uuid}` | Atualizar validade V2Ray | ✅ |
| `PUT` | `/v2ray_user/disable/{uuid}` | Desabilitar usuário V2Ray | ✅ |
| `PUT` | `/v2ray_user/enable/{uuid}` | Habilitar usuário V2Ray | ✅ |
| `POST` | `/v2ray_user/delete` | Deletar usuários V2Ray | ✅ |

**Total: 16 rotas implementadas e documentadas** 🚀
