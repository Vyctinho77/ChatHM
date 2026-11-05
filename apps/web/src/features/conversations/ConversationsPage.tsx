import { safeStorage } from "@/lib/storage";
import { useState } from "react";
import { useNavigate, useParams } from "react-router";
import { MessageSquareText } from "lucide-react";
import { Group, Panel, useDefaultLayout } from "react-resizable-panels";
import { useMediaQuery } from "@/lib/theme";
import { ResizeHandle } from "@/features/shell/AppShell";
import { ChatPanel } from "./ChatPanel";
import { ContactPanel } from "./ContactPanel";
import { ConversationList } from "./ConversationList";
import { useConversations } from "./store";

function EmptyChat() {
  return (
    <div className="chat-wallpaper flex h-full flex-col items-center justify-center gap-3 p-8 text-center text-muted">
      <MessageSquareText className="size-12 opacity-40" />
      <p className="text-lg font-medium text-fg">Selecione uma conversa</p>
      <p className="max-w-sm text-sm">
        Conversas novas chegam pela fila da sua equipe. Ao responder, você assume o atendimento.
      </p>
    </div>
  );
}

export function ConversationsPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { data } = useConversations();
  const [contactOpen, setContactOpen] = useState(false); // fechado por padrão
  const wide = useMediaQuery("(min-width: 768px)");
  const roomy = useMediaQuery("(min-width: 1280px)");
  const panelIds = contactOpen && roomy ? ["list", "chat", "contact"] : ["list", "chat"];
  const layout = useDefaultLayout({ id: "chathm-conversations", storage: safeStorage, panelIds });

  const conversation = data?.find((c) => c.id === id);
  const select = (cid: string) => navigate(`/conversas/${cid}`);
  const toggleContact = () => setContactOpen((o) => !o);

  // Telas pequenas: uma coluna por vez (lista → chat → contato).
  if (!wide) {
    if (!conversation) return <ConversationList selectedId={id} onSelect={select} />;
    return (
      <div className="relative h-full">
        <ChatPanel
          conversation={conversation}
          contactOpen={contactOpen}
          onToggleContact={toggleContact}
          onBack={() => navigate("/conversas")}
        />
        {contactOpen && (
          <div className="absolute inset-0 z-20">
            <ContactPanel key={conversation.id} conversation={conversation} onClose={() => setContactOpen(false)} />
          </div>
        )}
      </div>
    );
  }

  const chat = conversation ? (
    <ChatPanel conversation={conversation} contactOpen={contactOpen} onToggleContact={toggleContact} />
  ) : (
    <EmptyChat />
  );

  return (
    <Group orientation="horizontal" defaultLayout={layout.defaultLayout} onLayoutChanged={layout.onLayoutChanged}>
      <Panel id="list" defaultSize={380} minSize={300} maxSize={520} groupResizeBehavior="preserve-pixel-size">
        <ConversationList selectedId={id} onSelect={select} />
      </Panel>
      <ResizeHandle />
      <Panel id="chat" minSize={360}>
        {/* Em telas médias o painel de contato sobrepõe o chat sem desmontá-lo (rascunho preservado). */}
        <div className="relative h-full">
          {chat}
          {conversation && contactOpen && !roomy && (
            <div className="absolute inset-y-0 right-0 z-20 w-80 border-l border-line shadow-2xl">
              <ContactPanel key={conversation.id} conversation={conversation} onClose={() => setContactOpen(false)} />
            </div>
          )}
        </div>
      </Panel>
      {conversation && contactOpen && roomy && (
        <>
          <ResizeHandle />
          <Panel id="contact" defaultSize={340} minSize={280} maxSize={480} groupResizeBehavior="preserve-pixel-size">
            <ContactPanel key={conversation.id} conversation={conversation} onClose={() => setContactOpen(false)} />
          </Panel>
        </>
      )}
    </Group>
  );
}
