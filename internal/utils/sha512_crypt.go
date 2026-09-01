package utils

import (
	"crypto/rand"
	"crypto/sha512"
	"fmt"
	"strings"
	"sync"
)

const (
	b64Table = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	saltChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789./"
	defaultRounds = 5000
)

// GenerateRandomSalt gera um salt aleatório seguro com tamanho especificado (padrão 16 chars).
func GenerateRandomSalt(length int) (string, error) {
	if length <= 0 {
		length = 16
	}
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.Grow(length)
	for _, b := range bytes {
		sb.WriteByte(saltChars[int(b)%len(saltChars)])
	}
	return sb.String(), nil
}

// Sha512Crypt gera o hash compatível com o formato $6$ do /etc/shadow (glibc crypt SHA-512).
func Sha512Crypt(key, salt string, rounds int) (string, error) {
	// Limpar salt caso venha com prefixo $6$
	if strings.HasPrefix(salt, "$6$") {
		parts := strings.Split(salt, "$")
		if len(parts) >= 3 {
			salt = parts[2]
		}
	}
	// Truncar salt para 16 caracteres se maior
	if len(salt) > 16 {
		salt = salt[:16]
	}
	if len(salt) == 0 {
		var err error
		salt, err = GenerateRandomSalt(16)
		if err != nil {
			return "", fmt.Errorf("falha ao gerar salt aleatório: %w", err)
		}
	}

	if rounds <= 0 {
		rounds = defaultRounds
	}
	if rounds < 1000 {
		rounds = 1000
	}
	if rounds > 999999999 {
		rounds = 999999999
	}

	keyBytes := []byte(key)
	saltBytes := []byte(salt)

	// 1. Digest B = SHA-512(key + salt + key)
	hb := sha512.New()
	hb.Write(keyBytes)
	hb.Write(saltBytes)
	hb.Write(keyBytes)
	digestB := hb.Sum(nil)

	// 2. Digest A
	ha := sha512.New()
	ha.Write(keyBytes)
	ha.Write(saltBytes)

	// Adicionar digest B repetido para cobrir o tamanho de key
	for cnt := len(keyBytes); cnt > 64; cnt -= 64 {
		ha.Write(digestB)
	}
	if rem := len(keyBytes) % 64; rem > 0 {
		ha.Write(digestB[:rem])
	}

	// Para cada bit em len(keyBytes) do LSB para MSB
	for cnt := len(keyBytes); cnt > 0; cnt >>= 1 {
		if (cnt & 1) != 0 {
			ha.Write(digestB)
		} else {
			ha.Write(keyBytes)
		}
	}
	digestA := ha.Sum(nil)

	// 3. Digest P (Key expansion)
	hp := sha512.New()
	for i := 0; i < len(keyBytes); i++ {
		hp.Write(keyBytes)
	}
	digestDP := hp.Sum(nil)
	pSeq := make([]byte, 0, len(keyBytes))
	for cnt := len(keyBytes); cnt > 64; cnt -= 64 {
		pSeq = append(pSeq, digestDP...)
	}
	if rem := len(keyBytes) % 64; rem > 0 {
		pSeq = append(pSeq, digestDP[:rem]...)
	}

	// 4. Digest S (Salt expansion)
	hs := sha512.New()
	for i := 0; i < 16+int(digestA[0]); i++ {
		hs.Write(saltBytes)
	}
	digestDS := hs.Sum(nil)
	sSeq := make([]byte, 0, len(saltBytes))
	for cnt := len(saltBytes); cnt > 64; cnt -= 64 {
		sSeq = append(sSeq, digestDS...)
	}
	if rem := len(saltBytes) % 64; rem > 0 {
		sSeq = append(sSeq, digestDS[:rem]...)
	}

	// 5. Rounds de iteração
	digestC := digestA
	for i := 0; i < rounds; i++ {
		hc := sha512.New()

		if (i & 1) != 0 {
			hc.Write(pSeq)
		} else {
			hc.Write(digestC)
		}

		if (i % 3) != 0 {
			hc.Write(sSeq)
		}

		if (i % 7) != 0 {
			hc.Write(pSeq)
		}

		if (i & 1) != 0 {
			hc.Write(digestC)
		} else {
			hc.Write(pSeq)
		}

		digestC = hc.Sum(nil)
	}

	// 6. Base64 encoding no formato glibc
	var encoded strings.Builder
	encoded.Grow(86)

	b64From24Bit := func(b2, b1, b0 byte, n int) {
		w := (uint32(b2) << 16) | (uint32(b1) << 8) | uint32(b0)
		for ; n > 0; n-- {
			encoded.WriteByte(b64Table[w&0x3f])
			w >>= 6
		}
	}

	b64From24Bit(digestC[0], digestC[21], digestC[42], 4)
	b64From24Bit(digestC[22], digestC[43], digestC[1], 4)
	b64From24Bit(digestC[44], digestC[2], digestC[23], 4)
	b64From24Bit(digestC[3], digestC[24], digestC[45], 4)
	b64From24Bit(digestC[25], digestC[46], digestC[4], 4)
	b64From24Bit(digestC[47], digestC[5], digestC[26], 4)
	b64From24Bit(digestC[6], digestC[27], digestC[48], 4)
	b64From24Bit(digestC[28], digestC[49], digestC[7], 4)
	b64From24Bit(digestC[50], digestC[8], digestC[29], 4)
	b64From24Bit(digestC[9], digestC[30], digestC[51], 4)
	b64From24Bit(digestC[31], digestC[52], digestC[10], 4)
	b64From24Bit(digestC[53], digestC[11], digestC[32], 4)
	b64From24Bit(digestC[12], digestC[33], digestC[54], 4)
	b64From24Bit(digestC[34], digestC[55], digestC[13], 4)
	b64From24Bit(digestC[56], digestC[14], digestC[35], 4)
	b64From24Bit(digestC[15], digestC[36], digestC[57], 4)
	b64From24Bit(digestC[37], digestC[58], digestC[16], 4)
	b64From24Bit(digestC[59], digestC[17], digestC[38], 4)
	b64From24Bit(digestC[18], digestC[39], digestC[60], 4)
	b64From24Bit(digestC[40], digestC[61], digestC[19], 4)
	b64From24Bit(digestC[62], digestC[20], digestC[41], 4)
	b64From24Bit(0, 0, digestC[63], 2)

	if rounds == defaultRounds {
		return fmt.Sprintf("$6$%s$%s", salt, encoded.String()), nil
	}
	return fmt.Sprintf("$6$rounds=%d$%s$%s", rounds, salt, encoded.String()), nil
}

// BatchSha512Crypt computa hashes SHA-512 em paralelo para um conjunto de senhas.
func BatchSha512Crypt(passwords []string, concurrency int) ([]string, error) {
	if concurrency <= 0 {
		concurrency = 4
	}
	if concurrency > len(passwords) && len(passwords) > 0 {
		concurrency = len(passwords)
	}

	hashes := make([]string, len(passwords))
	errs := make([]error, len(passwords))

	jobs := make(chan int, len(passwords))
	var wg sync.WaitGroup

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				hash, err := Sha512Crypt(passwords[idx], "", defaultRounds)
				hashes[idx] = hash
				errs[idx] = err
			}
		}()
	}

	for i := 0; i < len(passwords); i++ {
		jobs <- i
	}
	close(jobs)

	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	return hashes, nil
}
