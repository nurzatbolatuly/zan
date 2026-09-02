import { Transform } from 'class-transformer';
import { IsBoolean, IsOptional, IsString, MaxLength, MinLength } from 'class-validator';

/**
 * Обрезаем пробелы до валидации длины — иначе MinLength(10) пропускает строку из одних
 * пробелов ("          "), а сама строка с пробелами по краям бесполезна для агентов и БД.
 */
function trim({ value }: { value: unknown }): unknown {
  return typeof value === 'string' ? value.trim() : value;
}

export class CreateRequestDto {
  @Transform(trim)
  @IsString()
  @MinLength(10, { message: 'Опишите проблему подробнее (минимум 10 символов)' })
  @MaxLength(4000, { message: 'Слишком длинный запрос (максимум 4000 символов)' })
  queryText!: string;

  @IsOptional()
  @IsBoolean()
  includeDocument?: boolean;

  @IsOptional()
  @Transform(trim)
  @IsString()
  @MaxLength(200)
  documentType?: string;
}
