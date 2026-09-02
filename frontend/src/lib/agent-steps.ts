import type { AgentName } from './types';

/** Порядок шагов пайплайна — см. PLAN.md, Этапы 3–6. Названия — в словарях (i18n/dictionaries). */
export const PIPELINE_STEP_ORDER: readonly AgentName[] = [
  'search',
  'verification',
  'editor',
  'document',
];
