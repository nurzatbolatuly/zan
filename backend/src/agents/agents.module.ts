import { Module } from '@nestjs/common';
import { ANSWER_AGENT, DOCUMENT_AGENT } from './agent.types.js';
import { LawAnswerAgent } from './answer/law-answer.agent.js';
import { LawDocumentAgent } from './document/law-document.agent.js';

/**
 * Единственное место, где реализации агентов привязываются к DI-токенам — точка подмены
 * реализации на будущее (например, для A/B-тестирования промптов) без изменений в оркестраторе.
 */
@Module({
  providers: [
    { provide: ANSWER_AGENT, useClass: LawAnswerAgent },
    { provide: DOCUMENT_AGENT, useClass: LawDocumentAgent },
  ],
  exports: [ANSWER_AGENT, DOCUMENT_AGENT],
})
export class AgentsModule {}
