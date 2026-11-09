import { useEffect, useState, type FormEvent } from "react";
import QRCode from "react-qr-code";
import { AnimatePresence, motion } from "motion/react";
import { Check, Copy, MessageSquarePlus, Pencil, Phone, Send, ShieldAlert, TriangleAlert } from "lucide-react";
import { Avatar } from "@/components/Avatar";
import { WhatsAppIcon } from "@/components/WhatsAppIcon";
import { DetailSkeleton } from "@/components/Skeleton";
import { Alert, Button, Field, IconButton, Modal, Spinner } from "@/components/ui";
import { errorMessage } from "@/lib/api";
import { cn } from "@/lib/cn";
import { stateDot, stateLabel, useConnect, useConnection, useDisconnect, useSimulateInbound, useSimulateTyping, useRequestHistory } from "./api";

function useSecondsLeft(iso?: string) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!iso) return;
    const t = setInterval(() => setNow(Date.now()), 500);
    return () => clearInterval(t);
  }, [iso]);
  return iso ? Math.max(0, Math.round((new Date(iso).getTime() - now) / 1000)) : 0;
}

export function WhatsAppPage() {
  const { data: conn, isPending, error } = useConnection();
  const connect = useConnect();
  const disconnect = useDisconnect();
  const history = useRequestHistory();
  const [days, setDays] = useState(180);
  const [confirming, setConfirming] = useState(false);
  const secondsLeft = useSecondsLeft(conn?.qr?.expiresAt);

  return (
    <div className="h-full overflow-y-auto bg-app">
      <div className="mx-auto flex max-w-2xl flex-col gap-4 px-4 py-6 sm:px-8">
        <div>
          <h1 className="text-xl font-semibold">Conexão WhatsApp</h1>
          <p className="mt-1 text-[13px] text-muted">
            Conecte o número do escritório lendo o QR Code no celular. As mensagens recebidas entram em Conversas.
          </p>
        </div>

        {isPending && <DetailSkeleton className="px-0" />}
        {error && <Alert>{errorMessage(error)}</Alert>}

        {conn && (
          <section className="border-y border-line">
            <header className="flex flex-wrap items-center gap-3 border-b border-line px-5 py-3.5">
              <span className={cn("size-2.5 rounded-full", stateDot[conn.state], conn.state === "pending_qr" && "animate-pulse")} />
              <span className="font-medium">{conn.state === "pending_qr" && conn.pairingComplete ? "QR Code lido · Confirmando conexão" : stateLabel[conn.state]}</span>
            </header>

            <div className="px-5 py-5">
              <AnimatePresence mode="wait" initial={false}>
                <motion.div
                  key={conn.state}
                  initial={{ opacity: 0, y: 6 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, y: -6 }}
                  transition={{ duration: 0.18 }}
                  className="flex flex-col gap-4"
                >
                  {conn.lastError && (
                    <Alert tone={conn.state === "error" ? "danger" : "info"}>{conn.lastError}</Alert>
                  )}

                  {conn.state === "disconnected" && (
                    <>
                      <p className="text-[13px] text-muted">
                        Nenhum número conectado. Ao conectar, um QR Code temporário aparece aqui.
                      </p>
                      <Button className="w-fit" loading={connect.isPending} onClick={() => connect.mutate()}>
                        <WhatsAppIcon className="size-4" /> Conectar WhatsApp
                      </Button>
                    </>
                  )}

                  {conn.state === "pending_qr" && conn.pairingComplete && (
                    <div className="flex items-center gap-3 py-4 text-sm text-muted" role="status">
                      <Spinner /> O celular reconheceu o QR Code. Aguardando o WhatsApp concluir a conexão…
                    </div>
                  )}

                  {conn.state === "pending_qr" && !conn.pairingComplete && (
                    <div className="flex flex-col gap-5 sm:flex-row sm:items-start">
                      <div className="mx-auto flex shrink-0 flex-col items-center gap-2 sm:mx-0">
                        <div className="rounded-xl bg-white p-3">
                          {conn.qr ? (
                            <QRCode value={conn.qr.code} size={188} />
                          ) : (
                            <div className="flex size-[188px] items-center justify-center">
                              <Spinner />
                            </div>
                          )}
                        </div>
                        {conn.qr && (
                          <div className="w-full">
                            <div className="h-1 overflow-hidden rounded-full bg-hover">
                              <motion.div
                                className="h-full bg-brand"
                                animate={{ width: `${Math.min(100, (secondsLeft / 60) * 100)}%` }}
                                transition={{ ease: "linear", duration: 0.5 }}
                              />
                            </div>
                            <p className="mt-1 text-center text-[11px] text-muted tabular-nums">
                              Atualiza em {secondsLeft}s
                            </p>
                          </div>
                        )}
                      </div>
                      <ol className="flex list-decimal flex-col gap-1.5 pl-5 text-[13px]">
                        <li>Abra o WhatsApp no celular do escritório.</li>
                        <li>
                          Toque em <strong>Mais opções</strong> (Android) ou <strong>Configurações</strong> (iPhone).
                        </li>
                        <li>
                          Entre em <strong>Dispositivos conectados</strong> › <strong>Conectar dispositivo</strong>.
                        </li>
                        <li>Aponte a câmera para este QR Code.</li>
                        <li className="list-none pt-2 text-muted">
                          O código é renovado automaticamente e expira em alguns minutos se não for lido.
                        </li>
                        <li className="list-none">
                          <Button variant="secondary" className="mt-1" loading={disconnect.isPending} onClick={() => disconnect.mutate()}>
                            Cancelar
                          </Button>
                        </li>
                      </ol>
                    </div>
                  )}

                  {conn.state === "connected" && (
                    <>
                      <p className="text-sm text-muted" role="status">{conn.preparing ? "WhatsApp conectado. Preparando mensagens e sincronização…" : "WhatsApp conectado. Pronto para receber mensagens."}</p>
                      <div className="text-[13px]" role="status" aria-live="polite">
                        <p className="font-medium">{conn.sync?.state === "syncing" ? "Importando dados do WhatsApp…" : conn.sync?.messages ? "Histórico recebido" : conn.sync?.contacts ? "Contatos sincronizados · aguardando mensagens" : "Aguardando o histórico do WhatsApp…"}</p>
                        {conn.sync && <p className="mt-1 text-muted">{conn.sync.messages} mensagens importadas · {conn.sync.contacts} registros de contato processados</p>}
                        {conn.sync?.state === "partial" && <p className="mt-1 text-danger">Não foi possível importar {conn.sync.failed} registros.</p>}
                        <p className="mt-2 text-xs text-muted">As conversas individuais aparecem em Conversas conforme o celular envia os dados. O WhatsApp pode fornecer apenas parte do histórico. Mensagens antigas entram como texto; PDFs antigos (guias, extratos, notas) são baixados em segundo plano para dar contexto aos atendimentos. Imagens, áudios e vídeos antigos entram só como registro, para não pesar.</p>
                        {!conn.simulator && <p className="mt-2 text-xs text-muted">{conn.historyRequest === "requesting" ? "Solicitando histórico automaticamente…" : conn.historyRequest === "failed" ? "Não foi possível solicitar o histórico automaticamente. Tente novamente." : "O histórico é solicitado automaticamente após a conexão. Mantenha o WhatsApp conectado à internet no celular."}</p>}
                        {!conn.simulator && (
                          <div className="mt-2 flex flex-wrap items-center gap-2">
                            <select aria-label="Período do histórico" value={days} onChange={(e) => setDays(Number(e.target.value))} className="h-9 rounded-lg border border-line bg-elevated px-2 text-[13px]">
                              {[30, 90, 180, 365].map((d) => <option key={d} value={d}>Últimos {d} dias</option>)}
                            </select>
                            <Button variant="ghost" disabled={conn.preparing} loading={history.isPending || conn.historyRequest === "requesting"} onClick={() => history.mutate(days)}>Buscar histórico e PDFs antigos</Button>
                          </div>
                        )}
                        {history.isSuccess && <p className="mt-2 text-xs text-muted">Solicitação enviada. Mantenha o WhatsApp aberto e conectado à internet no celular. As mensagens e os PDFs aparecem conforme o aparelho enviar os dados dos últimos {days} dias; o WhatsApp pode entregar menos do que o pedido e arquivos muito antigos podem já ter sido apagados dele.</p>}
                        {history.error && <p className="mt-2 text-xs text-danger">{errorMessage(history.error)}</p>}
                      </div>
                      <dl className="space-y-6 py-3 text-sm">
                        <div>
                          <dt className="mb-2 text-xs text-muted">Nome no WhatsApp</dt>
                          <dd className="font-medium">{conn.displayName || "—"}</dd>
                        </div>
                        <div>
                          <dt className="mb-2 text-xs text-muted">Telefone</dt>
                          <dd><PhoneNumber phone={conn.phone} /></dd>
                        </div>
                        {conn.connectedAt && (
                          <div className="text-xs text-muted">
                            <dt className="inline">Conectado desde </dt>
                            <dd className="inline">{new Date(conn.connectedAt).toLocaleString("pt-BR")}</dd>
                          </div>
                        )}
                      </dl>
                      <Button variant="secondary" className="w-fit" onClick={() => setConfirming(true)}>
                        Desconectar
                      </Button>
                    </>
                  )}

                  {conn.state === "error" && (
                    <div className="flex flex-wrap gap-2">
                      <Button loading={connect.isPending} onClick={() => connect.mutate()}>
                        Gerar novo QR Code
                      </Button>
                      <Button variant="secondary" onClick={() => setConfirming(true)}>
                        Desconectar
                      </Button>
                    </div>
                  )}

                  {(connect.error || disconnect.error) && <Alert>{errorMessage(connect.error ?? disconnect.error)}</Alert>}
                </motion.div>
              </AnimatePresence>
            </div>
          </section>
        )}

        <div className="flex gap-3 border-t border-line px-5 py-4 text-[12.5px] text-muted">
          <ShieldAlert className="mt-0.5 size-4 shrink-0" />
          <p>
            A conexão usa o protocolo do WhatsApp Web (não oficial) e pode ficar instável ou sofrer restrições do
            WhatsApp. A sessão é guardada criptografada no servidor. O sistema está preparado para migrar para a API
            oficial (WhatsApp Business Platform), cujo cadastro é feito pela Meta, sem QR Code.
          </p>
        </div>

        {conn?.simulator && <Simulator />}
      </div>

      <Modal open={confirming} onClose={() => setConfirming(false)} title="Desconectar WhatsApp">
        <p className="flex gap-2 text-[13px]">
          <TriangleAlert className="mt-0.5 size-4 shrink-0 text-warning" />O aparelho será desvinculado e o ChatHM
          deixará de receber mensagens até uma nova conexão por QR Code.
        </p>
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={() => setConfirming(false)}>
            Cancelar
          </Button>
          <Button
            variant="danger"
            loading={disconnect.isPending}
            onClick={() => disconnect.mutate(undefined, { onSuccess: () => setConfirming(false) })}
          >
            Desconectar
          </Button>
        </div>
      </Modal>
    </div>
  );
}

