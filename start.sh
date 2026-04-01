#!/bin/bash
# Script para iniciar a API V2

# Cores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}🚀 API Atlas V2 - Iniciando...${NC}"

# Verificar se a variável de ambiente está definida
if [ -z "$API_ATLAS_KEY" ]; then
    echo -e "${RED}❌ Erro: Variável de ambiente API_ATLAS_KEY não definida${NC}"
    echo -e "${YELLOW}💡 Defina a variável de ambiente:${NC}"
    echo -e "   export API_ATLAS_KEY=sua-chave-api"
    echo -e "   ${YELLOW}Ou adicione ao ~/.bashrc para persistir:${NC}"
    echo -e "   echo 'export API_ATLAS_KEY=sua-chave-api' >> ~/.bashrc"
    echo -e "   source ~/.bashrc"
    exit 1
fi

# Verificar se o binário existe
if [ ! -f "./build/api-v2" ]; then
    echo -e "${YELLOW}⚠️  Binário não encontrado. Compilando...${NC}"
    go build -o build/api-v2 ./cmd/api
    if [ $? -ne 0 ]; then
        echo -e "${RED}❌ Erro ao compilar a API${NC}"
        exit 1
    fi
    echo -e "${GREEN}✅ Compilação concluída${NC}"
fi

# Verificar se a porta foi fornecida
if [ -z "$1" ]; then
    echo -e "${YELLOW}⚠️  Porta não especificada. Usando porta padrão 8080${NC}"
    PORT=8080
else
    PORT=$1
fi

echo -e "${GREEN}🔑 API Key: ${API_ATLAS_KEY:0:8}...${NC}"
echo -e "${GREEN}🌐 Porta: $PORT${NC}"
echo -e "${GREEN}⏰ Iniciando servidor...${NC}"
echo ""

# Executar a API
./build/api-v2 $PORT
