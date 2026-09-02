import { Module } from '@nestjs/common';
import { SEARCH_AGENT, VERIFICATION_AGENT, EDITOR_AGENT, DOCUMENT_AGENT } from './agent.types.js';
import { LawSearchAgent } from './search/law-search.agent.js';
import { LawVerificationAgent } from './verification/law-verification.agent.js';
import { LawEditorAgent } from './editor/law-editor.agent.js';
import { LawDocumentAgent } from './document/law-document.agent.js';

/**
 * Единственное место, где реализации агентов привязываются к DI-токенам — точка подмены
 * реализации на будущее (например, для A/B-тестирования промптов) без изменений в оркестраторе.
 */
@Module({
  providers: [
    { provide: SEARCH_AGENT, useClass: LawSearchAgent },
    { provide: VERIFICATION_AGENT, useClass: LawVerificationAgent },
    { provide: EDITOR_AGENT, useClass: LawEditorAgent },
    { provide: DOCUMENT_AGENT, useClass: LawDocumentAgent },
  ],
  exports: [SEARCH_AGENT, VERIFICATION_AGENT, EDITOR_AGENT, DOCUMENT_AGENT],
})
export class AgentsModule {}