/** Formulário de teste: disponível só com o provedor simulado (desenvolvimento). */
function Simulator() {
  const sim = useSimulateInbound();
  const typing = useSimulateTyping();
  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState({ phone: "5511988887777", name: "Cliente teste", text: "Bom dia, quero emitir uma nota" });
  function submit(e: FormEvent) {
    e.preventDefault();
    sim.mutate({ ...form, phone: form.phone.replace(/\D/g, "") });
  }
  return (
    <section className="border-t border-line px-5 py-6">
      <h2 className="flex items-center gap-2 text-[13.5px] font-semibold">
        <MessageSquarePlus className="size-4 text-muted" /> Testar uma conversa
      </h2>
      <p className="mt-1 text-xs text-muted">Envie uma mensagem fictícia para ver como ela aparece em Conversas.</p>
      <form onSubmit={submit} className="mt-6 flex flex-col gap-5">
        <div className="flex items-center gap-3">
          <Avatar name={form.name} size="md" />
          <p className="flex-1 text-xs text-muted">Contato de teste</p>
          <IconButton label={editing ? "Concluir edição do contato" : "Editar contato de teste"} aria-expanded={editing} aria-controls="simulator-contact" onClick={() => setEditing(!editing)}>
            {editing ? <Check className="size-4" /> : <Pencil className="size-4" />}
          </IconButton>
        </div>
        <div id="simulator-contact">
          {editing ? <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Nome" required maxLength={120} value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            <Field label="Telefone com DDI" type="tel" required placeholder="+55 (11) 98888-7777" value={form.phone} onChange={(e) => setForm({ ...form, phone: e.target.value })} />
          </div> : <dl className="space-y-5 text-sm">
            <div><dt className="mb-2 text-xs text-muted">Nome</dt><dd className="font-medium">{form.name || "Sem nome"}</dd></div>
            <div><dt className="mb-2 text-xs text-muted">Telefone</dt><dd><PhoneNumber phone={form.phone} /></dd></div>
          </dl>}
        </div>
        <label className="flex flex-col gap-2 text-xs text-muted">
          Mensagem de teste
          <textarea required rows={3} maxLength={4000} value={form.text} onChange={(e) => setForm({ ...form, text: e.target.value })} className="w-full resize-y rounded-xl border border-line bg-elevated px-4 py-3 text-sm leading-relaxed text-fg outline-none focus:border-brand-ink" />
        </label>
        <div className="flex flex-wrap items-center gap-3">
          <Button type="submit" loading={sim.isPending} disabled={!form.name.trim() || !form.text.trim() || !form.phone.replace(/\D/g, "")}>
            <Send className="size-4" /> Enviar teste
          </Button>
          <Button type="button" variant="ghost" loading={typing.isPending} disabled={!form.phone.replace(/\D/g, "")} onClick={() => typing.mutate(form.phone)}>Testar digitando</Button>
          {sim.isSuccess && <span className="text-[12px] text-muted">Entregue. Veja em Conversas.</span>}
          {sim.error && <span className="text-[12px] text-danger">{errorMessage(sim.error)}</span>}
        </div>
        {typing.isSuccess && <p role="status" className="text-xs text-muted">Indicador enviado por 10 segundos. Veja a conversa desse contato na lista; envie uma mensagem de teste primeiro se ela ainda não existir.</p>}
        {typing.error && <p role="alert" className="text-xs text-danger">{errorMessage(typing.error)}</p>}
        <p className="text-[11px] text-muted">Disponível apenas no ambiente de desenvolvimento.</p>
      </form>
    </section>
  );
}

