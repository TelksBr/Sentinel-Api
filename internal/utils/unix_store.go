package utils

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultBaseDir = "/etc"
	DefaultStartUID = 1000
)

// PasswdEntry representa uma linha em /etc/passwd
// username:password:UID:GID:GECOS:home_dir:shell
type PasswdEntry struct {
	Username string
	Password string // "x"
	UID      int
	GID      int
	GECOS    string
	Home     string
	Shell    string
}

// ShadowEntry representa uma linha em /etc/shadow
// username:password_hash:last_changed:min:max:warn:inact:expire:reserved
type ShadowEntry struct {
	Username     string
	PasswordHash string
	LastChanged  string // Dias desde 1970-01-01
	MinDays      string // "0"
	MaxDays      string // "99999"
	WarnDays     string // "7"
	InactDays    string // ""
	ExpireDays   string // Dias desde 1970-01-01 ou ""
	Reserved     string // ""
}

// GroupEntry representa uma linha em /etc/group
// group_name:password:GID:members
type GroupEntry struct {
	Name     string
	Password string // "x"
	GID      int
	Members  []string
}

// GShadowEntry representa uma linha em /etc/gshadow
// group_name:password:admins:members
type GShadowEntry struct {
	Name     string
	Password string // "!" ou ""
	Admins   []string
	Members  []string
}

// UnixStore gerencia as tabelas de usuários do Unix em memória com sincronização atômica.
type UnixStore struct {
	baseDir  string
	mutex    sync.Mutex

	Passwd  []PasswdEntry
	Shadow  []ShadowEntry
	Group   []GroupEntry
	GShadow []GShadowEntry

	passwdMap  map[string]int // username -> slice index
	shadowMap  map[string]int // username -> slice index
	groupMap   map[string]int // name -> slice index
	gshadowMap map[string]int // name -> slice index
}

// NewUnixStore cria uma nova instância do gerenciador UnixStore.
func NewUnixStore(baseDir string) *UnixStore {
	if baseDir == "" {
		baseDir = DefaultBaseDir
	}
	return &UnixStore{
		baseDir:    baseDir,
		passwdMap:  make(map[string]int),
		shadowMap:  make(map[string]int),
		groupMap:   make(map[string]int),
		gshadowMap: make(map[string]int),
	}
}

// DefaultUnixStore singleton global para operações no sistema Linux.
var DefaultUnixStore = NewUnixStore(DefaultBaseDir)

// Lock adquire o mutex de memória do UnixStore.
func (s *UnixStore) Lock() (func(), error) {
	s.mutex.Lock()
	return func() {
		s.mutex.Unlock()
	}, nil
}

// Load carrega e faz o parsing dos 4 arquivos do Unix.
func (s *UnixStore) Load() error {
	passwdPath := filepath.Join(s.baseDir, "passwd")
	shadowPath := filepath.Join(s.baseDir, "shadow")
	groupPath := filepath.Join(s.baseDir, "group")
	gshadowPath := filepath.Join(s.baseDir, "gshadow")

	// 1. Carregar Passwd
	passwdEntries, err := s.loadPasswd(passwdPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("erro ao carregar %s: %w", passwdPath, err)
	}

	// 2. Carregar Shadow
	shadowEntries, err := s.loadShadow(shadowPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("erro ao carregar %s: %w", shadowPath, err)
	}

	// 3. Carregar Group
	groupEntries, err := s.loadGroup(groupPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("erro ao carregar %s: %w", groupPath, err)
	}

	// 4. Carregar GShadow
	gshadowEntries, err := s.loadGShadow(gshadowPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("erro ao carregar %s: %w", gshadowPath, err)
	}

	s.Passwd = passwdEntries
	s.Shadow = shadowEntries
	s.Group = groupEntries
	s.GShadow = gshadowEntries

	s.rebuildMaps()
	return nil
}

func (s *UnixStore) rebuildMaps() {
	s.passwdMap = make(map[string]int, len(s.Passwd))
	for i, e := range s.Passwd {
		s.passwdMap[e.Username] = i
	}

	s.shadowMap = make(map[string]int, len(s.Shadow))
	for i, e := range s.Shadow {
		s.shadowMap[e.Username] = i
	}

	s.groupMap = make(map[string]int, len(s.Group))
	for i, e := range s.Group {
		s.groupMap[e.Name] = i
	}

	s.gshadowMap = make(map[string]int, len(s.GShadow))
	for i, e := range s.GShadow {
		s.gshadowMap[e.Name] = i
	}
}

