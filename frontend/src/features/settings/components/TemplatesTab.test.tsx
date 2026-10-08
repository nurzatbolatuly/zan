import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { TemplatesTab } from "./TemplatesTab";
import { ConfirmModal } from "@/shared/ui/ConfirmModal";
import { ToastViewport } from "@/shared/ui/toast/ToastViewport";
import { useLangStore } from "@/shared/stores/useLangStore";
import { createTestQueryClient } from "@/test/queryClient";
import type { DocumentTemplateDto, DocumentTypeDto } from "@/shared/types/api";

vi.mock("@/shared/lib/api", async () => {
  const actual =
    await vi.importActual<typeof import("@/shared/lib/api")>("@/shared/lib/api");
  return {
    ...actual,
    api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), patch: vi.fn(), delete: vi.fn() },
  };
});
import { api, ApiError } from "@/shared/lib/api";

const TYPES: DocumentTypeDto[] = [
  { id: "contract", name: "Договор", updated_at: "2026-10-01T10:00:00Z" },
  { id: "order", name: "Приказ", updated_at: "2026-10-01T10:00:00Z" },
];

function makeTemplate(overrides: Partial<DocumentTemplateDto>): DocumentTemplateDto {
  return {
    id: "tpl-1",
    document_type_id: "contract",
    title: "Договор аренды",
    original_name: "lease.docx",
    mime_type: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    size_bytes: 20480,
    preview_url: "https://storage.example/converted/lease.pdf",
    created_at: "2026-10-05T10:00:00Z",
    updated_at: "2026-10-05T10:00:00Z",
    ...overrides,
  };
}

let templates: DocumentTemplateDto[] = [];

function mockApi() {
  vi.mocked(api.get).mockImplementation((path: string) => {
    if (path === "/admin/document-types") return Promise.resolve(TYPES);
    if (path === "/admin/document-templates") return Promise.resolve(templates);
    throw new Error(`unexpected GET ${path}`);
  });
}

function renderTab() {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <TemplatesTab />
      <ConfirmModal />
      <ToastViewport />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.stubEnv("VITE_FILE_MAX_SIZE_BYTES", String(15 * 1024 * 1024));
});

afterEach(() => {
  vi.unstubAllEnvs();
  useLangStore.setState({ lang: "ru" });
  vi.mocked(api.get).mockReset();
  vi.mocked(api.post).mockReset();
  vi.mocked(api.delete).mockReset();
});

describe("TemplatesTab", () => {
  it("группирует шаблоны по типу документа и показывает число шаблонов у типа", async () => {
    templates = [
      makeTemplate({ id: "t1", title: "Договор аренды" }),
      makeTemplate({ id: "t2", title: "Приказ об отпуске", document_type_id: "order" }),
    ];
    mockApi();
    renderTab();

    const contractGroup = await screen.findByRole("region", { name: "Договор" });
    expect(within(contractGroup).getByText("Договор аренды")).toBeInTheDocument();
    expect(
      within(screen.getByRole("region", { name: "Приказ" })).getByText(
        "Приказ об отпуске",
      ),
    ).toBeInTheDocument();
    expect(screen.getAllByText("1 шаблон")).toHaveLength(2);
  });

  it("открывает PDF-версию шаблона на просмотр", async () => {
    templates = [makeTemplate({})];
    mockApi();
    const user = userEvent.setup();
    renderTab();

    await user.click(
      await screen.findByRole("button", { name: "Открыть: Договор аренды" }),
    );

    const dialog = screen.getByRole("dialog", { name: "Договор аренды" });
    expect(within(dialog).getByTitle("Договор аренды")).toHaveAttribute(
      "src",
      "https://storage.example/converted/lease.pdf",
    );
  });

  it("загружает файл с названием из имени файла и выбранным типом", async () => {
    templates = [];
    mockApi();
    vi.mocked(api.post).mockResolvedValue(makeTemplate({}));
    const user = userEvent.setup();
    renderTab();

    await user.click(await screen.findByRole("button", { name: "Загрузить шаблон" }));
    const dialog = screen.getByRole("dialog", { name: "Новый шаблон" });
    const file = new File(["PK"], "Договор поставки.docx", {
      type: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    });
    await user.upload(within(dialog).getByLabelText("ФАЙЛ"), file);
    await user.selectOptions(within(dialog).getByRole("combobox"), "contract");
    await user.click(within(dialog).getByRole("button", { name: "Сохранить" }));

    await waitFor(() => expect(api.post).toHaveBeenCalledTimes(1));
    const [path, body] = vi.mocked(api.post).mock.calls[0]!;
    expect(path).toBe("/admin/document-templates");
    const form = body as FormData;
    expect(form.get("title")).toBe("Договор поставки");
    expect(form.get("document_type_id")).toBe("contract");
    expect((form.get("file") as File).name).toBe("Договор поставки.docx");
    expect(await screen.findByText("Шаблон сохранён")).toBeInTheDocument();
  });

  it("не отправляет файл неподдерживаемого формата", async () => {
    templates = [];
    mockApi();
    const user = userEvent.setup({ applyAccept: false });
    renderTab();

    await user.click(await screen.findByRole("button", { name: "Загрузить шаблон" }));
    const dialog = screen.getByRole("dialog", { name: "Новый шаблон" });
    await user.upload(
      within(dialog).getByLabelText("ФАЙЛ"),
      new File(["x"], "scan.png", { type: "image/png" }),
    );
    await user.selectOptions(within(dialog).getByRole("combobox"), "contract");
    await user.click(within(dialog).getByRole("button", { name: "Сохранить" }));

    expect(
      await within(dialog).findByText("Подходят только файлы PDF и DOCX"),
    ).toBeInTheDocument();
    expect(api.post).not.toHaveBeenCalled();
  });

  it("сообщает, что тип с шаблонами удалить нельзя", async () => {
    templates = [makeTemplate({})];
    mockApi();
    vi.mocked(api.delete).mockRejectedValue(
      new ApiError("conflict", 409, "trace", {
        code: "document_type_in_use",
        message: "in use",
      }),
    );
    const user = userEvent.setup();
    renderTab();

    await user.click(await screen.findByRole("button", { name: "Удалить: Договор" }));
    await user.click(
      within(screen.getByRole("dialog", { name: "Удалить тип документа?" })).getByRole(
        "button",
        { name: "Удалить" },
      ),
    );

    expect(
      await screen.findByText(
        "У этого типа есть шаблоны — сначала удалите их или перенесите в другой тип.",
      ),
    ).toBeInTheDocument();
    expect(api.delete).toHaveBeenCalledWith("/admin/document-types/contract", {
      admin: true,
    });
  });
});
