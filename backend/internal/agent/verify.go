package agent

import (
	"strings"

	"zan-backend/internal/domain"
	"zan-backend/internal/grpcclient"
)

// verifySources — сверяет источники, которые процитировала LLM, с тем,
// что реально вернул RagService.Search — риск явно назван в backend-
// roadmap.md §1.4: "валидация, что модель не цитирует несуществующие
// статьи (галлюцинация закона), должна быть реализована именно в Go...
// стоит явно заложить эту проверку в код, а не полагаться только на
// промпт" (BACKEND_PLAN.md Stage 5: "юнит-тестируемая отдельно от
// остального пайплайна"). Вынесена в отдельную функцию намеренно — не
// метод Client, не требует сети/состояния, только сравнение двух срезов.
//
// true (unverified) — хотя бы один ref, который назвала LLM, не совпадает
// ни с одним ref, который реально вернул RAG (регистронезависимо, без
// пробелов по краям — сверка "то же самое обозначение статьи", а не
// байт-в-байт равенство строки). Пустой llmSources -> false (нечего
// проверять — ответ без ссылок не может быть недостоверным по источникам).
func verifySources(llmSources []domain.Source, ragMatches []grpcclient.RagMatch) bool {
	if len(llmSources) == 0 {
		return false
	}

	known := make(map[string]struct{}, len(ragMatches))
	for _, m := range ragMatches {
		known[normalizeRef(m.Ref)] = struct{}{}
	}

	for _, s := range llmSources {
		if _, ok := known[normalizeRef(s.Ref)]; !ok {
			return true
		}
	}
	return false
}

func normalizeRef(ref string) string {
	return strings.ToLower(strings.TrimSpace(ref))
}