// Save salva atomicamente os 4 arquivos em disco usando rename.
func (s *UnixStore) Save() error {
	pid := os.Getpid()
	timestamp := time.Now().UnixNano()

	passwdPath := filepath.Join(s.baseDir, "passwd")
	shadowPath := filepath.Join(s.baseDir, "shadow")
	groupPath := filepath.Join(s.baseDir, "group")
	gshadowPath := filepath.Join(s.baseDir, "gshadow")

	tmpPasswd := fmt.Sprintf("%s.tmp.%d.%d", passwdPath, pid, timestamp)
	tmpShadow := fmt.Sprintf("%s.tmp.%d.%d", shadowPath, pid, timestamp)
	tmpGroup := fmt.Sprintf("%s.tmp.%d.%d", groupPath, pid, timestamp)
	tmpGShadow := fmt.Sprintf("%s.tmp.%d.%d", gshadowPath, pid, timestamp)

	// Obter permissões existentes ou padrões seguros
	passwdMode := os.FileMode(0644)
	if info, err := os.Stat(passwdPath); err == nil {
		passwdMode = info.Mode()
	}
	shadowMode := os.FileMode(0640)
	if info, err := os.Stat(shadowPath); err == nil {
		shadowMode = info.Mode()
	}
	groupMode := os.FileMode(0644)
	if info, err := os.Stat(groupPath); err == nil {
		groupMode = info.Mode()
	}
	gshadowMode := os.FileMode(0640)
	if info, err := os.Stat(gshadowPath); err == nil {
		gshadowMode = info.Mode()
	}

	// 1. Escrever tmpPasswd
	if err := s.writePasswdFile(tmpPasswd, passwdMode); err != nil {
		_ = os.Remove(tmpPasswd)
		return err
	}

	// 2. Escrever tmpShadow
	if err := s.writeShadowFile(tmpShadow, shadowMode); err != nil {
		_ = os.Remove(tmpPasswd)
		_ = os.Remove(tmpShadow)
		return err
	}

	// 3. Escrever tmpGroup
	if err := s.writeGroupFile(tmpGroup, groupMode); err != nil {
		_ = os.Remove(tmpPasswd)
		_ = os.Remove(tmpShadow)
		_ = os.Remove(tmpGroup)
		return err
	}

	// 4. Escrever tmpGShadow
	if err := s.writeGShadowFile(tmpGShadow, gshadowMode); err != nil {
		_ = os.Remove(tmpPasswd)
		_ = os.Remove(tmpShadow)
		_ = os.Remove(tmpGroup)
		_ = os.Remove(tmpGShadow)
		return err
	}

	// Renomear atomicamente
	if err := os.Rename(tmpPasswd, passwdPath); err != nil {
		return fmt.Errorf("falha ao renomear %s -> %s: %w", tmpPasswd, passwdPath, err)
	}
	if err := os.Rename(tmpShadow, shadowPath); err != nil {
		return fmt.Errorf("falha ao renomear %s -> %s: %w", tmpShadow, shadowPath, err)
	}
	if err := os.Rename(tmpGroup, groupPath); err != nil {
		return fmt.Errorf("falha ao renomear %s -> %s: %w", tmpGroup, groupPath, err)
	}
	if err := os.Rename(tmpGShadow, gshadowPath); err != nil {
		return fmt.Errorf("falha ao renomear %s -> %s: %w", tmpGShadow, gshadowPath, err)
	}

	return nil
}

// NextAvailableUID encontra o próximo UID/GID livre a partir de startUID (padrão 1000).
func (s *UnixStore) NextAvailableUID(startUID int) int {
	if startUID < 1000 {
		startUID = DefaultStartUID
	}

	usedUIDs := make(map[int]bool, len(s.Passwd))
	for _, p := range s.Passwd {
		usedUIDs[p.UID] = true
	}
	for _, g := range s.Group {
		usedUIDs[g.GID] = true
	}

	for uid := startUID; uid < 65000; uid++ {
		if !usedUIDs[uid] {
			return uid
		}
	}
	return 65000
}

// IsSystemUser verifica se um usuário é considerado protegido/sistema (UID < 1000 ou nome reservado).
func (s *UnixStore) IsSystemUser(username string) bool {
	if IsReservedUsername(username) {
		return true
	}
	if idx, exists := s.passwdMap[username]; exists {
		if s.Passwd[idx].UID < 1000 {
			return true
		}
	}
	return false
}

// FormatLimitGECOS formata o limite de conexões para o campo GECOS do /etc/passwd.
// Exemplo: 2 -> "limit=2", 0 -> "limit=0".
func FormatLimitGECOS(limit int) string {
	if limit < 0 {
		limit = 0
	}
	return fmt.Sprintf("limit=%d", limit)
}

