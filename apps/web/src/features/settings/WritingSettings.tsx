import { useState } from "react";
import { X } from "lucide-react";
import { Button } from "@/components/ui";
import { ignoreWord, setWritingPrefs, unignoreWord, useWritingPrefs } from "@/features/conversations/writingPrefs";

export function WritingSettings() {
  const prefs = useWritingPrefs();
  const [word, setWord] = useState("");
  return <div className="flex flex-col gap-5">
    <label className="flex cursor-pointer items-start justify-between gap-4 text-sm">
      <span>
        <span className="block font-medium">Corrigir acentos ao digitar</span>
        <span className="mt-1 block text-xs text-muted">Ao fechar uma palavra com espaço ou pontuação, o campo põe o acento: “voce” vira “você”, “nao” vira “não”, “orcamento” vira “orçamento”. Vale na mensagem, na legenda dos anexos, nos lembretes e na observação do cliente. Para desfazer uma correção, aperte Backspace logo em seguida.</span>
      </span>
      <input type="checkbox" className="mt-1 size-4 shrink-0" checked={prefs.autoAccent} onChange={(e) => setWritingPrefs({ autoAccent: e.target.checked })} />
    </label>
    <label className="flex cursor-pointer items-start justify-between gap-4 text-sm">
      <span>
        <span className="block font-medium">Expandir abreviações de chat</span>
        <span className="mt-1 block text-xs text-muted">“vc” vira “você”, “tb” vira “também”, “obg” vira “obrigado”, “blz” vira “beleza”, “hj” vira “hoje”, “pfv” vira “por favor” e outras. Desligado por padrão: o cliente vê a mensagem como ficou.</span>
      </span>
      <input type="checkbox" className="mt-1 size-4 shrink-0" checked={prefs.abbreviations} onChange={(e) => setWritingPrefs({ abbreviations: e.target.checked })} />
    </label>
    <div>
      <p className="text-sm font-medium">Palavras que não corrijo</p>
      <p className="mt-1 text-xs text-muted">Nomes, siglas e termos do escritório. Se você desfaz a correção de uma palavra duas vezes, ela vem para cá sozinha.</p>
      <form className="mt-3 flex gap-2" onSubmit={(e) => { e.preventDefault(); ignoreWord(word); setWord(""); }}>
        <input value={word} onChange={(e) => setWord(e.target.value)} maxLength={40} placeholder="Ex.: marcio" aria-label="Palavra a não corrigir" className="h-9 min-w-0 flex-1 rounded-lg border border-line bg-elevated px-3 text-sm outline-none focus:border-brand-ink" />
        <Button type="submit" variant="secondary" disabled={!/^\p{L}+$/u.test(word.trim())}>Adicionar</Button>
      </form>
      {prefs.ignored.length > 0 ? (
        <ul className="mt-3 flex flex-wrap gap-2" aria-label="Palavras ignoradas">
          {prefs.ignored.map((w) => (
            <li key={w} className="inline-flex items-center gap-1 rounded-full bg-elevated py-1 pl-3 pr-1 text-xs">
              {w}
              <button type="button" aria-label={`Voltar a corrigir ${w}`} onClick={() => unignoreWord(w)} className="rounded-full p-1 text-muted hover:bg-hover hover:text-fg"><X className="size-3" /></button>
            </li>
          ))}
        </ul>
      ) : <p className="mt-3 text-xs text-muted">Nenhuma palavra por aqui.</p>}
    </div>
    <p className="text-xs text-muted">O navegador também sublinha em vermelho palavras que não conhece; clique com o botão direito para ver as sugestões.</p>
  </div>;
}
