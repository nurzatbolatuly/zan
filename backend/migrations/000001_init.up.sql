-- Stage 0 (BACKEND_PLAN.md): пустая миграция — проверяет, что пайплайн
-- golang-migrate (CLI, не встроен в бинарь — см. BACKEND_PLAN.md §1.1,
-- "Отдельный бинарь/CLI, не завязан на рантайм приложения") работает
-- end-to-end (make migrate, docker-compose, CI) ещё до того, как Stage 1
-- принесёт первую реальную схему (Session/Service/Tariff/UserCredit/
-- Payment/Thread/Message/FileAttachment/AgentPrompt, см. v2 §2/v3 §3).
SELECT 1;
