import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./app/App";
import "./shared/styles/index.css";

const rootElement = document.getElementById("root");
if (!rootElement) {
  throw new Error("Root element #root не найден");
}

createRoot(rootElement).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
