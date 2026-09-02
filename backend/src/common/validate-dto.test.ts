import { describe, expect, it } from 'vitest';
import { BadRequestException } from '@nestjs/common';
import { validateDto } from './validate-dto.js';
import { CreateRequestDto } from '../requests/dto/create-request.dto.js';

describe('validateDto', () => {
  it('пропускает валидные данные', async () => {
    const dto = await validateDto(CreateRequestDto, {
      queryText: 'Достаточно длинный текст вопроса',
    });
    expect(dto.queryText).toBe('Достаточно длинный текст вопроса');
  });

  it('отклоняет слишком короткий queryText', async () => {
    await expect(validateDto(CreateRequestDto, { queryText: 'коротко' })).rejects.toBeInstanceOf(
      BadRequestException,
    );
  });

  it('отклоняет лишние поля (forbidNonWhitelisted)', async () => {
    await expect(
      validateDto(CreateRequestDto, {
        queryText: 'Достаточно длинный текст вопроса',
        hackerField: 'x',
      }),
    ).rejects.toBeInstanceOf(BadRequestException);
  });

  it('отклоняет отсутствие обязательного поля', async () => {
    await expect(validateDto(CreateRequestDto, {})).rejects.toBeInstanceOf(BadRequestException);
  });
});
