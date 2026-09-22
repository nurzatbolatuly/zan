import { Component } from "react";
import type { ErrorInfo, ReactNode } from "react";
import { logger } from "@/shared/lib/logger";
import { Button } from "@/shared/ui/Button";
import { useLangStore } from "@/shared/stores/useLangStore";
import { navDictionary } from "@/shared/locales/nav";

interface Props {
  children: ReactNode;
  /** Каким разделом упало — попадает в лог, чтобы не гадать (FRONT_CODING_STANDARDS.md §8). */
  scope: string;
}

interface State {
  hasError: boolean;
}

/**
 * Один ErrorBoundary на route (оборачивается в router.tsx), не один общий
 * на всё приложение — падение одной страницы не должно рушить остальные
 * (FRONT_CODING_STANDARDS.md §8).
 */
export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false };

  static getDerivedStateFromError(): State {
    return { hasError: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    logger.error({
      scope: this.props.scope,
      event: "render_crashed",
      data: { componentStack: info.componentStack },
      error,
    });
  }

  handleReset = (): void => {
    this.setState({ hasError: false });
  };

  render(): ReactNode {
    if (!this.state.hasError) return this.props.children;
    // Классовый компонент — не может звать хук `useLangStore(selector)`, читаем
    // текущее значение напрямую через getState() (тот же паттерн non-reactive чтения,
    // что и в useChatThread.ts/useTariffs.ts для сторов-модалок); экран ошибки рендерится
    // один раз за случай падения, переподписка на смену языка здесь не нужна.
    const t = navDictionary[useLangStore.getState().lang];
    return (
      <div className="flex flex-col items-center gap-4 px-4 py-16 text-center">
        <div className="text-h2 text-ink">{t.errorTitle}</div>
        <p className="max-w-sm text-body-sm text-muted">{t.errorBody}</p>
        <Button onClick={this.handleReset}>{t.errorRetry}</Button>
      </div>
    );
  }
}
