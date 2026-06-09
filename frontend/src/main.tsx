import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import App from "./App";
import { applyAccentHue, loadAccentHue } from "@/lib/accent";
import { applyConsoleTitle, loadConsoleName } from "@/lib/brand";
import { applyTheme } from "@/lib/theme";

// Apply saved appearance prefs before first paint so there's no flash of the default.
applyTheme();
applyAccentHue(loadAccentHue());
applyConsoleTitle(loadConsoleName());
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
