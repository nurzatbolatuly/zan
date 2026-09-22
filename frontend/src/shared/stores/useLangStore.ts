import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { Lang } from "@/shared/types/common";

interface LangState {
  lang: Lang;
  setLang: (lang: Lang) => void;
  toggleLang: () => void;
}

export const useLangStore = create<LangState>()(
  persist(
    (set, get) => ({
      lang: "ru",
      setLang: (lang) => set({ lang }),
      toggleLang: () => set({ lang: get().lang === "ru" ? "kz" : "ru" }),
    }),
    { name: "zan.lang" },
  ),
);
