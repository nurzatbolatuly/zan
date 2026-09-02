import { Module } from '@nestjs/common';
import { AgentsModule } from '../agents/agents.module.js';
import { RequestsModule } from '../requests/requests.module.js';
import { OrchestratorService } from './orchestrator.service.js';

@Module({
  imports: [AgentsModule, RequestsModule],
  providers: [OrchestratorService],
  exports: [OrchestratorService],
})
export class OrchestratorModule {}
