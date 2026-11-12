import { safeStorage } from "@/lib/storage";

// Preferência deste navegador, por conta: "sempre levar meu lembrete ao transferir, sem perguntar".
const key = (userId: string) => `chathm.transfer.shareReminder.${userId}`;

export const getShareReminderAlways = (userId: string) => safeStorage.getItem(key(userId)) === "1";

export function setShareReminderAlways(userId: string, on: boolean) {
  safeStorage.setItem(key(userId), on ? "1" : "0");
}
