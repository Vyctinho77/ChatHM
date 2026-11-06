import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { del, get, patch, post, put } from "@/lib/api";
import type { Role } from "@/features/auth/api";

export type User = {
  id: string;
  email: string;
  name: string;
  role: Role;
  isActive: boolean;
  mfaEnabled: boolean;
  lastLoginAt: string | null;
  createdAt: string;
  teams: { id: string; name: string }[];
};

export type Team = { id: string; name: string; description: string; memberCount: number; createdAt: string };

export type Member = { userId: string; name: string; email: string; role: Role; isActive: boolean; joinedAt: string };

export const teamKeys = {
  users: ["users"] as const,
  teams: ["teams"] as const,
  members: (id: string) => ["teams", id, "members"] as const,
};

export function useUsers(enabled: boolean) {
  return useQuery({
    queryKey: teamKeys.users,
    enabled,
    queryFn: async () => (await get<{ users: User[] }>("/users")).users,
  });
}

export function useTeams() {
  return useQuery({ queryKey: teamKeys.teams, queryFn: async () => (await get<{ teams: Team[] }>("/teams")).teams });
}

export function useMembers(teamId: string) {
  return useQuery({
    queryKey: teamKeys.members(teamId),
    queryFn: async () => (await get<{ members: Member[] }>(`/teams/${teamId}/members`)).members,
  });
}

function useInvalidateAll() {
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: teamKeys.users });
    void qc.invalidateQueries({ queryKey: teamKeys.teams });
  };
}

export type CreateUserInput = { email: string; name: string; password: string; role: Role; teamIds: string[] };

export function useCreateUser() {
  const done = useInvalidateAll();
  return useMutation({ mutationFn: (i: CreateUserInput) => post<User>("/users", i), onSuccess: done });
}

export function useUpdateUser() {
  const done = useInvalidateAll();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string; name?: string; role?: Role; isActive?: boolean }) =>
      patch<User>(`/users/${id}`, body),
    onSuccess: done,
  });
}

export function useResetPassword() {
  return useMutation({
    mutationFn: ({ id, password }: { id: string; password: string }) => post<void>(`/users/${id}/password`, { password }),
  });
}

export function useCreateTeam() {
  const done = useInvalidateAll();
  return useMutation({
    mutationFn: (i: { name: string; description: string }) => post<Team>("/teams", i),
    onSuccess: done,
  });
}

export function useDeleteTeam() {
  const done = useInvalidateAll();
  return useMutation({ mutationFn: (id: string) => del<void>(`/teams/${id}`), onSuccess: done });
}

/** Aplica a diferença entre as equipes atuais e as desejadas de um usuário. */
export function useSetUserTeams() {
  const done = useInvalidateAll();
  return useMutation({
    mutationFn: async ({ userId, current, next }: { userId: string; current: string[]; next: string[] }) => {
      const add = next.filter((t) => !current.includes(t));
      const remove = current.filter((t) => !next.includes(t));
      await Promise.all([
        ...add.map((t) => put<void>(`/teams/${t}/members/${userId}`)),
        ...remove.map((t) => del<void>(`/teams/${t}/members/${userId}`)),
      ]);
    },
    onSuccess: done,
  });
}

/** Pessoa do escritório (visível a qualquer usuário autenticado; sem e-mail). */
export type Person = { id: string; name: string; role: Role; teams: string[] };

export function usePeople() {
  return useQuery({
    queryKey: ["people"] as const,
    staleTime: 60_000,
    queryFn: async () => (await get<{ people: Person[] }>("/people")).people,
  });
}

export type InviteResult = { user: User; inviteCode: string; expiresAt: string };
export type InviteInput = { email: string; name: string; role: Role; teamIds: string[] };

export function useInviteUser() {
  const done = useInvalidateAll();
  return useMutation({ mutationFn: (i: InviteInput) => post<InviteResult>("/users/invite", i), onSuccess: done });
}

/** Novo código de acesso; redefine o autenticador da pessoa (celular perdido ou trocado). */
export function useReinviteUser() {
  const done = useInvalidateAll();
  return useMutation({ mutationFn: (id: string) => post<InviteResult>(`/users/${id}/invite`), onSuccess: done });
}
