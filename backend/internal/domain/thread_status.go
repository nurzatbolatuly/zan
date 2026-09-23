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
	// ThreadStatusAwaitingPayment — последний вопрос треда сохранён, но не
	// оплачен: на балансе не было единицы услуги. Один вопрос = одна
	// консультация: тред рождается в этом статусе и возвращается в него с
	// каждым новым вопросом, после чего сразу пытается оплатиться с баланса
	// (thread.Service.activateFromBalance); не вышло — остаётся здесь и
	// виден в истории, пока пользователь не пополнит баланс и не
	// перезапустит вопрос или не задаст новый (zan-backend-tz-v2.md §4.2 п.2).
	ThreadStatusAwaitingPayment ThreadStatus = "awaiting_payment"
	// ThreadStatusProcessing — идёт вызов агента (Agent.Process).
	ThreadStatusProcessing ThreadStatus = "processing"
	// ThreadStatusDone — агент дал финальный ответ. Не терминален: новый
	// вопрос переводит тред в AwaitingPayment (оплата следующего раунда).
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
	case ThreadStatusAwaitingPayment, ThreadStatusProcessing, ThreadStatusDone,
		ThreadStatusError, ThreadStatusCanceled:
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
//   - AwaitingPayment -> Processing: единица услуги списана с баланса,
//     агент стартует.
//   - AwaitingPayment -> Canceled: отмена до оплаты (§3.2 — "до оплаты").
//   - Processing -> Done/Error: результат Agent.Process.
//   - Processing -> Canceled: отмена во время обработки (§3.2 — "во время
//     обработки").
//   - Done -> AwaitingPayment: новый вопрос в уже отвеченном треде —
//     отдельная консультация, оплачивается как первая.
var threadTransitions = map[ThreadStatus]map[ThreadStatus]bool{
	ThreadStatusAwaitingPayment: {
		ThreadStatusProcessing: true,
		ThreadStatusCanceled:   true,
	},
	ThreadStatusProcessing: {
		ThreadStatusDone:     true,
		ThreadStatusError:    true,
		ThreadStatusCanceled: true,
	},
	ThreadStatusDone: {
		ThreadStatusAwaitingPayment: true,
	},
}

// CanTransitionTo — допустим ли переход s -> target по статус-машине треда.
func (s ThreadStatus) CanTransitionTo(target ThreadStatus) bool {
	return threadTransitions[s][target]
}
