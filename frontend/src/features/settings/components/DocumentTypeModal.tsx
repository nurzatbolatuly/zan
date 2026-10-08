import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Button, Input, Modal } from "@/shared/ui";
import type { TemplatesDictionary } from "../locales";
import type { DocumentType } from "../types";

/** Тот же лимит, что у бэка (`openapi.yaml#DocumentType.name`). */
const TYPE_NAME_MAX_LENGTH = 100;

interface DocumentTypeModalProps {
  open: boolean;
  /** `null` — новый тип, иначе переименование. */
  initialType: DocumentType | null;
  labels: TemplatesDictionary;
  saveLabel: string;
  cancelLabel: string;
  isPending: boolean;
  onSave: (name: string) => void;
  onClose: () => void;
}

/** Создание/переименование типа документа — одно поле, форма монтируется заново на каждое открытие (`key`). */
export function DocumentTypeModal({
  open,
  initialType,
  labels,
  saveLabel,
  cancelLabel,
  isPending,
  onSave,
  onClose,
}: DocumentTypeModalProps) {
  const title = initialType ? labels.renameTypeTitle : labels.newTypeTitle;
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<{ name: string }>({
    resolver: zodResolver(
      z.object({
        name: z
          .string()
          .trim()
          .min(1, labels.typeNameRequiredError)
          .max(TYPE_NAME_MAX_LENGTH),
      }),
    ),
    defaultValues: { name: initialType?.name ?? "" },
  });

  return (
    <Modal open={open} onClose={onClose} ariaLabel={title}>
      <div className="mb-4 text-h3 text-ink">{title}</div>
      <form
        onSubmit={handleSubmit(({ name }) => onSave(name.trim()))}
        className="flex flex-col gap-3.5"
      >
        <div>
          <Input
            label={labels.fTypeName}
            maxLength={TYPE_NAME_MAX_LENGTH}
            autoFocus
            {...register("name")}
          />
          {errors.name && (
            <p className="mt-1 text-caption text-danger">{errors.name.message}</p>
          )}
        </div>
        <div className="flex flex-col gap-2">
          <Button type="submit" disabled={isPending}>
            {isPending ? "…" : saveLabel}
          </Button>
          <Button type="button" variant="ghost" onClick={onClose} disabled={isPending}>
            {cancelLabel}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
