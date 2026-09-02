import { createHash, randomBytes } from 'node:crypto';
import type { Request, Response } from 'express';
import { config } from '../config.js';

/**
 * Разграничение прав доступа (Этап 8): полноценных аккаунтов ещё нет (Этап 13 — личный
 * кабинет), поэтому владение запросом привязано к анонимной сессии — случайному токену в
 * httpOnly-cookie браузера. На сервере хранится только SHA-256 хэш токена (requests.owner_token_hash),
 * сам токен никогда не логируется и не попадает в БД в открытом виде.
 */
export const SESSION_COOKIE_NAME = 'zan_session';
const SESSION_MAX_AGE_MS = 365 * 24 * 60 * 60 * 1000;

export function hashSessionToken(token: string): string {
  return createHash('sha256').update(token).digest('hex');
}

/**
 * Возвращает хэш текущего токена сессии, выставляя новую cookie в ответ, если её ещё не было
 * (например, при первом запросе анонимного посетителя).
 */
export function ensureSessionTokenHash(req: Request, res: Response): string {
  const existingToken = req.cookies?.[SESSION_COOKIE_NAME] as string | undefined;
  const token =
    existingToken && existingToken.length >= 32 ? existingToken : randomBytes(32).toString('hex');

  if (token !== existingToken) {
    // sameSite: 'none' в проде — фронт (Vercel) и бэк (Render/…) живут на разных доменах,
    // это кросс-сайтовый запрос с точки зрения браузера. 'none' обязательно требует secure:
    // true, иначе браузер молча отбросит cookie. Локально фронт и бэк оба на localhost
    // (разные порты — тот же сайт), там 'lax' + secure: false работают как обычно.
    res.cookie(SESSION_COOKIE_NAME, token, {
      httpOnly: true,
      sameSite: config.nodeEnv === 'production' ? 'none' : 'lax',
      secure: config.nodeEnv === 'production',
      maxAge: SESSION_MAX_AGE_MS,
      path: '/',
    });
  }

  return hashSessionToken(token);
}

/** Для чтения (GET) — не создаёт новую сессию, просто хэширует то, что уже есть в cookie. */
export function readSessionTokenHash(req: Request): string | null {
  const token = req.cookies?.[SESSION_COOKIE_NAME] as string | undefined;
  return token ? hashSessionToken(token) : null;
}
