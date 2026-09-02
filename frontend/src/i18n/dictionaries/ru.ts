import type { Dictionary } from '../dictionary';

export const ru: Dictionary = {
  meta: {
    title: 'zan — юридический ассистент по законодательству РК',
    description:
      'Опишите правовую проблему и получите структурированную справку со ссылками на статьи законодательства Республики Казахстан.',
  },
  nav: {
    home: 'Главная',
    history: 'История',
    analytics: 'Аналитика',
    tagline: 'Юридический ассистент по законодательству Республики Казахстан',
  },
  footer: {
    disclaimer:
      'Сервис даёт информационную справку на основе закона. Перед принятием решений рекомендуем обратиться к квалифицированному юристу.',
  },
  home: {
    heading: 'Задайте вопрос по законодательству РК',
    subheading:
      'Опишите ситуацию своими словами — ассистент найдёт применимые нормы, проверит их и подготовит понятный ответ со ссылками на статьи закона.',
  },
  form: {
    queryLabel: 'Опишите вашу правовую проблему',
    queryPlaceholder: 'Например: меня уволили без предупреждения, законно ли это?',
    minLengthHint: (min) => `Минимум ${min} символов`,
    includeDocumentLabel: 'Подготовить готовый документ (заявление, претензия и т.д.)',
    documentTypeLabel: 'Какой документ нужен?',
    documentTypePlaceholder: 'Например: заявление о расторжении трудового договора',
    submit: 'Отправить запрос',
    submitting: 'Отправляем…',
    genericError: 'Не удалось отправить запрос. Попробуйте ещё раз.',
  },
  status: {
    yourQuestion: 'Ваш вопрос',
    pipelineProgress: 'Ход обработки',
    statusLabel: {
      pending: 'Запрос принят, ожидает обработки в очереди…',
      processing: 'Идёт обработка запроса…',
      needs_clarification: 'Ждём уточнения от вас',
      completed: 'Обработка завершена',
      failed: 'Обработка завершилась ошибкой',
      cancelled: 'Запрос отменён',
    },
    processingGeneric: 'Обрабатываю запрос…',
    waitingForReplyHint: 'Дождитесь ответа на текущий вопрос, чтобы отправить следующий.',
    elapsedLabel: (seconds) => `Прошло ${seconds} сек с момента отправки`,
    stalledWarning:
      'Обработка идёт заметно дольше обычного — возможны технические неполадки (например, недоступен внешний сервис). Можно подождать ещё немного или попробовать отправить запрос позже.',
    failedPrefix: 'Не удалось обработать запрос',
    answerHeading: 'Ответ',
    documentHeading: 'Документ',
    loading: 'Загружаем статус запроса…',
    loadError: 'Не удалось загрузить статус запроса.',
    clarificationHeading: 'Нужно уточнение',
    clarificationPlaceholder: 'Ваш ответ…',
    clarificationSubmit: 'Отправить уточнение',
    clarificationSubmitting: 'Отправляем…',
    clarificationError: 'Не удалось отправить уточнение. Попробуйте ещё раз.',
    cancelButton: 'Отменить запрос',
    cancelling: 'Отменяем…',
    cancelError: 'Не удалось отменить запрос. Попробуйте ещё раз.',
    cancelConfirm: 'Отменить обработку этого запроса? Это действие нельзя отменить.',
  },
  pipeline: {
    search: 'Поиск норм права',
    verification: 'Проверка достоверности',
    editor: 'Подготовка ответа',
    document: 'Формирование документа',
    reprocessingNotice:
      'Проверка нашла замечания к черновику ответа — Агент 1 дорабатывает ответ с их учётом. Это нормальная часть обработки, а не зависание.',
  },
  pipelineStepStatus: {
    pending: 'не начат',
    running: 'выполняется…',
    success: 'готово',
    failed: 'ошибка',
  },
  document: {
    download: (title) => `Скачать «${title}»`,
    downloadError:
      'Не удалось скачать документ — содержимое повреждено. Попробуйте обновить страницу.',
  },
  history: {
    heading: 'История обращений',
    empty: 'Пока нет ни одного запроса.',
    openLink: 'Открыть →',
    loading: 'Загружаем историю обращений…',
    loadError: 'Не удалось загрузить историю обращений.',
    statusLabel: {
      pending: 'В очереди',
      processing: 'Обрабатывается',
      needs_clarification: 'Ждём уточнения',
      completed: 'Готово',
      failed: 'Ошибка',
      cancelled: 'Отменён',
    },
    cancelButton: 'Отменить',
    cancelling: 'Отменяем…',
    cancelError: 'Не удалось отменить запрос.',
    cancelConfirm: 'Отменить обработку этого запроса? Это действие нельзя отменить.',
    processedInLabel: (seconds) => `Обработано за ${seconds} сек`,
  },
  analytics: {
    heading: 'Аналитика по обращениям',
    totalRequestsLabel: 'Всего запросов',
    byStatusHeading: 'Запросы по статусу',
    avgProcessingLabel: 'Среднее время обработки завершённых запросов',
    avgProcessingUnit: 'сек',
    loading: 'Загружаем аналитику…',
    loadError: 'Не удалось загрузить аналитику.',
    statusLabel: {
      pending: 'В очереди',
      processing: 'Обрабатывается',
      needs_clarification: 'Ждёт уточнения',
      completed: 'Готово',
      failed: 'Ошибка',
      cancelled: 'Отменён',
    },
  },
  localeSwitcher: { ru: 'Рус', kk: 'Қаз' },
};
