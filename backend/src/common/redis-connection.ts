/** Разбор REDIS_URL в опции подключения ioredis — общее для BullModule.forRoot (app.module.ts)
 *  и стартовой проверки доступности Redis (startup-checks.ts), чтобы оба места видели один и
 *  тот же адрес одинаково. */
export function parseRedisConnection(redisUrl: string): {
  host: string;
  port: number;
  password?: string;
} {
  const url = new URL(redisUrl);
  return {
    host: url.hostname,
    port: Number(url.port || 6379),
    ...(url.password ? { password: url.password } : {}),
  };
}
