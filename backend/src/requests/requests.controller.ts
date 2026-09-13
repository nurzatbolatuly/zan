import {
  Body,
  Controller,
  Get,
  Inject,
  Param,
  ParseUUIDPipe,
  Post,
  Req,
  Res,
} from '@nestjs/common';
import { Throttle } from '@nestjs/throttler';
import type { Request, Response } from 'express';
import { RequestsService } from './requests.service.js';
import { CreateRequestDto } from './dto/create-request.dto.js';
import { CreateDocumentDto } from './dto/create-document.dto.js';
import { SubmitClarificationDto } from './dto/submit-clarification.dto.js';
import { validateDto } from '../common/validate-dto.js';
import { ensureSessionTokenHash, readSessionTokenHash } from '../common/session.js';

@Controller('requests')
export class RequestsController {
  constructor(@Inject(RequestsService) private readonly service: RequestsService) {}

  // Отдельный, более жёсткий лимит, чем глобальный ThrottlerGuard (см. app.module.ts):
  // каждый POST запускает пайплайн (Агент "answer" + опционально "document") = минимум 1-2
  // платных вызова OpenAI API.
  @Throttle({ default: { ttl: 60_000, limit: 5 } })
  @Post()
  async create(
    @Body() body: unknown,
    @Req() req: Request,
    @Res({ passthrough: true }) res: Response,
  ) {
    const dto = await validateDto(CreateRequestDto, body);
    const ownerTokenHash = ensureSessionTokenHash(req, res);
    const request = await this.service.create(dto, ownerTokenHash);
    return { id: request.id, status: request.status };
  }

  /** История обращений (Этап 13 — личный кабинет): запросы текущей анонимной сессии. */
  @Get()
  async listMine(@Req() req: Request) {
    return this.service.listMine(readSessionTokenHash(req));
  }

  @Get(':id')
  async getById(@Param('id', ParseUUIDPipe) id: string, @Req() req: Request) {
    return this.service.getById(id, readSessionTokenHash(req));
  }

  // Возобновляет пайплайн (снова вызывает агентов) — тот же лимит, что у создания запроса.
  @Throttle({ default: { ttl: 60_000, limit: 5 } })
  @Post(':id/clarification')
  async submitClarification(
    @Param('id', ParseUUIDPipe) id: string,
    @Body() body: unknown,
    @Req() req: Request,
  ) {
    const dto = await validateDto(SubmitClarificationDto, body);
    await this.service.submitClarification(id, dto, readSessionTokenHash(req));
    return { id, status: 'pending' };
  }

  // Этап 18 (§9.14): запускает пайплайн заново тем же способом, что и submitClarification, — тот
  // же лимит.
  @Throttle({ default: { ttl: 60_000, limit: 5 } })
  @Post(':id/document')
  async createDocument(
    @Param('id', ParseUUIDPipe) id: string,
    @Body() body: unknown,
    @Req() req: Request,
  ) {
    const dto = await validateDto(CreateDocumentDto, body);
    await this.service.requestDocument(id, dto, readSessionTokenHash(req));
    // Статус самого запроса не меняется (остаётся 'completed', см. RequestsService.requestDocument) —
    // прогресс генерации документа фронт отслеживает по request_steps через обычный GET/поллинг.
    return { id };
  }

  @Post(':id/cancel')
  async cancel(@Param('id', ParseUUIDPipe) id: string, @Req() req: Request) {
    await this.service.cancel(id, readSessionTokenHash(req));
    return { id, status: 'cancelled' };
  }
}
