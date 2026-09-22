package session_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/service/session"
)

func TestTokenSigner_SignVerify_RoundTrip(t *testing.T) {
	signer := session.NewTokenSigner("secret")
	id := "3fa85f64-5717-4562-b3fc-2c963f66afa6"

	token := signer.Sign(id)
	got, err := signer.Verify(token)

	require.NoError(t, err)
	require.Equal(t, id, got)
}

func TestTokenSigner_Verify_RejectsTamperedSignature(t *testing.T) {
	signer := session.NewTokenSigner("secret")
	token := signer.Sign("3fa85f64-5717-4562-b3fc-2c963f66afa6")

	tampered := token[:len(token)-1] + "0"

	_, err := signer.Verify(tampered)

	require.ErrorIs(t, err, session.ErrInvalidToken)
}

func TestTokenSigner_Verify_RejectsForeignSecret(t *testing.T) {
	id := "3fa85f64-5717-4562-b3fc-2c963f66afa6"
	token := session.NewTokenSigner("secret-a").Sign(id)

	_, err := session.NewTokenSigner("secret-b").Verify(token)

	require.ErrorIs(t, err, session.ErrInvalidToken)
}

func TestTokenSigner_Verify_RejectsMalformedToken(t *testing.T) {
	signer := session.NewTokenSigner("secret")

	for _, token := range []string{
		"",
		"no-dot-separator",
		"not-a-uuid.deadbeef",
		"3fa85f64-5717-4562-b3fc-2c963f66afa6.not-hex",
	} {
		_, err := signer.Verify(token)
		require.ErrorIsf(t, err, session.ErrInvalidToken, "token %q", token)
	}
}
