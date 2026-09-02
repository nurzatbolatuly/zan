import { Global, Module } from '@nestjs/common';
import { createAdminClient } from '@supabase/server/core';
import { config } from '../config.js';
import { SUPABASE_ADMIN } from './db.constants.js';

/**
 * Единственное место, где создаётся Supabase admin client. Инжектируется через токен
 * SUPABASE_ADMIN, а не через конкретный класс — репозитории зависят от токена, а не от
 * способа подключения к БД. @Global(), потому что подключение к БД — сквозная
 * инфраструктурная зависимость для всех будущих модулей, а не только для requests.
 *
 * Admin-клиент работает через secret key поверх PostgREST (HTTPS) и обходит RLS — как раньше
 * pg.Pool с прямым подключением давал полный доступ без ограничений. Прямое подключение к
 * db.<project>.supabase.co больше не используется бэкендом (только миграциями в ingestion/).
 */
const supabaseAdmin = createAdminClient({
  env: { url: config.supabaseUrl, secretKeys: { default: config.supabaseSecretKey } },
});

@Global()
@Module({
  providers: [{ provide: SUPABASE_ADMIN, useValue: supabaseAdmin }],
  exports: [SUPABASE_ADMIN],
})
export class DbModule {}
