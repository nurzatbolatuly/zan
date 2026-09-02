import { describe, expect, it } from 'vitest';
import { MetricsService } from './metrics.service.js';

describe('MetricsService', () => {
  it('экспонирует задержку LLM-вызова с лейблами model/operation/outcome', async () => {
    const metrics = new MetricsService();

    metrics.observeLlmCall('gpt-5.6-sol', 'chat', 'success', 4321);
    metrics.observeLlmCall('gpt-5.6-sol', 'responses', 'error', 1234);

    const text = await metrics.exposition();

    expect(text).toContain('zan_llm_request_duration_ms');
    expect(text).toContain('model="gpt-5.6-sol"');
    expect(text).toContain('operation="chat"');
    expect(text).toContain('outcome="success"');
    expect(text).toContain('zan_llm_request_total');
  });
});
