import { useNavigate, useParams } from "react-router";
import { Page, PageBody, PageHeader } from "@/components/list";
import { DetailSkeleton } from "@/components/Skeleton";
import { useMe } from "@/features/auth/api";
import { useSegments } from "./api";
import { KeywordsSections } from "./KeywordsEditor";

/** Palavras-chave de uma fila para quem é da equipe dela (o administrador usa a tela de configuração da fila). */
export function SegmentKeywordsPage() {
  const { key } = useParams();
  const navigate = useNavigate();
  const { data: me } = useMe();
  const segments = useSegments();
  const segment = segments.data?.find((s) => s.key === key);
  const back = () => navigate("/filas");

  if (segments.isPending) {
    return (
      <Page>
        <PageHeader title="Palavras-chave" onBack={back} />
        <DetailSkeleton />
      </Page>
    );
  }
  if (!segment || !segment.canEdit) {
    return (
      <Page>
        <PageHeader title="Palavras-chave" onBack={back} />
        <p className="p-8 text-center text-[13px] text-muted">
          {segment ? "Só o administrador ou a equipe desta fila pode mexer nas palavras-chave." : "Fila não encontrada."}
        </p>
      </Page>
    );
  }
  return (
    <Page>
      <PageHeader title={segment.name} onBack={back} />
      <PageBody>
        <p className="px-4 pt-4 text-[13px] text-muted">
          {segment.autoInclude
            ? "Quando o cliente falar deste assunto numa conversa já em atendimento por outra pessoa, a equipe entra como participante e vê a conversa em Minhas."
            : "A chamada automática da equipe em conversas em andamento está desligada para esta fila (o administrador decide)."}
        </p>
        <KeywordsSections segment={segment} isAdmin={me?.user.role === "admin"} />
      </PageBody>
    </Page>
  );
}
