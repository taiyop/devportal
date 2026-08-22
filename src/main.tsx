import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import { TooltipProvider } from "@/components/ui/tooltip";
import { applyTheme, readTheme } from "./theme";
import "./styles.css";

applyTheme(readTheme());

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <TooltipProvider delay={200}>
      <App />
    </TooltipProvider>
  </React.StrictMode>,
);
