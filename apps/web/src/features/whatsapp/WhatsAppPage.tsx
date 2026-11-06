import { useEffect, useState, type FormEvent } from "react";
import QRCode from "react-qr-code";
import { AnimatePresence, motion } from "motion/react";
import { MessageSquarePlus, ShieldAlert, Smartphone, TriangleAlert } from "lucide-react";
import { Alert, Button, Field, Modal, Spinner } from "@/components/ui";
import { errorMessage } from "@/lib/api";
import { cn } from "@/lib/cn";
import { stateDot, stateLabel, useConnect, useConnection, useDisconnect, useSimulateInbound, useRequestHistory } from "./api";

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

        {isPending && <Spinner className="mx-auto mt-8" />}
        {error && <Alert>{errorMessage(error)}</Alert>}

        {conn && (
          <section className="border-y border-line">
            <header className="flex flex-wrap items-center gap-3 border-b border-line px-5 py-3.5">
              <span className={cn("size-2.5 rounded-full", stateDot[conn.state], conn.state === "pending_qr" && "animate-pulse")} />
              <span className="font-medium">{conn.state === "pending_qr" && conn.pairingComplete ? "QR Code lido · Confirmando conexão" : stateLabel[conn.state]}</span>
              {conn.state === "connected" && conn.phone && (
                <span className="text-[13px] text-muted">
                  {conn.displayName ? `${conn.displayName} · ` : ""}
                  {conn.phone}
                </span>
              )}
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
                        <Smartphone className="size-4" /> Conectar WhatsApp
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
                        <p className="mt-2 text-xs text-muted">As conversas individuais aparecem em Conversas conforme o celular envia os dados. O WhatsApp pode fornecer apenas parte do histórico. Mídias antigas são importadas com seus metadados.</p>
                        {!conn.simulator && <p className="mt-2 text-xs text-muted">{conn.historyRequest === "requesting" ? "Solicitando histórico automaticamente…" : conn.historyRequest === "failed" ? "Não foi possível solicitar o histórico automaticamente. Tente novamente." : "O histórico é solicitado automaticamente após a conexão. Mantenha o WhatsApp conectado à internet no celular."}</p>}
                        {!conn.simulator && <Button variant="ghost" className="mt-2" disabled={conn.preparing} loading={history.isPending || conn.historyRequest === "requesting"} onClick={() => history.mutate()}>Tentar sincronizar novamente</Button>}
                        {history.isSuccess && <p className="mt-2 text-xs text-muted">Solicitação enviada. Mantenha o WhatsApp aberto e conectado à internet no celular. As mensagens aparecerão quando o aparelho enviar os dados dos últimos 30 dias; a disponibilidade depende do WhatsApp.</p>}
                        {history.error && <p className="mt-2 text-xs text-danger">{errorMessage(history.error)}</p>}
                      </div>
                      <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-1.5 text-[13px]">
                        <dt className="text-muted">Número</dt>
                        <dd>{conn.phone || "—"}</dd>
                        <dt className="text-muted">Perfil</dt>
                        <dd>{conn.displayName || "—"}</dd>
                        {conn.connectedAt && (
                          <>
                            <dt className="text-muted">Conectado desde</dt>
                            <dd>{new Date(conn.connectedAt).toLocaleString("pt-BR")}</dd>
                          </>
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
  const [form, setForm] = useState({ phone: "5511988887777", name: "Cliente teste", text: "Bom dia, quero emitir uma nota" });
  function submit(e: FormEvent) {
    e.preventDefault();
    sim.mutate(form);
  }
  return (
    <section className="border-t border-line px-5 py-4">
      <h2 className="flex items-center gap-2 text-[13.5px] font-semibold">
        <MessageSquarePlus className="size-4" /> Simular mensagem recebida
      </h2>
      <p className="mt-0.5 text-[12px] text-muted">Provedor simulado (desenvolvimento). Não aparece em produção.</p>
      <form onSubmit={submit} className="mt-3 grid gap-3 sm:grid-cols-2">
        <Field label="Telefone (DDI+DDD+número)" value={form.phone} onChange={(e) => setForm({ ...form, phone: e.target.value })} />
        <Field label="Nome no WhatsApp" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
        <Field className="sm:col-span-2" label="Mensagem" value={form.text} onChange={(e) => setForm({ ...form, text: e.target.value })} />
        <div className="flex items-center gap-3 sm:col-span-2">
          <Button type="submit" loading={sim.isPending}>
            Receber
          </Button>
          {sim.isSuccess && <span className="text-[12px] text-muted">Entregue. Veja em Conversas.</span>}
          {sim.error && <span className="text-[12px] text-danger">{errorMessage(sim.error)}</span>}
        </div>
      </form>
    </section>
  );
}
