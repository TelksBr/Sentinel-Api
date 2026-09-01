package utils

import (
	"strings"
	"testing"
)

func TestSha512Crypt_Format(t *testing.T) {
	hash, err := Sha512Crypt("teste123", "randomsalt12345", 5000)
	if err != nil {
		t.Fatalf("Erro ao gerar hash: %v", err)
	}

	if !strings.HasPrefix(hash, "$6$randomsalt12345$") {
		t.Errorf("Prefixo incorreto no hash: %s", hash)
	}

	parts := strings.Split(hash, "$")
	if len(parts) != 4 {
		t.Fatalf("Esperava 4 partes separadas por $, obteve %d (%s)", len(parts), hash)
	}

	if len(parts[3]) != 86 {
		t.Errorf("Esperava 86 caracteres na parte de hash codificado, obteve %d", len(parts[3]))
	}
}

func TestSha512Crypt_StandardVector(t *testing.T) {
	// Vetor padrão do Ulrich Drepper / glibc
	// Salt: "saltstring", Key: "Hello world!"
	// Hash esperado: $6$saltstring$svn8UoSVapNtMuq1ukKS4tPQgdEgmxTMXOpeqrqjimOVWhFioBRx./OxnoDp3BnQgCgm4AtHtp9bDRxRi9ddG.
	hash, err := Sha512Crypt("Hello world!", "saltstring", 5000)
	if err != nil {
		t.Fatalf("Erro ao gerar hash: %v", err)
	}

	expected := "$6$saltstring$svn8UoSVapNtMuq1ukKS4tPQd8iKwSMHWjl/O817G3uBnIFNjnQJuesI68u4OTLiBFdcbYEdFCoEOfaS35inz1"
	if hash != expected {
		t.Errorf("Hash gerado não confere com vetor padrão da glibc.\nObtido:   %s\nEsperado: %s", hash, expected)
	}
}

func TestBatchSha512Crypt(t *testing.T) {
	passwords := []string{"senha1", "senha2", "senha3", "senha4", "senha5"}
	hashes, err := BatchSha512Crypt(passwords, 4)
	if err != nil {
		t.Fatalf("Erro no batch crypt: %v", err)
	}

	if len(hashes) != len(passwords) {
		t.Fatalf("Esperava %d hashes, obteve %d", len(passwords), len(hashes))
	}

	for i, h := range hashes {
		if !strings.HasPrefix(h, "$6$") {
			t.Errorf("Hash %d inválido: %s", i, h)
		}
	}
}
