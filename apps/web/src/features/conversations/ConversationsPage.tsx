import { safeStorage } from "@/lib/storage";
import { useEffect, useState } from "react";
import { useLocation, useNavigate, useParams } from "react-router";
import { AnimatePresence, motion } from "motion/react";
import { MessageSquareText } from "lucide-react";
import { Group, Panel, useDefaultLayout } from "react-resizable-panels";
import { useMediaQuery } from "@/lib/theme";
import { ResizeHandle } from "@/features/shell/AppShell";
import { ChatPanel } from "./ChatPanel";
import { ContactPanel } from "./ContactPanel";
import { ConversationList } from "./ConversationList";
import { GettingStarted } from "./GettingStarted";
import { useConversations } from "./store";

// Painel sobreposto desliza a partir da direita.
const slideIn = {
  initial: { x: "100%" },
  animate: { x: 0 },
  exit: { x: "100%" },
  transition: { type: "spring", stiffness: 420, damping: 40 },
} as const;

function EmptyChat() {
  return (
    <div className="chat-wallpaper flex h-full flex-col items-center justify-center gap-3 p-8 text-center text-muted">
      <MessageSquareText className="size-12 opacity-40" />
      <p className="text-lg font-medium text-fg">Selecione uma conversa</p>
      <p className="max-w-sm text-sm">
        Conversas novas chegam pela fila da sua equipe. Ao responder, você assume o atendimento.
      </p>
      <GettingStarted />
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

  // Atendimento encerrado deixa de existir para a lista: volta para a tela de conversas.
  const conversation = data?.find((c) => c.id === id && c.status !== "closed");
  // Mantém o filtro de fila (?fila=) ao abrir e fechar conversas.
  const { search } = useLocation();
  const select = (cid: string) => navigate({ pathname: `/conversas/${cid}`, search });
  const stale = !!id && !!data && !conversation;
  useEffect(() => {
    if (stale) navigate({ pathname: "/conversas", search }, { replace: true });
  }, [stale, navigate, search]);
  const toggleContact = () => setContactOpen((o) => !o);

  // Telas pequenas: uma coluna por vez (lista → chat → contato).
  if (!wide) {
    if (!conversation) return <ConversationList selectedId={id} onSelect={select} />;
    return (
      <div className="relative h-full overflow-hidden">
        <motion.div
          key={conversation.id}
          className="h-full"
          initial={{ opacity: 0, x: 24 }}
          animate={{ opacity: 1, x: 0 }}
          transition={{ type: "spring", stiffness: 420, damping: 38 }}
        >
          <ChatPanel
            conversation={conversation}
            contactOpen={contactOpen}
            onToggleContact={toggleContact}
            onBack={() => navigate({ pathname: "/conversas", search })}
          />
        </motion.div>
        <AnimatePresence>
          {contactOpen && (
            <motion.div key="contact" className="absolute inset-0 z-20" {...slideIn}>
              <ContactPanel key={conversation.id} conversation={conversation} onClose={() => setContactOpen(false)} />
            </motion.div>
          )}
        </AnimatePresence>
      </div>
    );
  }

  const chat = conversation ? (
    <motion.div
      key={conversation.id}
      className="h-full"
      initial={{ opacity: 0, y: 6 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.2, ease: "easeOut" }}
    >
      <ChatPanel conversation={conversation} contactOpen={contactOpen} onToggleContact={toggleContact} />
    </motion.div>
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
        <div className="relative h-full overflow-hidden">
          {chat}
          <AnimatePresence>
            {conversation && contactOpen && !roomy && (
              <motion.div
                key="contact"
                className="absolute inset-y-0 right-0 z-20 w-80 border-l border-line shadow-2xl"
                {...slideIn}
              >
                <ContactPanel key={conversation.id} conversation={conversation} onClose={() => setContactOpen(false)} />
              </motion.div>
            )}
          </AnimatePresence>
        </div>
      </Panel>
      {conversation && contactOpen && roomy && (
        <>
          <ResizeHandle />
          <Panel id="contact" defaultSize={340} minSize={280} maxSize={480} groupResizeBehavior="preserve-pixel-size">
            <motion.div
              className="h-full"
              initial={{ opacity: 0, x: 24 }}
              animate={{ opacity: 1, x: 0 }}
              transition={{ type: "spring", stiffness: 420, damping: 38 }}
            >
              <ContactPanel key={conversation.id} conversation={conversation} onClose={() => setContactOpen(false)} />
            </motion.div>
          </Panel>
        </>
      )}
    </Group>
  );
}
