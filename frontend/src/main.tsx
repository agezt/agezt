import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import App from "./App";
import { applyAccentHue, loadAccentHue } from "@/lib/accent";

// Apply the saved accent hue before first paint so there's no flash of the default.
applyAccentHue(loadAccentHue());
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
