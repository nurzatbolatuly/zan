import { Transform } from 'class-transformer';
import { IsBoolean, IsOptional, IsString, MaxLength, MinLength } from 'class-validator';
import { trimField } from '../../common/validate-dto.js';

/**
 * `includeDocument`/`documentType` — чекбокс "приложить документ" (Этап 17, §9.13): доступен и на
 * первом экране (RequestForm.tsx), и в композере продолжения чата (RequestStatusView.tsx), не
 * только на первом сообщении. Автоопределение по тексту (Этап 16) убрано — явный чекбокс дешевле
 * и точнее отдельного LLM-классификатора на каждое сообщение.
 */
export class CreateRequestDto {
  @Transform(trimField)
  @IsString()
  @MinLength(10, { message: 'Опишите проблему подробнее (минимум 10 символов)' })
  @MaxLength(4000, { message: 'Слишком длинный запрос (максимум 4000 символов)' })
  queryText!: string;

  @IsOptional()
  @IsBoolean()
  includeDocument?: boolean;

  @IsOptional()
  @Transform(trimField)
  @IsString()
  @MaxLength(200)
  documentType?: string;
}
