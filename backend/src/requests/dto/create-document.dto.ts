import { Transform } from 'class-transformer';
import { IsString, MaxLength, MinLength } from 'class-validator';
import { trimField } from '../../common/validate-dto.js';

/**
 * Этап 18: догенерация документа к уже завершённому ответу (см. RequestsService.requestDocument) —
 * пользователь решает приложить документ ПОСЛЕ того, как увидел ответ, а не только в момент
 * отправки вопроса (см. CreateRequestDto). Отдельный DTO, а не переиспользование
 * CreateRequestDto.documentType, — здесь единственное поле, queryText/includeDocument берутся из
 * уже существующего запроса, не с клиента.
 */
export class CreateDocumentDto {
  @Transform(trimField)
  @IsString()
  @MinLength(1, { message: 'Укажите тип документа' })
  @MaxLength(200, { message: 'Слишком длинное название типа документа (максимум 200 символов)' })
  documentType!: string;
}