// ParseLimitGECOS extrai o limite de conexões a partir do campo GECOS.
// Suporta formatos como "limit=2", "2", ou campos delimitados por vírgula como "Nome,limit=2".
// Retorna 0 (ilimitado) caso o campo esteja vazio ou não contenha um limite válido.
func ParseLimitGECOS(gecos string) int {
	gecos = strings.TrimSpace(gecos)
	if gecos == "" {
		return 0
	}

	// Se houver subcampos separados por vírgula (formato padrão GECOS)
	fields := strings.Split(gecos, ",")
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if strings.HasPrefix(f, "limit=") {
			valStr := strings.TrimPrefix(f, "limit=")
			if val, err := strconv.Atoi(valStr); err == nil && val >= 0 {
				return val
			}
		}
	}

	// Caso seja diretamente "limit=N"
	if strings.HasPrefix(gecos, "limit=") {
		valStr := strings.TrimPrefix(gecos, "limit=")
		if val, err := strconv.Atoi(valStr); err == nil && val >= 0 {
			return val
		}
	}

	// Caso seja apenas um número puro "N"
	if val, err := strconv.Atoi(gecos); err == nil && val >= 0 {
		return val
	}

	return 0
}

// UpsertUser insere ou atualiza um usuário SSH nas tabelas do Unix.
func (s *UnixStore) UpsertUser(username, passwordHash string, uid, gid int, expireDays string, shell string, limit int) (isNew bool) {
	todayEpochDays := strconv.FormatInt(time.Now().Unix()/86400, 10)
	if shell == "" {
		shell = "/bin/false"
	}
	gecos := FormatLimitGECOS(limit)

	// Verificar se usuário já existe
	pIdx, pExists := s.passwdMap[username]
	sIdx, sExists := s.shadowMap[username]

	if pExists && sExists {
		// Atualizar existente
		s.Passwd[pIdx].Shell = shell
		s.Passwd[pIdx].GECOS = gecos
		s.Shadow[sIdx].PasswordHash = passwordHash
		s.Shadow[sIdx].LastChanged = todayEpochDays
		s.Shadow[sIdx].ExpireDays = expireDays
		return false
	}

	// Novo usuário: alocar UID/GID se não fornecido
	if uid <= 0 {
		uid = s.NextAvailableUID(DefaultStartUID)
	}
	if gid <= 0 {
		gid = uid
	}

	// 1. Passwd
	passwdEntry := PasswdEntry{
		Username: username,
		Password: "x",
		UID:      uid,
		GID:      gid,
		GECOS:    gecos,
		Home:     "/nonexistent",
		Shell:    shell,
	}
	if pExists {
		s.Passwd[pIdx] = passwdEntry
	} else {
		s.Passwd = append(s.Passwd, passwdEntry)
		s.passwdMap[username] = len(s.Passwd) - 1
	}

	// 2. Shadow
	shadowEntry := ShadowEntry{
		Username:     username,
		PasswordHash: passwordHash,
		LastChanged:  todayEpochDays,
		MinDays:      "0",
		MaxDays:      "99999",
		WarnDays:     "7",
		InactDays:    "",
		ExpireDays:   expireDays,
		Reserved:     "",
	}
	if sExists {
		s.Shadow[sIdx] = shadowEntry
	} else {
		s.Shadow = append(s.Shadow, shadowEntry)
		s.shadowMap[username] = len(s.Shadow) - 1
	}

	// 3. Group (criar grupo correspondente com GID = UID)
	if _, gExists := s.groupMap[username]; !gExists {
		groupEntry := GroupEntry{
			Name:     username,
			Password: "x",
			GID:      gid,
			Members:  nil,
		}
		s.Group = append(s.Group, groupEntry)
		s.groupMap[username] = len(s.Group) - 1
	}

	// 4. GShadow
	if _, gsExists := s.gshadowMap[username]; !gsExists {
		gshadowEntry := GShadowEntry{
			Name:     username,
			Password: "!",
			Admins:   nil,
			Members:  nil,
		}
		s.GShadow = append(s.GShadow, gshadowEntry)
		s.gshadowMap[username] = len(s.GShadow) - 1
	}

	return true
}

// UpdateUserLimit atualiza o limite no campo GECOS de um usuário existente.
func (s *UnixStore) UpdateUserLimit(username string, limit int) bool {
	pIdx, pExists := s.passwdMap[username]
	if !pExists {
		return false
	}
	s.Passwd[pIdx].GECOS = FormatLimitGECOS(limit)
	return true
}

