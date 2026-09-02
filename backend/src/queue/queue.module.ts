import { Module } from '@nestjs/common';
import { BullModule } from '@nestjs/bullmq';
import { OrchestratorModule } from '../orchestrator/orchestrator.module.js';
import { REQUEST_PROCESSING_QUEUE } from './queue.constants.js';
import { RequestProcessingProcessor } from './request-processing.processor.js';

@Module({
  imports: [
    BullModule.registerQueue({
      name: REQUEST_PROCESSING_QUEUE,
      defaultJobOptions: {
        // Джобы не удалялись по завершении вовсе — при заметном трафике Redis рос бы
        // неограниченно только под историю уже обработанных задач (см. request.id уже
        // персистится в Postgres — BullMQ не нужен как журнал).
        removeOnComplete: { age: 24 * 60 * 60, count: 1000 },
        removeOnFail: { age: 7 * 24 * 60 * 60 },
      },
    }),
    OrchestratorModule,
  ],
  providers: [RequestProcessingProcessor],
})
export class QueueModule {}
