import { describe, expect, it } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { applyRealtimeEvent } from "./useRealtime";
import { connectionKey, type Connection } from "@/features/whatsapp/api";

describe("WhatsApp events", () => {
  it("removes the QR as soon as the connection is confirmed and ignores late QR events", () => {
    const qc = new QueryClient();
    const qr = { code: "temporary", expiresAt: "2030-01-01T00:00:00Z" };
    qc.setQueryData<Connection>(connectionKey, { state: "pending_qr", phone: "", displayName: "", qr });
    applyRealtimeEvent(qc, { type: "whatsapp.status", data: { state: "connected", phone: "+5592000000000", displayName: "Escritório" } });
    applyRealtimeEvent(qc, { type: "whatsapp.qr", data: qr });
    expect(qc.getQueryData<Connection>(connectionKey)?.state).toBe("connected");
    expect(qc.getQueryData<Connection>(connectionKey)?.qr).toBeNull();
    qc.clear();
  });

  it("refreshes conversations and messages when a history chunk finishes", () => {
    const qc = new QueryClient();
    qc.setQueryData<Connection>(connectionKey, { state: "connected", phone: "", displayName: "" });
    qc.setQueryData(["conversations"], []);
    qc.setQueryData(["conversations", "chat", "messages"], []);
    applyRealtimeEvent(qc, { type: "whatsapp.sync", data: { state: "ready", messages: 20, contacts: 3, failed: 0 } });
    expect(qc.getQueryData<Connection>(connectionKey)?.sync?.messages).toBe(20);
    expect(qc.getQueryState(["conversations"])?.isInvalidated).toBe(true);
    expect(qc.getQueryState(["conversations", "chat", "messages"])?.isInvalidated).toBe(true);
    qc.clear();
  });
});