// GetUserLimit obtém o limite de conexões configurado no GECOS do usuário.
// Retorna o limite e se o usuário foi encontrado.
func (s *UnixStore) GetUserLimit(username string) (int, bool) {
	pIdx, pExists := s.passwdMap[username]
	if !pExists {
		return 0, false
	}
	return ParseLimitGECOS(s.Passwd[pIdx].GECOS), true
}

// DeleteUser remove um único usuário se não for usuário do sistema.
func (s *UnixStore) DeleteUser(username string) (deletedUID int, found bool, err error) {
	if s.IsSystemUser(username) {
		return -1, false, fmt.Errorf("não é permitido remover usuário protegido do sistema: %s", username)
	}

	pIdx, exists := s.passwdMap[username]
	if !exists {
		return -1, false, nil
	}

	deletedUID = s.Passwd[pIdx].UID

	// Remover de Passwd
	s.Passwd = append(s.Passwd[:pIdx], s.Passwd[pIdx+1:]...)

	// Remover de Shadow
	if sIdx, sExists := s.shadowMap[username]; sExists {
		s.Shadow = append(s.Shadow[:sIdx], s.Shadow[sIdx+1:]...)
	}

	// Remover de Group
	if gIdx, gExists := s.groupMap[username]; gExists {
		s.Group = append(s.Group[:gIdx], s.Group[gIdx+1:]...)
	}

	// Remover de GShadow
	if gsIdx, gsExists := s.gshadowMap[username]; gsExists {
		s.GShadow = append(s.GShadow[:gsIdx], s.GShadow[gsIdx+1:]...)
	}

	s.rebuildMaps()
	return deletedUID, true, nil
}

// DeleteUsers remove um lote de usuários e retorna a lista de UIDs deletados.
func (s *UnixStore) DeleteUsers(usernames []string) (deletedUIDs []int, notFound []string, systemUsers []string) {
	// 1. Identificar usuários do sistema antes de alterar qualquer fatia
	systemMap := make(map[string]bool, len(s.Passwd))
	for _, p := range s.Passwd {
		if p.UID < 1000 || IsReservedUsername(p.Username) {
			systemMap[p.Username] = true
		}
	}

	targets := make(map[string]bool, len(usernames))
	for _, u := range usernames {
		if systemMap[u] || IsReservedUsername(u) {
			systemUsers = append(systemUsers, u)
			continue
		}
		targets[u] = true
	}

	// 2. Filtrar Passwd
	deletedMap := make(map[int]bool)
	filteredPasswd := make([]PasswdEntry, 0, len(s.Passwd))
	for _, p := range s.Passwd {
		if targets[p.Username] {
			deletedUIDs = append(deletedUIDs, p.UID)
			deletedMap[p.UID] = true
			delete(targets, p.Username)
		} else {
			filteredPasswd = append(filteredPasswd, p)
		}
	}
	s.Passwd = filteredPasswd

	// Usuários que sobraram em targets não foram encontrados
	for u := range targets {
		notFound = append(notFound, u)
	}

	// 3. Filtrar Shadow
	deletedUsernamesMap := make(map[string]bool, len(deletedUIDs))
	for _, p := range s.Passwd {
		deletedUsernamesMap[p.Username] = false
	}
	filteredShadow := make([]ShadowEntry, 0, len(s.Shadow))
	for _, sh := range s.Shadow {
		if systemMap[sh.Username] || !deletedMap[s.getUIDFromShadow(sh.Username)] {
			filteredShadow = append(filteredShadow, sh)
		}
	}
	// Mais simples e seguro para Shadow: manter se ainda estiver no Passwd ou se for system user
	passwdUserSet := make(map[string]bool, len(s.Passwd))
	for _, p := range s.Passwd {
		passwdUserSet[p.Username] = true
	}
	filteredShadow = make([]ShadowEntry, 0, len(s.Shadow))
	for _, sh := range s.Shadow {
		if passwdUserSet[sh.Username] || systemMap[sh.Username] {
			filteredShadow = append(filteredShadow, sh)
		}
	}
	s.Shadow = filteredShadow

	// 4. Filtrar Group e GShadow
	filteredGroup := make([]GroupEntry, 0, len(s.Group))
	for _, g := range s.Group {
		if systemMap[g.Name] || (!deletedMap[g.GID] && (g.GID < 1000 || !targets[g.Name])) {
			filteredGroup = append(filteredGroup, g)
		}
	}
	s.Group = filteredGroup

	filteredGShadow := make([]GShadowEntry, 0, len(s.GShadow))
	groupNameSet := make(map[string]bool, len(s.Group))
	for _, g := range s.Group {
		groupNameSet[g.Name] = true
	}
	for _, gs := range s.GShadow {
		if groupNameSet[gs.Name] || systemMap[gs.Name] {
			filteredGShadow = append(filteredGShadow, gs)
		}
	}
	s.GShadow = filteredGShadow

	s.rebuildMaps()
	return deletedUIDs, notFound, systemUsers
}

