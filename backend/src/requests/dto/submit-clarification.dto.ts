import { Transform } from 'class-transformer';
import { IsString, MaxLength, MinLength } from 'class-validator';
import { trimField } from '../../common/validate-dto.js';

export class SubmitClarificationDto {
  @Transform(trimField)
  @IsString()
  @MinLength(1, { message: 'Ответ не может быть пустым' })
  @MaxLength(1000, { message: 'Слишком длинный ответ (максимум 1000 символов)' })
  answer!: string;
}
