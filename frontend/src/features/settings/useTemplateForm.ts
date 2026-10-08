import { useMemo } from "react";
import { useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { getFileMaxSizeBytes } from "@/shared/lib/env";
import { formatFileSize } from "@/shared/lib/format";
import {
  TEMPLATE_TITLE_MAX_LENGTH,
  isSupportedTemplateFile,
  titleFromFileName,
} from "./templateFiles";
import type { DocumentTemplate, TemplateFormResult } from "./types";

export interface TemplateFormValues {
  title: string;
  typeId: string;
  /** `null` при изменении — файл остаётся прежним. */
  file: File | null;
}

interface TemplateFormMessages {
  titleRequiredError: string;
  typeRequiredError: string;
  fileRequiredError: string;
  unsupportedFileError: string;
  fileTooLargeError: (maxSize: string) => string;
}

interface UseTemplateFormArgs {
  initialTemplate: DocumentTemplate | null;
  messages: TemplateFormMessages;
  onSave: (values: TemplateFormResult) => void;
}

/**
 * Форма `TemplateEditModal` — react-hook-form + zod. Новый шаблон требует
 * файл, при изменении файл необязателен (замена). Формат и размер
 * проверяются до отправки — те же правила, что у бэка, чтобы не гонять
 * заведомо отклонённый файл по сети.
 */
export function useTemplateForm({
  initialTemplate,
  messages,
  onSave,
}: UseTemplateFormArgs) {
  const isCreate = initialTemplate === null;
  const maxSizeBytes = getFileMaxSizeBytes();
  const maxSizeLabel = formatFileSize(maxSizeBytes);

  const schema = useMemo(
    () =>
      z
        .object({
          title: z
            .string()
            .trim()
            .min(1, messages.titleRequiredError)
            .max(TEMPLATE_TITLE_MAX_LENGTH),
          typeId: z.string().min(1, messages.typeRequiredError),
          file: z.instanceof(File).nullable(),
        })
        .superRefine(({ file }, ctx) => {
          if (file === null) {
            if (isCreate) {
              ctx.addIssue({
                code: "custom",
                path: ["file"],
                message: messages.fileRequiredError,
              });
            }
            return;
          }
          if (!isSupportedTemplateFile(file.name)) {
            ctx.addIssue({
              code: "custom",
              path: ["file"],
              message: messages.unsupportedFileError,
            });
          } else if (file.size > maxSizeBytes) {
            ctx.addIssue({
              code: "custom",
              path: ["file"],
              message: messages.fileTooLargeError(maxSizeLabel),
            });
          }
        }),
    [isCreate, maxSizeBytes, maxSizeLabel, messages],
  );

  const form = useForm<TemplateFormValues>({
    resolver: zodResolver(schema),
    // Модалка монтируется заново на каждое открытие (`key`), defaultValues
    // читаются один раз.
    defaultValues: {
      title: initialTemplate?.title ?? "",
      typeId: initialTemplate?.typeId ?? "",
      file: null,
    },
  });
  const selectedFile = useWatch({ control: form.control, name: "file" });

  /** Выбор файла; пустое название подставляется из имени файла. */
  function selectFile(file: File | null) {
    form.setValue("file", file, { shouldDirty: true, shouldValidate: true });
    if (file && form.getValues("title").trim() === "") {
      form.setValue("title", titleFromFileName(file.name), { shouldValidate: true });
    }
  }

  function submit(values: TemplateFormValues) {
    onSave({ typeId: values.typeId, title: values.title.trim(), file: values.file });
  }

  return {
    register: form.register,
    errors: form.formState.errors,
    selectedFile,
    selectFile,
    maxSizeLabel,
    isCreate,
    submitForm: form.handleSubmit(submit),
  };
}
