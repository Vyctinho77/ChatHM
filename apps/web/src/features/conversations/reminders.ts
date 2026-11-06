import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMe } from "@/features/auth/api";
import { api, del, get } from "@/lib/api";
import { DEMO } from "@/lib/env";

export const reminderColors = {
  olive: { name: "Verde", dot: "bg-[#8B9D40]", style: "bg-brand/10 text-brand-ink" },
  amber: { name: "Amarelo", dot: "bg-amber-500", style: "bg-amber-500/10 text-amber-800 dark:text-amber-200" },
  sky: { name: "Azul", dot: "bg-sky-500", style: "bg-sky-500/10 text-sky-800 dark:text-sky-200" },
  rose: { name: "Rosa", dot: "bg-rose-500", style: "bg-rose-500/10 text-rose-800 dark:text-rose-200" },
  violet: { name: "Lilás", dot: "bg-violet-500", style: "bg-violet-500/10 text-violet-800 dark:text-violet-200" },
  neutral: { name: "Neutro", dot: "bg-zinc-500", style: "bg-hover text-fg/80" },
} as const;

export type ReminderColor = keyof typeof reminderColors;
export type Reminder = { conversationId: string; text: string; color: ReminderColor };
export const reminderKey = ["conversation-reminders"] as const;
const demoReminders = new Map<string, Record<string, Reminder>>();

export function useReminders(enabled: boolean) {
  const { data: me } = useMe();
  const userId = me?.user.id ?? "";
  return useQuery({
    queryKey: [...reminderKey, userId],
    enabled: enabled && !!userId,
    queryFn: async (): Promise<Record<string, Reminder>> => {
      if (DEMO) return { ...demoReminders.get(userId) };
      const { reminders } = await get<{ reminders: Reminder[] }>("/conversation-reminders");
      return Object.fromEntries(reminders.map((r) => [r.conversationId, r]));
    },
  });
}

export function useSaveReminder() {
  const qc = useQueryClient();
  const { data: me } = useMe();
  const userId = me?.user.id ?? "";
  return useMutation({
    mutationFn: async (reminder: Reminder) => {
      if (DEMO) {
        const mine = { ...demoReminders.get(userId) };
        if (reminder.text.trim()) mine[reminder.conversationId] = { ...reminder, text: reminder.text.trim() };
        else delete mine[reminder.conversationId];
        demoReminders.set(userId, mine);
        return;
      }
      const path = `/conversations/${reminder.conversationId}/reminder`;
      if (!reminder.text.trim()) await del(path);
      else await api<void>("PUT", path, { text: reminder.text, color: reminder.color });
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: [...reminderKey, userId] }),
  });
}
