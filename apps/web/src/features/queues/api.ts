import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, del, get, patch, post } from "@/lib/api";
import type { SegmentOption } from "@/features/conversations/types";

export type Keyword = { id: string; phrase: string; priority: number };

export type Segment = {
  id: string;
  key: string;
  name: string;
  teamId: string | null;
  teamName?: string;
  isActive: boolean;
  /** Só para administradores. */
  keywords?: Keyword[];
};

export type ClassifyResult = {
  segmentId?: string;
  segmentKey?: string;
  reason: "keyword" | "conflict" | "none";
  matches: { segmentId: string; segmentKey: string; phrase: string; priority: number }[];
};

export const segmentsKey = ["segments"] as const;
const fetchSegments = async () => (await get<{ segments: Segment[] }>("/segments")).segments;

export function useSegments() {
  // Sempre busca a configuração atual ao abrir a tela: o admin pode ter mudado vínculos de equipe.
  return useQuery({ queryKey: segmentsKey, queryFn: fetchSegments, staleTime: 30_000, refetchOnMount: "always" });
}

/** Segmentos para os filtros da lista de conversas (mesma consulta de useSegments). */
export function useSegmentOptions() {
  return useQuery({
    queryKey: segmentsKey,
    queryFn: fetchSegments,
    select: (segs): SegmentOption[] => segs.map(({ key, name }) => ({ key, name })),
    staleTime: 30_000,
  });
}

function useSegmentMutation<V, R = unknown>(fn: (v: V) => Promise<R>) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: segmentsKey });
      // Mudar vínculos ou excluir um segmento altera quem enxerga as conversas.
      void qc.invalidateQueries({ queryKey: ["conversations"] });
    },
  });
}

/** Importa os segmentos de exemplo (não sobrescreve o que o gestor já tem). */
export const useImportExamples = () => useSegmentMutation(() => post<void>("/segments/examples"));
export const useCreateSegment = () =>
  useSegmentMutation((v: { name: string; teamId: string | null }) => post<Segment>("/segments", v));
export const useUpdateSegment = () =>
  useSegmentMutation(({ id, ...body }: { id: string; name?: string; isActive?: boolean }) => patch<Segment>(`/segments/${id}`, body));
export const useSetSegmentTeam = () =>
  useSegmentMutation(({ id, teamId }: { id: string; teamId: string | null }) =>
    api<Segment>("PUT", `/segments/${id}/team`, { teamId }),
  );
export const useDeleteSegment = () => useSegmentMutation((id: string) => del<void>(`/segments/${id}`));
export const useAddKeyword = () =>
  useSegmentMutation(({ segmentId, ...body }: { segmentId: string; phrase: string; priority: number }) =>
    post<Keyword>(`/segments/${segmentId}/keywords`, body),
  );
export const useUpdateKeyword = () =>
  useSegmentMutation(({ segmentId, id, priority }: { segmentId: string; id: string; priority: number }) =>
    patch<void>(`/segments/${segmentId}/keywords/${id}`, { priority }),
  );
export const useDeleteKeyword = () =>
  useSegmentMutation(({ segmentId, id }: { segmentId: string; id: string }) => del<void>(`/segments/${segmentId}/keywords/${id}`));

export function useClassifyText() {
  return useMutation({ mutationFn: (text: string) => post<ClassifyResult>("/segments/classify", { text }) });
}
