import { config as loadDotenv } from 'dotenv';
import { fileURLToPath } from 'node:url';

// Единый .env лежит в корне монорепозитория — см. frontend/next.config.mjs (тот же приём).
loadDotenv({ path: fileURLToPath(new URL('../../.env', import.meta.url)), quiet: true });

function requireEnv(name: string): string {
  const value = process.env[name];
  if (!value) {
    throw new Error(`Отсутствует обязательная переменная окружения: ${name}`);
  }
  return value;
}

export const config = {
  nodeEnv: process.env.NODE_ENV ?? 'development',
  port: Number(process.env.PORT ?? 3000),
  supabaseUrl: requireEnv('SUPABASE_URL'),
  supabaseSecretKey: requireEnv('SUPABASE_SECRET_KEY'),
  redisUrl: requireEnv('REDIS_URL'),
  openAiApiKey: requireEnv('OPENAI_API_KEY'),
  /** Origin фронтенда (Этап 7) для CORS — Next.js dev-сервер работает на другом порту. */
  frontendOrigin: process.env.FRONTEND_ORIGIN ?? 'http://localhost:3001',
  logLevel: process.env.LOG_LEVEL ?? 'info',
} as const;