func (s *UnixStore) getUIDFromShadow(username string) int {
	if idx, ok := s.passwdMap[username]; ok && idx < len(s.Passwd) {
		return s.Passwd[idx].UID
	}
	return -1
}

// DeleteAllNonSystemUsers remove todos os usuários SSH com shell /bin/false ou /usr/sbin/nologin (UID >= 1000 e não reservados).
// Usuários com /bin/bash, /bin/sh, /bin/zsh ou outros shells interativos NUNCA são deletados.
func (s *UnixStore) DeleteAllNonSystemUsers() (deletedUsernames []string, deletedUIDs []int, totalDeleted int) {
	// Identificar sistema primeiro
	systemUsersSet := make(map[string]bool, len(s.Passwd))
	for _, p := range s.Passwd {
		if p.UID < 1000 || IsReservedUsername(p.Username) {
			systemUsersSet[p.Username] = true
		}
	}

	filteredPasswd := make([]PasswdEntry, 0, len(s.Passwd))
	for _, p := range s.Passwd {
		// Deletar apenas se UID >= 1000, não reservado e com shell /bin/false ou /usr/sbin/nologin
		if p.UID >= 1000 && !IsReservedUsername(p.Username) && (p.Shell == "/bin/false" || p.Shell == "/usr/sbin/nologin") {
			deletedUIDs = append(deletedUIDs, p.UID)
			deletedUsernames = append(deletedUsernames, p.Username)
		} else {
			filteredPasswd = append(filteredPasswd, p)
		}
	}
	s.Passwd = filteredPasswd

	// Filtrar Shadow mantendo apenas system users e os que restaram no Passwd
	passwdUserSet := make(map[string]bool, len(s.Passwd))
	for _, p := range s.Passwd {
		passwdUserSet[p.Username] = true
	}
	filteredShadow := make([]ShadowEntry, 0, len(s.Shadow))
	for _, sh := range s.Shadow {
		if systemUsersSet[sh.Username] || passwdUserSet[sh.Username] {
			filteredShadow = append(filteredShadow, sh)
		}
	}
	s.Shadow = filteredShadow

	// Filtrar Group
	filteredGroup := make([]GroupEntry, 0, len(s.Group))
	for _, g := range s.Group {
		if g.GID < 1000 || IsReservedUsername(g.Name) || passwdUserSet[g.Name] {
			filteredGroup = append(filteredGroup, g)
		}
	}
	s.Group = filteredGroup

	// Filtrar GShadow
	groupNameSet := make(map[string]bool, len(s.Group))
	for _, g := range s.Group {
		groupNameSet[g.Name] = true
	}
	filteredGShadow := make([]GShadowEntry, 0, len(s.GShadow))
	for _, gs := range s.GShadow {
		if systemUsersSet[gs.Name] || groupNameSet[gs.Name] {
			filteredGShadow = append(filteredGShadow, gs)
		}
	}
	s.GShadow = filteredGShadow

	s.rebuildMaps()
	return deletedUsernames, deletedUIDs, len(deletedUIDs)
}

