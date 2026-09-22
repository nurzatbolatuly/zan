package session

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/google/uuid"
)

// TokenSigner подписывает/проверяет opaque-токен сессии: session_id +
// HMAC-SHA256 на секрете сервера, без пользовательских данных внутри
// (BACKEND_PLAN.md §1.1, zan-backend-tz-v3.md §2.1). Формат —
// "<session_id>.<hex(hmac)>": session_id не секрет (уже возвращается
// отдельным полем в ответе POST /sessions), подделать нельзя без секрета.
type TokenSigner struct {
	secret []byte
}

// NewTokenSigner строит TokenSigner на секрете из конфигурации
// (SESSION_HMAC_SECRET — internal/config).
func NewTokenSigner(secret string) *TokenSigner {
	return &TokenSigner{secret: []byte(secret)}
}

// Sign возвращает подписанный токен для id (каноническая строка UUID).
func (s *TokenSigner) Sign(id string) string {
	return id + "." + hex.EncodeToString(s.sign(id))
}

// Verify проверяет подпись и возвращает id в каноническом виде.
// ErrInvalidToken — на любую невалидность (неверный формат, подделанная
// подпись, не-UUID id) без уточнения причины, т.к. клиенту в обоих
// случаях положен один и тот же 401 без автосоздания сессии (v3 §5.5).
func (s *TokenSigner) Verify(token string) (string, error) {
	idPart, sigPart, ok := strings.Cut(token, ".")
	if !ok {
		return "", ErrInvalidToken
	}
	parsed, err := uuid.Parse(idPart)
	if err != nil {
		return "", ErrInvalidToken
	}
	id := parsed.String()

	sig, err := hex.DecodeString(sigPart)
	if err != nil {
		return "", ErrInvalidToken
	}
	if !hmac.Equal(sig, s.sign(id)) {
		return "", ErrInvalidToken
	}
	return id, nil
}

func (s *TokenSigner) sign(id string) []byte {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(id))
	return mac.Sum(nil)
}
