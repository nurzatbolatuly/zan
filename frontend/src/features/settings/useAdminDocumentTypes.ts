import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useLangStore } from "@/shared/stores/useLangStore";
import { useConfirmModalStore } from "@/shared/stores/useConfirmModalStore";
import { useToast } from "@/shared/ui";
import { logger } from "@/shared/lib/logger";
import { api } from "@/shared/lib/api";
import { describeApiError } from "@/shared/lib/apiErrorMessages";
import { settingsDictionary } from "./locales";
import type { DocumentType } from "./types";
import type { DocumentTypeDto, DocumentTypeWriteRequestDto } from "@/shared/types/api";

const DOCUMENT_TYPES_QUERY_KEY = ["admin", "document-types"] as const;

export type DocumentTypeModalState =
  { mode: "closed" } | { mode: "create" } | { mode: "rename"; type: DocumentType };

function mapDocumentType(dto: DocumentTypeDto): DocumentType {
  return { id: dto.id, name: dto.name };
}

/**
 * Settings → Шаблоны → справочник типов документов (`/admin/document-types`,
 * требует `X-Admin-Token`). Тип с шаблонами бэк удалить не даст
 * (`409 document_type_in_use`) — ошибка показывается тостом из ConfirmModal.
 */
export function useAdminDocumentTypes() {
  const lang = useLangStore((state) => state.lang);
  const t = settingsDictionary[lang];
  const toast = useToast();
  const openConfirm = useConfirmModalStore((state) => state.open);
  const queryClient = useQueryClient();

  const typesQuery = useQuery({
    queryKey: DOCUMENT_TYPES_QUERY_KEY,
    queryFn: () => api.get<DocumentTypeDto[]>("/admin/document-types", { admin: true }),
  });
  const types = useMemo(
    () => (typesQuery.data ?? []).map(mapDocumentType),
    [typesQuery.data],
  );

  const [typeModal, setTypeModal] = useState<DocumentTypeModalState>({ mode: "closed" });
  // Растёт при каждом открытии — `key` модалки, форма пересобирается с чистыми
  // значениями (тот же приём, что у редактора тарифа).
  const [typeModalKey, setTypeModalKey] = useState(0);
  const [isTypeModalPending, setTypeModalPending] = useState(false);

  function openModal(state: DocumentTypeModalState) {
    setTypeModalKey((key) => key + 1);
    setTypeModal(state);
  }

  function closeTypeModal() {
    setTypeModal({ mode: "closed" });
  }

  async function saveType(name: string) {
    const body: DocumentTypeWriteRequestDto = { name };
    setTypeModalPending(true);
    try {
      if (typeModal.mode === "create") {
        await api.post("/admin/document-types", body, { admin: true });
        logger.info({ scope: "settings.templates", event: "document_type_created" });
      } else if (typeModal.mode === "rename") {
        await api.put(`/admin/document-types/${typeModal.type.id}`, body, {
          admin: true,
        });
        logger.info({
          scope: "settings.templates",
          event: "document_type_renamed",
          data: { typeId: typeModal.type.id },
        });
      }
      await queryClient.invalidateQueries({ queryKey: DOCUMENT_TYPES_QUERY_KEY });
      toast(t.templates.typeSavedToast, "success");
      closeTypeModal();
    } catch (error) {
      logger.error({
        scope: "settings.templates",
        event: "document_type_save_failed",
        error,
      });
      toast(describeApiError(error, lang), "error");
    } finally {
      setTypeModalPending(false);
    }
  }

  function requestDeleteType(type: DocumentType) {
    logger.info({
      scope: "settings.templates",
      event: "document_type_delete_requested",
      data: { typeId: type.id },
    });
    openConfirm({
      title: t.templates.typeDeleteConfirmTitle,
      message: t.templates.typeDeleteConfirmMessage,
      confirmLabel: t.templates.deleteAction,
      cancelLabel: t.common.cancel,
      onConfirm: async () => {
        await api.delete(`/admin/document-types/${type.id}`, { admin: true });
        logger.info({
          scope: "settings.templates",
          event: "document_type_deleted",
          data: { typeId: type.id },
        });
        await queryClient.invalidateQueries({ queryKey: DOCUMENT_TYPES_QUERY_KEY });
        toast(t.templates.typeDeletedToast, "success");
      },
    });
  }

  return {
    types,
    isLoading: typesQuery.isLoading,
    isError: typesQuery.isError,
    retry: () => void typesQuery.refetch(),
    typeModal,
    typeModalKey,
    isTypeModalPending,
    openNewType: () => openModal({ mode: "create" }),
    openRenameType: (type: DocumentType) => openModal({ mode: "rename", type }),
    closeTypeModal,
    saveType,
    requestDeleteType,
  };
}
