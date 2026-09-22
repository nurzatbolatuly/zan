package agent

import (
	"testing"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
	"zan-backend/internal/grpcclient"
)

func TestVerifySources(t *testing.T) {
	ragMatches := []grpcclient.RagMatch{
		{Ref: "ст. 157 Трудового кодекса РК", Quote: "..."},
		{Ref: "ст. 178 Гражданского кодекса РК", Quote: "..."},
	}

	tests := []struct {
		name       string
		llmSources []domain.Source
		ragMatches []grpcclient.RagMatch
		want       bool
	}{
		{
			name:       "no sources cited -> nothing to verify",
			llmSources: nil,
			ragMatches: ragMatches,
			want:       false,
		},
		{
			name:       "all cited sources present in rag matches",
			llmSources: []domain.Source{{Ref: "ст. 157 Трудового кодекса РК", Quote: "x"}},
			ragMatches: ragMatches,
			want:       false,
		},
		{
			name:       "matches case/whitespace-insensitively",
			llmSources: []domain.Source{{Ref: "  СТ. 157 трудового кодекса рк  ", Quote: "x"}},
			ragMatches: ragMatches,
			want:       false,
		},
		{
			name:       "cited source not among rag matches -> unverified",
			llmSources: []domain.Source{{Ref: "ст. 999 Придуманного кодекса", Quote: "x"}},
			ragMatches: ragMatches,
			want:       true,
		},
		{
			name:       "one of several cited sources is not verified -> unverified",
			llmSources: []domain.Source{{Ref: "ст. 157 Трудового кодекса РК"}, {Ref: "ст. 42 Несуществующего кодекса"}},
			ragMatches: ragMatches,
			want:       true,
		},
		{
			name:       "rag degraded (no matches at all) but llm cited something -> unverified",
			llmSources: []domain.Source{{Ref: "ст. 157 Трудового кодекса РК"}},
			ragMatches: nil,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, verifySources(tt.llmSources, tt.ragMatches))
		})
	}
}
