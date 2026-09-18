import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
import { useGame } from "./store/game";
import { useAuth } from "./store/auth";
import "./index.css";

// Dev/test hook: lets the Playwright suite (and you, in the console) read
// the live stores without going through the DOM. Stripped from prod builds.
if (import.meta.env.DEV) {
  (window as unknown as { __monopsony: unknown }).__monopsony = { useGame, useAuth };
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </React.StrictMode>,
);
