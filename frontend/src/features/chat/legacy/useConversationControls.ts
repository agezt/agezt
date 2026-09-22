import { useState } from "react";

interface UseConversationControlsParams {
  newChat: () => void;
  setInput: (value: string) => void;
}

export function useConversationControls({ newChat, setInput }: UseConversationControlsParams) {
  // convFilter filters the conversation sidebar by title/message text (M732).
  const [convFilter, setConvFilter] = useState("");

  function startNewChat() {
    newChat();
    setInput("");
  }

  return {
    convFilter,
    setConvFilter,
    startNewChat,
  };
}