// DeleteExpiredUsers remove todos os usuários SSH não-sistema cuja data de expiração no shadow já passou (ExpireDays <= hoje).
// Apenas usuários com shell /bin/false ou /usr/sbin/nologin e UID >= 1000 são considerados.
func (s *UnixStore) DeleteExpiredUsers() (deletedUsernames []string, deletedUIDs []int, totalDeleted int) {
	currentEpochDays := time.Now().Unix() / 86400

	// 1. Identificar sistema primeiro
	systemUsersSet := make(map[string]bool, len(s.Passwd))
	for _, p := range s.Passwd {
		if p.UID < 1000 || IsReservedUsername(p.Username) {
			systemUsersSet[p.Username] = true
		}
	}

	// 2. Mapear expirações do shadow
	expiredUsernamesMap := make(map[string]bool)
	for _, sh := range s.Shadow {
		if systemUsersSet[sh.Username] {
			continue
		}
		if sh.ExpireDays == "" {
			continue
		}
		expireDays, err := strconv.ParseInt(sh.ExpireDays, 10, 64)
		if err != nil || expireDays <= 0 {
			continue
		}
		if currentEpochDays >= expireDays {
			expiredUsernamesMap[sh.Username] = true
		}
	}

	if len(expiredUsernamesMap) == 0 {
		return nil, nil, 0
	}

	// 3. Filtrar Passwd
	filteredPasswd := make([]PasswdEntry, 0, len(s.Passwd))
	for _, p := range s.Passwd {
		if expiredUsernamesMap[p.Username] && p.UID >= 1000 && !IsReservedUsername(p.Username) && (p.Shell == "/bin/false" || p.Shell == "/usr/sbin/nologin") {
			deletedUIDs = append(deletedUIDs, p.UID)
			deletedUsernames = append(deletedUsernames, p.Username)
		} else {
			filteredPasswd = append(filteredPasswd, p)
		}
	}
	s.Passwd = filteredPasswd

	// 4. Filtrar Shadow mantendo apenas system users e os que restaram no Passwd
	passwdUserSet := make(map[string]bool, len(s.Passwd))
	for _, p := range s.Passwd {
		passwdUserSet[p.Username] = true
	}
	filteredShadow := make([]ShadowEntry, 0, len(s.Shadow))
	for _, sh := range s.Shadow {
		if systemUsersSet[sh.Username] || passwdUserSet[sh.Username] {
			filteredShadow = append(filteredShadow, sh)
		}
	}
	s.Shadow = filteredShadow

	// 5. Filtrar Group
	filteredGroup := make([]GroupEntry, 0, len(s.Group))
	for _, g := range s.Group {
		if g.GID < 1000 || IsReservedUsername(g.Name) || passwdUserSet[g.Name] {
			filteredGroup = append(filteredGroup, g)
		}
	}
	s.Group = filteredGroup

	// 6. Filtrar GShadow
	groupNameSet := make(map[string]bool, len(s.Group))
	for _, g := range s.Group {
		groupNameSet[g.Name] = true
	}
	filteredGShadow := make([]GShadowEntry, 0, len(s.GShadow))
	for _, gs := range s.GShadow {
		if systemUsersSet[gs.Name] || groupNameSet[gs.Name] {
			filteredGShadow = append(filteredGShadow, gs)
		}
	}
	s.GShadow = filteredGShadow

	s.rebuildMaps()
	return deletedUsernames, deletedUIDs, len(deletedUIDs)
}

// SetUserDisabled desabilita um usuário (nologin + expiração no passado).
func (s *UnixStore) SetUserDisabled(username string) (uid int, err error) {
	if s.IsSystemUser(username) {
		return -1, fmt.Errorf("não é permitido desabilitar usuário do sistema: %s", username)
	}

	pIdx, pExists := s.passwdMap[username]
	sIdx, sExists := s.shadowMap[username]
	if !pExists || !sExists {
		return -1, fmt.Errorf("usuário %s não encontrado", username)
	}

	// 1. Shell nologin
	s.Passwd[pIdx].Shell = "/usr/sbin/nologin"

	// 2. Travar hash se não travado e definir expiração para ontem (dia 1 ou epoch ontem)
	if !strings.HasPrefix(s.Shadow[sIdx].PasswordHash, "!") {
		s.Shadow[sIdx].PasswordHash = "!" + s.Shadow[sIdx].PasswordHash
	}
	yesterdayEpoch := strconv.FormatInt((time.Now().Unix()/86400)-1, 10)
	s.Shadow[sIdx].ExpireDays = yesterdayEpoch

	return s.Passwd[pIdx].UID, nil
}

// SetUserEnabled reabilita um usuário (shell padrão + nova validade / restauração).
func (s *UnixStore) SetUserEnabled(username string, expireDays string) error {
	if s.IsSystemUser(username) {
		return fmt.Errorf("não é permitido alterar usuário do sistema: %s", username)
	}

	pIdx, pExists := s.passwdMap[username]
	sIdx, sExists := s.shadowMap[username]
	if !pExists || !sExists {
		return fmt.Errorf("usuário %s não encontrado", username)
	}

	// 1. Restaurar shell
	s.Passwd[pIdx].Shell = "/bin/false"

	// 2. Destravar hash se travado
	s.Shadow[sIdx].PasswordHash = strings.TrimPrefix(s.Shadow[sIdx].PasswordHash, "!")
	s.Shadow[sIdx].ExpireDays = expireDays
	s.Shadow[sIdx].LastChanged = strconv.FormatInt(time.Now().Unix()/86400, 10)

	return nil
}

// DaysToShadowExpireDays calcula os dias desde epoch (01/01/1970) para daqui a N dias.
func DaysToShadowExpireDays(daysFromNow int) string {
	if daysFromNow <= 0 {
		return ""
	}
	expTime := time.Now().AddDate(0, 0, daysFromNow)
	return strconv.FormatInt(expTime.Unix()/86400, 10)
}

