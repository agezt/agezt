import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import App from "./App";
import { EventsProvider } from "@/lib/events";
import { UIProvider } from "@/components/ui/feedback";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <UIProvider>
      <EventsProvider>
        <App />
      </EventsProvider>
    </UIProvider>
  </StrictMode>,
);
