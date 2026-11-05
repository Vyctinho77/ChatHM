import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { del, get, patch, post } from "@/lib/api";
import type { TagInfo } from "@/features/conversations/types";

export const tagsKey = ["tags"] as const;

/** Catálogo de etiquetas (usado também para aplicar etiquetas nas conversas). */
export function useTagCatalog() {
  return useQuery({
    queryKey: tagsKey,
    queryFn: async () => (await get<{ tags: TagInfo[] }>("/tags")).tags,
    staleTime: 60_000,
  });
}

function useTagMutation<V, R = unknown>(fn: (v: V) => Promise<R>) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: tagsKey });
      void qc.invalidateQueries({ queryKey: ["conversations"] });
    },
  });
}

export const useCreateTag = () => useTagMutation((name: string) => post<TagInfo>("/tags", { name }));
export const useRenameTag = () => useTagMutation(({ id, name }: { id: string; name: string }) => patch<TagInfo>(`/tags/${id}`, { name }));
export const useDeleteTag = () => useTagMutation((id: string) => del<void>(`/tags/${id}`));
