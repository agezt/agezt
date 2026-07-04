import { useChat } from "@/lib/chatStore";
import { useComposer } from "./useComposer";
import { useConversationControls } from "./useConversationControls";
import { useConversationRouting } from "./useConversationRouting";
import { useContextWindow } from "./useContextWindow";
import { useSteering } from "./useSteering";
import { useVoice } from "./useVoice";

export function useChatSession() {
  const {
    store,
    messages,
    busy,
    model,
    setModel,
    agent,
    setAgent,
    executionProfile,
    setExecutionProfile,
    activeModel,
    send,
    retry,
    continueRun,
    editAndResend,
    conversationPersona,
    setConversationPersona,
    autoApproveForge,
    setAutoApproveForge,
    trustWebContent,
    setTrustWebContent,
    historySummary,
    stop,
    newChat,
    selectConversation,
    removeConversation,
    renameConversation,
    togglePin,
    activeCorr,
    steer,
    queue,
    enqueue,
    removeQueued,
    reorderQueued,
    clearQueue,
    sendQueuedNow,
  } = useChat();

  const { pinned, setPinned, scrollRef, onScroll, jumpToLatest } = useContextWindow({ messages });

  const {
    input,
    setInput,
    attached,
    attachOpen,
    setAttachOpen,
    taRef,
    doSend,
    addAttachment,
    removeAttachment,
    onKeyDown,
  } = useComposer({ busy, enqueue, send, setPinned });

  const { autoSpeak, toggleAutoSpeak } = useVoice({ busy, messages });
  const { chatChain } = useConversationRouting();
  const { doSteer } = useSteering({ activeCorr, input, setInput, steer });
  const { convFilter, setConvFilter, startNewChat } = useConversationControls({ newChat, setInput });

  return {
    store,
    messages,
    busy,
    model,
    setModel,
    agent,
    setAgent,
    executionProfile,
    setExecutionProfile,
    activeModel,
    send,
    retry,
    continueRun,
    editAndResend,
    conversationPersona,
    setConversationPersona,
    autoApproveForge,
    setAutoApproveForge,
    trustWebContent,
    setTrustWebContent,
    historySummary,
    stop,
    newChat,
    selectConversation,
    removeConversation,
    renameConversation,
    togglePin,
    activeCorr,
    steer,
    queue,
    enqueue,
    removeQueued,
    reorderQueued,
    clearQueue,
    sendQueuedNow,
    input,
    setInput,
    convFilter,
    setConvFilter,
    pinned,
    attached,
    attachOpen,
    setAttachOpen,
    autoSpeak,
    scrollRef,
    taRef,
    chatChain,
    toggleAutoSpeak,
    onScroll,
    jumpToLatest,
    doSend,
    doSteer,
    addAttachment,
    removeAttachment,
    startNewChat,
    onKeyDown,
  };
}
