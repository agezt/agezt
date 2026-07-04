import { ArrowDown, Download, Forward, ListPlus, Paperclip, Plus, Radio, Search, Send, Square, StickyNote, Volume2, VolumeX, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { ModelPicker } from "@/components/ModelPicker";
import { AgentPicker } from "@/components/AgentPicker";
import { AttachPicker } from "@/components/AttachPicker";
import { MicButton } from "@/components/MicButton";
import { speechSupported } from "@/lib/speech";
import { conversationToMarkdown, slugify, downloadText } from "@/lib/export";
import { filterConversations, sortConversations } from "@/lib/conversations";
import { ChannelSessions } from "@/views/ChannelSessions";
import { SuggestionsBar } from "@/components/SuggestionsBar";
import { ConversationItem, EmptyState, lastAssistantTools, QueuePanel } from "./conversation";
import { useChatSession } from "./useChatSession";
import { ConversationPersona, ExecutionProfilePicker, MessageRow, PromptLauncher, SummaryDivider } from "./message";
export { ConversationItem } from "./conversation";
export {
  AssistantBubble,
  CompactionNote,
  ContextChip,
  ContextModal,
  ConversationPersona,
  ExecutionProfilePicker,
  FallbackNote,
  PromptLauncher,
  SummaryDivider,
  UserBubble,
  barTone,
} from "./message";

// Chat is the humane front door to the agent: a conversational thread where you
// type an intent and watch the governed loop answer live — streaming text, the
// tool calls it made (with the policy verdict), and the final answer with its
// cost. The engine (store, streaming, model) lives in ChatProvider so a run keeps
// going when you leave the view; this component is the full-screen UI over it.
export function Chat() {
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
  } = useChatSession();

  return (
    <div className="flex h-full gap-3">
      {attachOpen && (
        <AttachPicker
          selectedIds={new Set(attached.map((r) => r.id))}
          onPick={addAttachment}
          onClose={() => setAttachOpen(false)}
        />
      )}
      {/* Conversation list — past threads, ChatGPT-style (desktop). */}
      <aside className="hidden w-52 shrink-0 flex-col border-r border-border/50 pr-3 md:flex">
        <button
          onClick={startNewChat}
          className="mb-2 inline-flex items-center justify-center gap-1.5 rounded-md bg-accent/10 px-2 py-1.5 text-xs font-medium text-accent transition-colors hover:bg-accent hover:text-white"
        >
          <Plus className="size-3.5" /> New chat
        </button>
        {store.conversations.length > 1 && (
          <div className="relative mb-2">
            <Search className="pointer-events-none absolute left-2 top-1/2 size-3 -translate-y-1/2 text-muted" />
            <input
              value={convFilter}
              onChange={(e) => setConvFilter(e.target.value)}
              placeholder="Search chats…"
              aria-label="Search conversations"
              className="h-7 w-full rounded-md bg-panel/60 pl-7 pr-6 text-xs outline-none ring-1 ring-border/50 focus:ring-accent/60"
            />
            {convFilter && (
              <button
                onClick={() => setConvFilter("")}
                aria-label="Clear conversation search"
                className="absolute right-1.5 top-1/2 -translate-y-1/2 text-muted transition-colors hover:text-foreground"
              >
                <X className="size-3" />
              </button>
            )}
          </div>
        )}
        <div className="min-h-0 flex-1 space-y-0.5 overflow-auto">
          {(() => {
            const shown = sortConversations(filterConversations(store.conversations, convFilter));
            if (shown.length === 0) {
              return <p className="px-2 py-3 text-center text-[11px] text-muted">No chats match “{convFilter.trim()}”</p>;
            }
            return shown.map((c) => (
              <ConversationItem
                key={c.id}
                title={c.title || "New chat"}
                active={c.id === store.activeId}
                pinned={!!c.pinned}
                onSelect={() => selectConversation(c.id)}
                onRemove={() => removeConversation(c.id)}
                onRename={(t) => renameConversation(c.id, t)}
                onTogglePin={() => togglePin(c.id)}
              />
            ));
          })()}
        </div>
        {/* Channel-originated sessions (Telegram/Slack/…) — follow them live (M841). */}
        <ChannelSessions />
      </aside>

      {/* Active thread + composer. */}
      <div className="mx-auto flex h-full min-w-0 max-w-3xl flex-1 flex-col">
        {/* Small screens have no sidebar — keep a New chat affordance here. */}
        {messages.length > 0 && (
          <div className="flex items-center justify-between border-b border-border/50 pb-2 md:hidden">
            <span className="text-xs text-muted">
              {messages.filter((m) => m.role === "user").length} message
              {messages.filter((m) => m.role === "user").length === 1 ? "" : "s"}
            </span>
            <button
              onClick={startNewChat}
              className="inline-flex items-center gap-1 rounded-md bg-panel/60 px-2 py-1 text-xs text-muted shadow-sm transition-colors hover:bg-panel hover:text-foreground"
            >
              <Plus className="size-3.5" /> New chat
            </button>
          </div>
        )}
        <div className="relative min-h-0 flex-1">
          <div ref={scrollRef} onScroll={onScroll} className="h-full overflow-auto">
            {messages.length === 0 ? (
              <EmptyState onPick={setInput} />
            ) : (
              <div className="space-y-4 py-2">
                {messages.map((m, i) => {
                  const isLast = i === messages.length - 1;
                  const canRetry = isLast && !busy && m.role === "assistant" && m.turn.status === "error";
                  // Regenerate a completed answer (re-run the same intent, replacing
                  // this turn) — the staple chat affordance, reusing retry's logic.
                  const canRegenerate = isLast && !busy && m.role === "assistant" && m.turn.status === "done";
                  return (
                    <div key={i} className="msg-in">
                      {historySummary && historySummary.upto === i && i > 0 && (
                        <SummaryDivider summary={historySummary} />
                      )}
                      <MessageRow
                        msg={m}
                        agent={agent}
                        onRetry={canRetry ? retry : undefined}
                        onContinue={canRetry ? continueRun : undefined}
                        onRegenerate={canRegenerate ? retry : undefined}
                        onEdit={!busy && m.role === "user" ? (text) => editAndResend(i, text) : undefined}
                      />
                    </div>
                  );
                })}
              </div>
            )}
          </div>
          {!pinned && messages.length > 0 && (
            <button
              onClick={jumpToLatest}
              title="Jump to latest"
              className="absolute bottom-3 left-1/2 inline-flex -translate-x-1/2 items-center gap-1.5 rounded-full bg-card px-3 py-1.5 text-xs shadow-lg shadow-black/20 transition-all hover:shadow-xl hover:-translate-y-0.5"
            >
              <ArrowDown className="size-3.5" /> Jump to latest
            </button>
          )}
        </div>

        <div className="gradient-rule" />
        <div className="pt-3">
        {/* Suggested next prompts (memory-derived + tool-context): shown once a
            turn has completed. Clicking a chip drops its prompt into the input. */}
        {messages.length > 0 && (
          <SuggestionsBar
            sessionId={store.activeId}
            recentTools={lastAssistantTools(messages)}
            busy={busy}
            onPick={setInput}
          />
        )}
        {/* Queued follow-ups (M962): lined up while a run streams; the next one
            auto-sends when the current run finishes. Reorder / delete / clear. */}
        {queue.length > 0 && (
          <QueuePanel
            queue={queue}
            busy={busy}
            onUp={(id) => reorderQueued(id, -1)}
            onDown={(id) => reorderQueued(id, 1)}
            onRemove={removeQueued}
            onClear={clearQueue}
            onSendNow={sendQueuedNow}
          />
        )}
        {attached.length > 0 && (
          <div className="mb-1.5 flex flex-wrap items-center gap-1.5">
            {attached.map((ref) => (
              <span
                key={ref.id}
                title={`${ref.kind}: ${ref.label}`}
                className="inline-flex min-w-0 max-w-64 items-center gap-1 rounded-full border border-accent/30 bg-accent/10 px-2 py-0.5 text-xs text-accent"
              >
                <Paperclip className="size-3 shrink-0" />
                <span className="truncate">{ref.label}</span>
                <button
                  type="button"
                  onClick={() => removeAttachment(ref.id)}
                  aria-label={`Remove attachment ${ref.label}`}
                  className="rounded-full p-0.5 text-accent/70 transition-colors hover:bg-accent/15 hover:text-accent"
                >
                  <X className="size-3" />
                </button>
              </span>
            ))}
          </div>
        )}
        {/* Composer surface (M995): input + controls in one elevated card that
            lights an accent ring on focus, like a modern chat composer. */}
        <div className="rounded-xl bg-panel/40 px-2 py-1.5 shadow-e1 transition-all focus-within-glow">
        <div className="flex items-end gap-2">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => setAttachOpen(true)}
            title="Attach a skill, memory, or past run"
          >
            <Paperclip className="size-4" />
          </Button>
          <MicButton
            onText={(t) => setInput((cur) => (cur.trim() ? cur.trimEnd() + " " : "") + t)}
            disabled={busy}
          />
          <Button
            type="button"
            variant="ghost"
            size="icon"
            title="Hands-free Voice mode"
            onClick={() => {
              location.hash = "voice";
            }}
          >
            <Radio className="size-4" />
          </Button>
          <textarea
            ref={taRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={onKeyDown}
            rows={1}
            placeholder={
              busy
                ? "Run in flight — Enter queues a follow-up; Steer or BTW the running agent →"
                : "Ask the agent to do something…  (Enter to send, Shift+Enter for a new line)"
            }
            className="max-h-40 min-h-[2.5rem] flex-1 resize-none overflow-y-auto bg-transparent px-1.5 py-1.5 text-sm outline-none placeholder:text-muted"
          />
          {busy ? (
            <div className="flex items-end gap-1">
              {activeCorr && (
                <>
                  <Button variant="ghost" size="icon" onClick={() => doSteer("note")} disabled={!input.trim()} title="BTW — the agent reads this and keeps going (doesn't break its task)">
                    <StickyNote className="size-4" />
                  </Button>
                  <Button variant="accent" size="icon" onClick={() => doSteer("steer")} disabled={!input.trim()} title="Steer — interrupt at the next safe point and follow this">
                    <Forward className="size-4" />
                  </Button>
                </>
              )}
              <Button variant="ghost" size="icon" onClick={doSend} disabled={!input.trim()} title="Queue — send after the current run finishes">
                <ListPlus className="size-4" />
              </Button>
              <Button variant="danger" size="icon" onClick={stop} title="Stop">
                <Square className="size-4" />
              </Button>
            </div>
          ) : (
            <Button variant="accent" size="icon" onClick={doSend} disabled={!input.trim()} title="Send">
              <Send className="size-4" />
            </Button>
          )}
        </div>
        <div className="mt-1.5 flex flex-wrap items-center gap-2 border-t border-border/40 px-1 pt-1.5 text-xs text-muted">
          <span>model</span>
          <ModelPicker
            value={model}
            onChange={setModel}
            // With no explicit pick the chat ROUTING CHAIN serves the run, not the
            // kernel default — show the chain's primary so the label tells the truth.
            activeModel={chatChain.length ? `${chatChain[0]} (routing)` : activeModel}
            pinned={chatChain.length ? { label: "chat routing chain", ids: chatChain } : undefined}
          />
          <AgentPicker value={agent} onChange={setAgent} />
          <ExecutionProfilePicker value={executionProfile} onChange={setExecutionProfile} />
          <ConversationPersona value={conversationPersona} onChange={setConversationPersona} />
          <button
            onClick={() => setAutoApproveForge(!autoApproveForge)}
            role="switch"
            aria-checked={autoApproveForge}
            title={
              autoApproveForge
                ? "Tool Forge actions are auto-approved for this session — click to require approval again"
                : "Auto-approve Tool Forge actions for this session (no approval prompt while building an agent army)"
            }
            className={
              "inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-xs transition-colors " +
              (autoApproveForge
                ? "bg-accent/15 text-accent"
                : "text-muted hover:text-foreground")
            }
          >
            <span aria-hidden>{autoApproveForge ? "🔓" : "🔒"}</span>
            Forge auto-approve
          </button>
          <button
            onClick={() => setTrustWebContent(!trustWebContent)}
            role="switch"
            aria-checked={trustWebContent}
            title={
              trustWebContent
                ? "Trusting this run's web/file content — the prompt-injection guard warns instead of asking. Click to require approval again."
                : "Trust web/file content for this session so you aren't prompted to approve every action after a search (the guard still warns + audits)."
            }
            className={
              "inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-xs transition-colors " +
              (trustWebContent
                ? "bg-accent/15 text-accent"
                : "text-muted hover:text-foreground")
            }
          >
            <span aria-hidden>{trustWebContent ? "🌐" : "🛡️"}</span>
            Trust web content
          </button>
          <PromptLauncher onPick={(text) => setInput((cur) => (cur.trim() ? cur.trimEnd() + "\n" : "") + text)} />
          {messages.length > 0 && (
            <button
              onClick={() => {
                const title =
                  store.conversations.find((c) => c.id === store.activeId)?.title || "Conversation";
                downloadText(`${slugify(title)}.md`, conversationToMarkdown(title, messages));
              }}
              title="Export this conversation as Markdown"
              className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-muted transition-colors hover:text-foreground"
            >
              <Download className="size-3.5" />
              <span>export</span>
            </button>
          )}
          {speechSupported() && (
            <button
              onClick={toggleAutoSpeak}
              title={autoSpeak ? "Auto-speak answers: on" : "Auto-speak answers: off"}
              className={cn(
                "inline-flex items-center gap-1 rounded px-1.5 py-0.5 transition-colors hover:text-foreground",
                autoSpeak ? "text-accent" : "text-muted",
              )}
            >
              {autoSpeak ? <Volume2 className="size-3.5" /> : <VolumeX className="size-3.5" />}
              <span>speak</span>
            </button>
          )}
          <span className="ml-auto">runs through the governed loop · same as the CLI</span>
        </div>
        </div>
        </div>
      </div>
    </div>
  );
}

