import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import App from "./App";
import { EventsProvider } from "@/lib/events";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <EventsProvider>
      <App />
    </EventsProvider>
  </StrictMode>,
);
