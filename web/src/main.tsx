import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import "@fontsource-variable/inter";
import "@fontsource-variable/geist-mono";
import "@fontsource-variable/noto-sans-sc";
import "./styles.css";
import { App } from "./App";
import { initTheme } from "./theme";
import { initI18n } from "./i18n";

initTheme();
initI18n();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>,
);