// HoursToShadowExpireDays calcula os dias desde epoch para daqui a N horas (mínimo 1 dia).
func HoursToShadowExpireDays(hoursFromNow int) string {
	if hoursFromNow <= 0 {
		return ""
	}
	expTime := time.Now().Add(time.Duration(hoursFromNow) * time.Hour)
	return strconv.FormatInt(expTime.Unix()/86400, 10)
}

// --- Funções internas de parsing e escrita ---

func (s *UnixStore) loadPasswd(path string) ([]PasswdEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var entries []PasswdEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 7 {
			continue
		}
		uid, _ := strconv.Atoi(parts[2])
		gid, _ := strconv.Atoi(parts[3])
		entries = append(entries, PasswdEntry{
			Username: parts[0],
			Password: parts[1],
			UID:      uid,
			GID:      gid,
			GECOS:    parts[4],
			Home:     parts[5],
			Shell:    parts[6],
		})
	}
	return entries, scanner.Err()
}

func (s *UnixStore) loadShadow(path string) ([]ShadowEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var entries []ShadowEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 8 {
			continue
		}
		entry := ShadowEntry{
			Username:     parts[0],
			PasswordHash: parts[1],
			LastChanged:  parts[2],
			MinDays:      parts[3],
			MaxDays:      parts[4],
			WarnDays:     parts[5],
			InactDays:    parts[6],
			ExpireDays:   parts[7],
		}
		if len(parts) >= 9 {
			entry.Reserved = parts[8]
		}
		entries = append(entries, entry)
	}
	return entries, scanner.Err()
}

func (s *UnixStore) loadGroup(path string) ([]GroupEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var entries []GroupEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 3 {
			continue
		}
		gid, _ := strconv.Atoi(parts[2])
		var members []string
		if len(parts) >= 4 && parts[3] != "" {
			members = strings.Split(parts[3], ",")
		}
		entries = append(entries, GroupEntry{
			Name:     parts[0],
			Password: parts[1],
			GID:      gid,
			Members:  members,
		})
	}
	return entries, scanner.Err()
}

func (s *UnixStore) loadGShadow(path string) ([]GShadowEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var entries []GShadowEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 2 {
			continue
		}
		var admins, members []string
		if len(parts) >= 3 && parts[2] != "" {
			admins = strings.Split(parts[2], ",")
		}
		if len(parts) >= 4 && parts[3] != "" {
			members = strings.Split(parts[3], ",")
		}
		entries = append(entries, GShadowEntry{
			Name:     parts[0],
			Password: parts[1],
			Admins:   admins,
			Members:  members,
		})
	}
	return entries, scanner.Err()
}

func (s *UnixStore) writePasswdFile(path string, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer file.Close()

	w := bufio.NewWriter(file)
	for _, e := range s.Passwd {
		line := fmt.Sprintf("%s:%s:%d:%d:%s:%s:%s\n",
			e.Username, e.Password, e.UID, e.GID, e.GECOS, e.Home, e.Shell)
		if _, err := w.WriteString(line); err != nil {
			return err
		}
	}
	return w.Flush()
}

func (s *UnixStore) writeShadowFile(path string, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer file.Close()

	w := bufio.NewWriter(file)
	for _, e := range s.Shadow {
		line := fmt.Sprintf("%s:%s:%s:%s:%s:%s:%s:%s:%s\n",
			e.Username, e.PasswordHash, e.LastChanged, e.MinDays, e.MaxDays, e.WarnDays, e.InactDays, e.ExpireDays, e.Reserved)
		if _, err := w.WriteString(line); err != nil {
			return err
		}
	}
	return w.Flush()
}

func (s *UnixStore) writeGroupFile(path string, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer file.Close()

	w := bufio.NewWriter(file)
	for _, e := range s.Group {
		membersStr := strings.Join(e.Members, ",")
		line := fmt.Sprintf("%s:%s:%d:%s\n", e.Name, e.Password, e.GID, membersStr)
		if _, err := w.WriteString(line); err != nil {
			return err
		}
	}
	return w.Flush()
}

func (s *UnixStore) writeGShadowFile(path string, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer file.Close()

	w := bufio.NewWriter(file)
	for _, e := range s.GShadow {
		adminsStr := strings.Join(e.Admins, ",")
		membersStr := strings.Join(e.Members, ",")
		line := fmt.Sprintf("%s:%s:%s:%s\n", e.Name, e.Password, adminsStr, membersStr)
		if _, err := w.WriteString(line); err != nil {
			return err
		}
	}
	return w.Flush()
}

