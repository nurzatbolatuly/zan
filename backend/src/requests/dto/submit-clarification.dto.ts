import { Transform } from 'class-transformer';
import { IsString, MaxLength, MinLength } from 'class-validator';

function trim({ value }: { value: unknown }): unknown {
  return typeof value === 'string' ? value.trim() : value;
}

export class SubmitClarificationDto {
  @Transform(trim)
  @IsString()
  @MinLength(1, { message: 'Ответ не может быть пустым' })
  @MaxLength(1000, { message: 'Слишком длинный ответ (максимум 1000 символов)' })
  answer!: string;
}