function PhoneNumber({ phone }: { phone: string }) {
  const [copied, setCopied] = useState(false);
  const [copyError, setCopyError] = useState(false);
  useEffect(() => {
    if (!copied) return;
    const timer = setTimeout(() => setCopied(false), 2000);
    return () => clearTimeout(timer);
  }, [copied]);
  const digits = phone.replace(/\D/g, "");
  const formatted = /^55\d{10,11}$/.test(digits)
    ? `+55 (${digits.slice(2, 4)}) ${digits.slice(4, -4)}-${digits.slice(-4)}`
    : phone || "—";
  return <>
    <div className="flex items-center gap-3">
      <Phone className="size-4 shrink-0 text-muted" />
      <span className="min-w-0 flex-1 break-words tabular-nums">{formatted}</span>
      {phone && <IconButton label={copied ? "Telefone copiado" : "Copiar telefone"} onClick={async () => {
        try {
          await navigator.clipboard.writeText(phone);
          setCopyError(false);
          setCopied(true);
        } catch {
          setCopyError(true);
        }
      }}>{copied ? <Check className="size-4 text-brand-ink" /> : <Copy className="size-4" />}</IconButton>}
    </div>
    <span role="status" className="text-xs text-muted">{copied ? "Telefone copiado." : copyError ? "Não foi possível copiar. Selecione o número para copiar." : ""}</span>
  </>;
}
