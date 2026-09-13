import ReactMarkdown, { type Components } from 'react-markdown';

// react-markdown по умолчанию не рендерит сырой HTML и сам отсекает опасные URL-схемы
// (javascript: и т.п.) — этого достаточно от XSS. rel="noopener noreferrer" — отдельная,
// не связанная с XSS гигиена: без него target="_blank" даёт открытой вкладке доступ
// к window.opener (родительской странице) через window.opener.location.
const components: Components = {
  a: ({ href, children, ...props }) => (
    <a href={href} target="_blank" rel="noopener noreferrer" {...props}>
      {children}
    </a>
  ),
};

/** Агент "answer" отдаёт markdown с заголовками и ссылками на статьи закона (см. backend/src/agents/answer/). */
export function MarkdownAnswer({ markdown }: { markdown: string }) {
  return (
    <div className="prose prose-slate prose-sm max-w-none prose-a:text-blue-700 prose-a:no-underline hover:prose-a:underline">
      <ReactMarkdown components={components}>{markdown}</ReactMarkdown>
    </div>
  );
}
