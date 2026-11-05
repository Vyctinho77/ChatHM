import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { get, post } from "@/lib/api";

export type ConnectionState = "disconnected" | "pending_qr" | "connected" | "error";

export type HistoryProgress = { state: "syncing" | "ready" | "partial"; messages: number; contacts: number; failed: number };

export type Connection = {
  state: ConnectionState;
  phone: string;
  displayName: string;
  lastError?: string;
  connectedAt?: string;
  pairingComplete?: boolean;
  sync?: HistoryProgress;
  historyRequest?: "requesting" | "requested" | "failed";
  preparing?: boolean;
  qr?: { code: string; expiresAt: string } | null;
  simulator?: boolean;
};

export const connectionKey = ["whatsapp", "connection"] as const;

export function useConnection() {
  return useQuery({
    queryKey: connectionKey,
    queryFn: () => get<Connection>("/whatsapp/connection"),
    // O WebSocket atualiza em tempo real; a consulta periódica cobre quedas do WebSocket.
    refetchInterval: (q) => (q.state.data?.state === "pending_qr" || q.state.data?.preparing || q.state.data?.sync?.state === "syncing" ? 1_000 : 10_000),
  });
}

function useConnectionMutation(path: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => post<Connection>(path),
    // A provider event may already be newer than the POST response.
    onSuccess: () => qc.invalidateQueries({ queryKey: connectionKey }),
  });
}

export const useConnect = () => useConnectionMutation("/whatsapp/connect");
export const useDisconnect = () => useConnectionMutation("/whatsapp/disconnect");

export function useRequestHistory() {
  return useMutation({ mutationFn: () => post<{status: string}>("/whatsapp/history") });
}

export function useSimulateInbound() {
  return useMutation({
    mutationFn: (body: { phone: string; name: string; text: string }) => post<void>("/dev/whatsapp/inbound", body),
  });
}

export const stateLabel: Record<ConnectionState, string> = {
  disconnected: "Desconectado",
  pending_qr: "Aguardando leitura do QR Code",
  connected: "Conectado",
  error: "Atenção",
};

export const stateDot: Record<ConnectionState, string> = {
  disconnected: "bg-muted/60",
  pending_qr: "bg-warning",
  connected: "bg-unread",
  error: "bg-danger",
};
