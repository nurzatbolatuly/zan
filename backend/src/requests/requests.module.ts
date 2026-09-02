import { Module } from '@nestjs/common';
import { BullModule } from '@nestjs/bullmq';
import { REQUEST_PROCESSING_QUEUE } from '../queue/queue.constants.js';
import { REQUESTS_REPOSITORY, PgRequestsRepository } from './requests.repository.js';
import { RequestsService } from './requests.service.js';
import { RequestsController } from './requests.controller.js';

@Module({
  imports: [BullModule.registerQueue({ name: REQUEST_PROCESSING_QUEUE })],
  controllers: [RequestsController],
  providers: [{ provide: REQUESTS_REPOSITORY, useClass: PgRequestsRepository }, RequestsService],
  exports: [REQUESTS_REPOSITORY],
})
export class RequestsModule {}
