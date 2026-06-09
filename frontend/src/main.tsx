import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import App from "./App";
import { EventsProvider } from "@/lib/events";
import { UIProvider } from "@/components/ui/feedback";
import { ChatProvider } from "@/lib/chatStore";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <UIProvider>
      <EventsProvider>
        <ChatProvider>
          <App />
        </ChatProvider>
      </EventsProvider>
    </UIProvider>
  </StrictMode>,
);
