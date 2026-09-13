import { BadRequestException } from '@nestjs/common';
import { plainToInstance } from 'class-transformer';
import { validate } from 'class-validator';

/**
 * `@Transform(trimField)` для строковых полей DTO — общий для всех DTO пакета (см.
 * CreateRequestDto/CreateDocumentDto/SubmitClarificationDto), раньше был скопирован по одной
 * копии на файл. Обрезаем пробелы до валидации длины — иначе MinLength пропускает строку из
 * одних пробелов, а сама строка с пробелами по краям бесполезна для агентов и БД.
 */
export function trimField({ value }: { value: unknown }): unknown {
  return typeof value === 'string' ? value.trim() : value;
}

/**
 * ВАЖНО: не полагаемся на Nest'овский глобальный ValidationPipe с автоматическим выводом
 * типа DTO из параметра контроллера — это требует TypeScript emitDecoratorMetadata
 * (design:paramtypes), а мы запускаем backend через tsx (esbuild), который эту метадату
 * не эмитит. Nest в этом случае молча пропускает валидацию (metatype === undefined),
 * а не падает с ошибкой — поэтому каждый контроллер вызывает validateDto() явно.
 * См. также правило про @Inject() в src/orchestrator/orchestrator.service.ts.
 */
export async function validateDto<T extends object>(
  dtoClass: new () => T,
  plain: unknown,
): Promise<T> {
  const instance = plainToInstance(dtoClass, plain ?? {});
  const errors = await validate(instance, { whitelist: true, forbidNonWhitelisted: true });

  if (errors.length > 0) {
    const messages = errors.flatMap((error) => Object.values(error.constraints ?? {}));
    throw new BadRequestException(messages.length > 0 ? messages : 'Некорректные данные запроса');
  }

  return instance;
}
