import { useState } from "react";
import { getWritingPrefs, setWritingPrefs } from "@/features/conversations/writingPrefs";

export function WritingSettings() {
  const [prefs, setPrefs] = useState(getWritingPrefs);
  return <div className="flex flex-col gap-3">
    <label className="flex cursor-pointer items-start justify-between gap-4 text-sm">
      <span>
        <span className="block font-medium">Corrigir acentos ao digitar</span>
        <span className="mt-1 block text-xs text-muted">Ao fechar uma palavra com espaço ou pontuação, o campo de mensagem põe o acento: “voce” vira “você”, “nao” vira “não”, “orcamento” vira “orçamento”. Para desfazer uma correção, aperte Backspace logo em seguida. A mudança vale na próxima conversa que você abrir.</span>
      </span>
      <input type="checkbox" className="mt-1 size-4 shrink-0" checked={prefs.autoAccent} onChange={(e) => setPrefs(setWritingPrefs({ autoAccent: e.target.checked }))} />
    </label>
    <p className="text-xs text-muted">O navegador também sublinha em vermelho palavras que não conhece; clique com o botão direito para ver as sugestões.</p>
  </div>;
}
