package domain

// AnalyticsOverview — агрегаты GET /admin/analytics/overview
// (zan-backend-tz-v2.md §3.8, BACKEND_PLAN.md Stage 7): "total_threads,
// avg_processing_time_sec, satisfaction_rate (доля like среди оценённых
// сообщений), разбивка тредов по статусам". Только своя БД (`core`), без
// внешних систем — считается на лету на каждый запрос (то же решение, что
// и `AgentPrompt`: без долгого кеша, см. internal/service/analytics).
//
// AvgProcessingTimeSec/SatisfactionRate — nullable, не 0: на пустой/свежей
// БД (ни одного отвеченного сообщения, ни одной оценки) отношение
// 0-к-0 не имеет смысла как метрика и не должно маскироваться под "всё
// идеально" (0.0) или "всё плохо" (тоже 0.0): explicit nil вместо
// санитайзинг-нуля.
type AnalyticsOverview struct {
	TotalThreads         int
	StatusBreakdown      map[ThreadStatus]int
	AvgProcessingTimeSec *float64
	SatisfactionRate     *float64
}
