package domain

import "fmt"

// ThreadStatus — статус-машина треда (zan-backend-tz-v2.md §2.6,
// buквально совпадает со строками frontend/src/shared/types/common.ts#ThreadStatus
// — "Общий словарь FE↔BE", BACKEND_PLAN.md §6 п.4). Явный тип с таблицей
// допустимых переходов (CanTransitionTo) вместо "голой" строки, которую
// можно выставить в любое значение из любого места
// (BACKEND_CODING_STANDARDS.md §6.1, пример с AgentResultStatus).
type ThreadStatus string

const (
	// ThreadStatusQueued — тред создан, ждёт либо старта обработки (баланс
	// хватило — service/thread переводит в Processing синхронно в том же
	// запросе), либо оплаты (баланса не было — zan-backend-tz-v2.md §4.2 п.2).
	ThreadStatusQueued ThreadStatus = "queued"
	// ThreadStatusProcessing — идёт вызов агента (Agent.Process).
	ThreadStatusProcessing ThreadStatus = "processing"
	// ThreadStatusClarify — агент запросил уточнение (needs_clarification),
	// это не ошибка: кредит не возвращается, тред ждёт следующее сообщение
	// пользователя (zan-backend-tz-v3.md §5.2).
	ThreadStatusClarify ThreadStatus = "clarify"
	// ThreadStatusDone — агент дал финальный ответ. Не терминален: новое
	// сообщение в пределах free_until переводит обратно в Processing (§4.3).
	ThreadStatusDone ThreadStatus = "done"
	// ThreadStatusError — обработка не удалась (таймаут/невалидный JSON/
	// отказ модерации), кредит услуги возвращён на баланс. Терминален.
	ThreadStatusError ThreadStatus = "error"
	// ThreadStatusCanceled — отменён пользователем до оплаты или во время
	// обработки (zan-backend-tz-v2.md §3.2). Терминален.
	ThreadStatusCanceled ThreadStatus = "canceled"
)

// ParseThreadStatus валидирует сырое значение (из запроса/БД) как ThreadStatus.
func ParseThreadStatus(s string) (ThreadStatus, error) {
	switch v := ThreadStatus(s); v {
	case ThreadStatusQueued, ThreadStatusProcessing, ThreadStatusClarify,
		ThreadStatusDone, ThreadStatusError, ThreadStatusCanceled:
		return v, nil
	default:
		return "", fmt.Errorf("domain: invalid thread status %q", s)
	}
}

// threadTransitions — единственная таблица допустимых переходов
// статус-машины (zan-backend-tz-v2.md §2.6, §3.2, §4.3;
// zan-backend-tz-v3.md §5.2). Terminal-статусы (Error, Canceled) сюда не
// добавляются — отсутствие записи означает "переходов из этого статуса нет".
//
//   - Queued -> Processing: баланс списан (или уже был is_paid=true),
//     агент стартует.
//   - Queued -> Canceled: отмена до оплаты (§3.2 — "до оплаты").
//   - Processing -> Done/Clarify/Error: результат Agent.Process.
//   - Processing -> Canceled: отмена во время обработки (§3.2 — "во время
//     обработки").
//   - Clarify -> Processing: пользователь ответил на уточнение.
//   - Done -> Processing: новое сообщение внутри free_until (§4.3) —
//     повторное бесплатное уточнение уже отвеченного треда.
var threadTransitions = map[ThreadStatus]map[ThreadStatus]bool{
	ThreadStatusQueued: {
		ThreadStatusProcessing: true,
		ThreadStatusCanceled:   true,
	},
	ThreadStatusProcessing: {
		ThreadStatusDone:     true,
		ThreadStatusClarify:  true,
		ThreadStatusError:    true,
		ThreadStatusCanceled: true,
	},
	ThreadStatusClarify: {
		ThreadStatusProcessing: true,
	},
	ThreadStatusDone: {
		ThreadStatusProcessing: true,
	},
}

// CanTransitionTo — допустим ли переход s -> target по статус-машине треда.
func (s ThreadStatus) CanTransitionTo(target ThreadStatus) bool {
	return threadTransitions[s][target]
}