// CountSSHUsers conta quantos usuários SSH (não-sistema com /bin/false ou /usr/sbin/nologin) existem no UnixStore carregado.
func (s *UnixStore) CountSSHUsers() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	count := 0
	for _, p := range s.Passwd {
		if p.UID >= 1000 && !IsReservedUsername(p.Username) && (p.Shell == "/bin/false" || p.Shell == "/usr/sbin/nologin") {
			count++
		}
	}
	return count
}

// CountTotalSSHUsers lê o arquivo passwd diretamente e conta usuários SSH não-sistema com /bin/false ou /usr/sbin/nologin.
// Leitura ultrarrápida (< 0.1ms).
func CountTotalSSHUsers(baseDir ...string) int {
	dir := DefaultBaseDir
	if len(baseDir) > 0 && baseDir[0] != "" {
		dir = baseDir[0]
	}

	passwdPath := filepath.Join(dir, "passwd")
	file, err := os.Open(passwdPath)
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 7 {
			continue
		}
		username := parts[0]
		uid, err := strconv.Atoi(parts[2])
		if err != nil {
			continue
		}
		shell := parts[6]
		if uid >= 1000 && !IsReservedUsername(username) && (shell == "/bin/false" || shell == "/usr/sbin/nologin") {
			count++
		}
	}
	return count
}

// CountExpiredSSHUsers conta quantos usuários SSH expirados existem no UnixStore carregado.
func (s *UnixStore) CountExpiredSSHUsers() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	currentEpochDays := time.Now().Unix() / 86400

	systemUsersSet := make(map[string]bool, len(s.Passwd))
	for _, p := range s.Passwd {
		if p.UID < 1000 || IsReservedUsername(p.Username) {
			systemUsersSet[p.Username] = true
		}
	}

	expiredMap := make(map[string]bool)
	for _, sh := range s.Shadow {
		if systemUsersSet[sh.Username] || sh.ExpireDays == "" {
			continue
		}
		exp, err := strconv.ParseInt(sh.ExpireDays, 10, 64)
		if err == nil && exp > 0 && currentEpochDays >= exp {
			expiredMap[sh.Username] = true
		}
	}

	count := 0
	for _, p := range s.Passwd {
		if expiredMap[p.Username] && p.UID >= 1000 && !IsReservedUsername(p.Username) && (p.Shell == "/bin/false" || p.Shell == "/usr/sbin/nologin") {
			count++
		}
	}
	return count
}

// CountTotalExpiredSSHUsers lê os arquivos shadow e passwd diretamente e conta usuários SSH expirados (não-sistema com /bin/false ou /usr/sbin/nologin).
// Leitura ultrarrápida (< 0.2ms).
func CountTotalExpiredSSHUsers(baseDir ...string) int {
	dir := DefaultBaseDir
	if len(baseDir) > 0 && baseDir[0] != "" {
		dir = baseDir[0]
	}

	shadowPath := filepath.Join(dir, "shadow")
	shadowFile, err := os.Open(shadowPath)
	if err != nil {
		return 0
	}
	defer shadowFile.Close()

	currentEpochDays := time.Now().Unix() / 86400
	expiredShadow := make(map[string]bool)

	shadowScanner := bufio.NewScanner(shadowFile)
	for shadowScanner.Scan() {
		line := strings.TrimSpace(shadowScanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 8 {
			continue
		}
		username := parts[0]
		expireDaysStr := parts[7]
		if expireDaysStr == "" {
			continue
		}
		expireDays, err := strconv.ParseInt(expireDaysStr, 10, 64)
		if err != nil || expireDays <= 0 {
			continue
		}
		if currentEpochDays >= expireDays {
			expiredShadow[username] = true
		}
	}

	if len(expiredShadow) == 0 {
		return 0
	}

	passwdPath := filepath.Join(dir, "passwd")
	passwdFile, err := os.Open(passwdPath)
	if err != nil {
		return 0
	}
	defer passwdFile.Close()

	passwdScanner := bufio.NewScanner(passwdFile)
	count := 0
	for passwdScanner.Scan() {
		line := strings.TrimSpace(passwdScanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 7 {
			continue
		}
		username := parts[0]
		if !expiredShadow[username] {
			continue
		}
		uid, err := strconv.Atoi(parts[2])
		if err != nil {
			continue
		}
		shell := parts[6]
		if uid >= 1000 && !IsReservedUsername(username) && (shell == "/bin/false" || shell == "/usr/sbin/nologin") {
			count++
		}
	}

	return count
}

