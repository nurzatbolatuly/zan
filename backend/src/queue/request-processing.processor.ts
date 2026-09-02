import { Inject } from '@nestjs/common';
import { Processor, WorkerHost } from '@nestjs/bullmq';
import type { Job } from 'bullmq';
import { createLogger, type Logger } from '@zan/shared';
import { OrchestratorService } from '../orchestrator/orchestrator.service.js';
import { REQUEST_PROCESSING_QUEUE, type RequestProcessingJobData } from './queue.constants.js';

/**
 * Без concurrency BullMQ по умолчанию берёт 1 — один медленный запрос занимает единственный
 * слот воркера, и все остальные джобы в очереди ждут его молча, без ошибки (см. также
 * REQUEST_TIMEOUT_MS в shared/src/llm/chatClient.ts, который ограничивает срок этого
 * ожидания). 5 — с запасом под rate limit OpenAI-тарифа, пересмотреть при апгрейде тарифа.
 */
const WORKER_CONCURRENCY = 5;

@Processor(REQUEST_PROCESSING_QUEUE, { concurrency: WORKER_CONCURRENCY })
export class RequestProcessingProcessor extends WorkerHost {
  private readonly logger: Logger = createLogger('queue-worker');

  constructor(@Inject(OrchestratorService) private readonly orchestrator: OrchestratorService) {
    super();
  }

  async process(job: Job<RequestProcessingJobData>): Promise<void> {
    this.logger.info({ jobId: job.id, requestId: job.data.requestId }, 'Взял задачу в обработку');
    await this.orchestrator.processRequest(job.data.requestId);
  }
}
