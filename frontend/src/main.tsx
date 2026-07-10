import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
// Self-hosted variable sans for all UI chrome (M951). Vite emits the woff2 as a
// hashed same-origin asset, so it loads under the strict CSP (font-src 'self').
// JetBrains Mono Variable for code, IDs, and numbers — crisper than system mono.
// Space Grotesk Variable is the display face: page titles, brand, big numbers.
import "@fontsource-variable/inter";
import "@fontsource-variable/jetbrains-mono";
import "@fontsource-variable/space-grotesk";
import "./index.css";
import App from "./App";
import { applyAccentHue, loadAccentHue } from "@/lib/accent";
import { applyConsoleTitle, loadConsoleName } from "@/lib/brand";
import { applyTheme } from "@/lib/theme";
import { applyAdvanced } from "@/lib/advanced";

// Apply saved appearance prefs before first paint so there's no flash of the default.
applyTheme();
applyAccentHue(loadAccentHue());
applyConsoleTitle(loadConsoleName());
applyAdvanced();
import { EventsProvider } from "@/lib/events";
import { UIProvider } from "@/components/ui/feedback";
import { ChatProvider } from "@/lib/chatStore";
import { AuthGate } from "@/views/Login";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <UIProvider>
      {/* Password gate (M817): when the console is password-protected, this
          renders the lock screen and holds back the data providers (EventSource
          et al.) until the user logs in. Transparent when no password is set. */}
      <AuthGate>
        <EventsProvider>
          <ChatProvider>
            <App />
          </ChatProvider>
        </EventsProvider>
      </AuthGate>
    </UIProvider>
  </StrictMode>,
);
