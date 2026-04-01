package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"

	"api-v2/internal/cron"
	"api-v2/internal/middleware"
	"api-v2/internal/routes"
	"api-v2/internal/services"
)

func main() {
	// Otimização: Limitar GOMAXPROCS para reduzir consumo de CPU
	// Usar 50% dos cores disponíveis (mínimo 1, máximo 4)
	numCPU := runtime.NumCPU()
	maxProcs := numCPU / 2
	if maxProcs < 1 {
		maxProcs = 1
	}
	if maxProcs > 4 {
		maxProcs = 4
	}
	runtime.GOMAXPROCS(maxProcs)
	log.Printf("⚙️ GOMAXPROCS configurado: %d (de %d cores disponíveis)", maxProcs, numCPU)

	// Flags de linha de comando
	port := flag.Int("port", 8080, "Porta para o servidor HTTP")
	tlsCert := flag.String("tls-cert", "", "Caminho para o certificado TLS (opcional)")
	tlsKey := flag.String("tls-key", "", "Caminho para a chave privada TLS (opcional)")
	flag.Parse()

	// Retrocompatibilidade: se passou porta como argumento posicional (sem flag)
	if flag.NArg() > 0 && *port == 8080 {
		if p, err := fmt.Sscanf(flag.Arg(0), "%d", port); err != nil || p != 1 {
			fmt.Fprintf(os.Stderr, "❌ Erro: Porta inválida '%s'. Deve ser um número.\n", flag.Arg(0))
			os.Exit(1)
		}
	}

	// Obter API key da variável de ambiente
	apiKey, err := middleware.GetAPIKeyFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Erro: %v\n", err)
		fmt.Fprintln(os.Stderr, "Defina a variável de ambiente API_ATLAS_KEY com sua chave de API")
		fmt.Fprintln(os.Stderr, "Exemplo: export API_ATLAS_KEY=minha-chave-api")
		os.Exit(1)
	}

	// Inicializar serviços
	sshService := services.NewSSHService()
	v2rayService := services.NewV2RayService()
	monitorService := services.NewMonitorService(v2rayService.GetConfigPath())

	// Inicializar sistema de cronjobs
	cronService := cron.NewCronjobService(sshService, v2rayService)
	if err := cronService.Start(); err != nil {
		fmt.Printf("Erro ao iniciar serviço de cronjobs: %v\n", err)
		os.Exit(1)
	}
	defer cronService.Stop()

	// Inicializar serviço de monitoramento
	monitorService.Start()
	defer monitorService.Stop()

	// Configurar rotas
	authMiddleware := middleware.NewAuthMiddleware(apiKey)
	router := routes.SetupRoutes(sshService, v2rayService, monitorService, cronService, authMiddleware)

	// Iniciar servidor
	addr := fmt.Sprintf(":%d", *port)
	fmt.Printf("🚀 Iniciando servidor na porta %d...\n", *port)
	fmt.Printf("🔑 API Key obtida da variável de ambiente API_ATLAS_KEY\n")
	fmt.Printf("📁 Detectando configurações V2Ray/Xray...\n")
	fmt.Printf("⏰ Cronjobs iniciados (usuários teste: 5min, V2Ray expirados: 1h)\n")

	if *tlsCert != "" && *tlsKey != "" {
		fmt.Printf("🔒 TLS habilitado com certificado: %s\n", *tlsCert)
		fmt.Printf("🌐 Servidor rodando em: https://localhost%s\n", addr)
		if err := router.RunTLS(addr, *tlsCert, *tlsKey); err != nil {
			log.Fatalf("Erro ao iniciar servidor TLS: %v", err)
		}
	} else {
		fmt.Printf("🌐 Servidor rodando em: http://localhost%s\n", addr)
		if err := router.Run(addr); err != nil {
			log.Fatalf("Erro ao iniciar servidor: %v", err)
		}
	}
}
