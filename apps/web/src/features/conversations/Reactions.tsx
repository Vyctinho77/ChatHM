import { memo, useEffect, useMemo, useRef, useState } from "react";
import { Flag, Hash, Lightbulb, PawPrint, Plane, Plus, Search, Smile, Trophy, UtensilsCrossed, type LucideIcon } from "lucide-react";
import { cn } from "@/lib/cn";
import { loadEmojis, recentEmojis, rememberEmoji, searchEmojis, type EmojiCategory } from "./emojiData";
import { EMOJI_FONT, ensureEmojiFont } from "./emojiFont";
import { QUICK_REACTIONS, type Reaction } from "./reactions";

/** Um emoji na fonte do ChatHM. */
export function Emoji({ char, className }: { char: string; className?: string }) {
  ensureEmojiFont();
  return (
    <span className={cn(EMOJI_FONT, className)} role="img" aria-label={char}>
      {char}
    </span>
  );
}

/** Barra de reações rápidas (👍 ❤️ 😂 😮 😢 🙏 e "+"): o emoji da reação atual fica marcado. */
export function QuickReactions({ current, onPick, onMore }: { current?: string; onPick: (emoji: string) => void; onMore: () => void }) {
  return (
    <div role="toolbar" aria-label="Reagir" className="flex items-center gap-0.5 rounded-full border border-line bg-panel px-1.5 py-1 shadow-xl">
      {QUICK_REACTIONS.map((e) => (
        <button
          key={e}
          type="button"
          onClick={() => onPick(e)}
          aria-label={`Reagir com ${e}`}
          aria-pressed={current === e}
          className={cn("flex size-9 items-center justify-center rounded-full text-[22px] leading-none transition-transform hover:scale-125 hover:bg-hover", current === e && "bg-hover")}
        >
          <Emoji char={e} />
        </button>
      ))}
      <button type="button" onClick={onMore} aria-label="Mais emojis" className="flex size-9 items-center justify-center rounded-full text-muted hover:bg-hover hover:text-fg">
        <Plus className="size-5" />
      </button>
    </div>
  );
}

const TAB_ICONS: Record<string, LucideIcon> = {
  smileys: Smile,
  animals: PawPrint,
  food: UtensilsCrossed,
  activities: Trophy,
  travel: Plane,
  objects: Lightbulb,
  symbols: Hash,
  flags: Flag,
};

const Cell = memo(function Cell({ char, label, onPick }: { char: string; label: string; onPick: (c: string) => void }) {
  return (
    <button type="button" onClick={() => onPick(char)} title={label} aria-label={label} className="flex size-9 items-center justify-center rounded-lg text-[24px] leading-none hover:bg-hover">
      <span className={EMOJI_FONT}>{char}</span>
    </button>
  );
});

/**
 * Cartela completa de emojis (a que abre no "+"): abas por assunto, busca em português ("coração", "sorrindo"), recentes primeiro.
 * Os nomes vêm da emojibase (Unicode CLDR) e carregam só ao abrir; a arte é a Noto Color Emoji.
 */
export function EmojiPicker({ onPick, className, searchLabel = "Pesquisar reação" }: { onPick: (emoji: string) => void; className?: string; searchLabel?: string }) {
  ensureEmojiFont();
  const [cats, setCats] = useState<EmojiCategory[] | null>(null);
  const [failed, setFailed] = useState(false);
  const [query, setQuery] = useState("");
  const [active, setActive] = useState("smileys");
  const [recent] = useState(recentEmojis);
  const scroller = useRef<HTMLDivElement>(null);
  const search = useRef<HTMLInputElement>(null);

  useEffect(() => {
    let alive = true;
    loadEmojis().then(
      (c) => alive && setCats(c),
      () => alive && setFailed(true),
    );
    search.current?.focus();
    return () => {
      alive = false;
    };
  }, []);

  const results = useMemo(() => (cats && query.trim() ? searchEmojis(cats, query) : null), [cats, query]);
  const pick = (c: string) => {
    rememberEmoji(c);
    onPick(c);
  };

  // A aba destacada acompanha a rolagem.
  function onScroll() {
    const box = scroller.current;
    if (!box || results) return;
    const top = box.getBoundingClientRect().top;
    let current = cats?.[0]?.key ?? "smileys";
    for (const sec of box.querySelectorAll<HTMLElement>("[data-cat]")) {
      if (sec.getBoundingClientRect().top - top <= 24) current = sec.dataset.cat ?? current;
    }
    setActive(current);
  }
  function goTo(key: string) {
    setActive(key);
    const box = scroller.current;
    const sec = box?.querySelector<HTMLElement>(`[data-cat="${key}"]`);
    if (box && sec) box.scrollTo({ top: sec.offsetTop - box.offsetTop, behavior: "smooth" });
  }

  return (
    <div className={cn("flex h-[22rem] w-[21rem] max-w-[calc(100vw-1rem)] flex-col overflow-hidden rounded-2xl border border-line bg-panel shadow-xl", className)}>
      <div role="tablist" aria-label="Categorias" className="flex shrink-0 justify-between border-b border-line px-2">
        {(cats ?? []).map((c) => {
          const Icon = TAB_ICONS[c.key] ?? Smile;
          const on = !results && active === c.key;
          return (
            <button
              key={c.key}
              type="button"
              role="tab"
              aria-selected={on}
              aria-label={c.label}
              title={c.label}
              onClick={() => goTo(c.key)}
              className={cn("flex h-10 w-9 items-center justify-center border-b-[3px] text-muted hover:text-fg", on ? "border-brand text-fg" : "border-transparent")}
            >
              <Icon className="size-5" />
            </button>
          );
        })}
      </div>
      <div className="shrink-0 px-3 pt-2.5">
        <label className="flex items-center gap-2 rounded-full border border-line bg-app px-3 py-1.5 focus-within:border-muted">
          <Search className="size-4 shrink-0 text-muted" aria-hidden />
          <input
            ref={search}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={searchLabel}
            aria-label="Pesquisar emoji"
            className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-muted"
          />
        </label>
      </div>
      <div ref={scroller} onScroll={onScroll} className="min-h-0 flex-1 overflow-y-auto px-2 pb-2 pt-1">
        {failed && <p className="p-4 text-center text-sm text-muted">Não foi possível carregar os emojis.</p>}
        {!cats && !failed && <p className="p-4 text-center text-sm text-muted">Carregando…</p>}
        {results && (results.length ? (
          <div className="grid grid-cols-7 justify-items-center">{results.map((e) => <Cell key={e.char} char={e.char} label={e.label} onPick={pick} />)}</div>
        ) : (
          <p className="p-4 text-center text-sm text-muted">Nenhum emoji encontrado.</p>
        ))}
        {cats && !results && (
          <>
            {recent.length > 0 && (
              <section>
                <h3 className="px-1 pb-1 pt-2 text-xs font-semibold text-muted">Usados recentemente</h3>
                <div className="grid grid-cols-7 justify-items-center">{recent.map((c) => <Cell key={c} char={c} label={c} onPick={pick} />)}</div>
              </section>
            )}
            {cats.map((c) => (
              <section key={c.key} data-cat={c.key} className="[contain-intrinsic-size:auto_300px] [content-visibility:auto]">
                <h3 className="px-1 pb-1 pt-2 text-xs font-semibold text-muted">{c.label}</h3>
                <div className="grid grid-cols-7 justify-items-center">{c.items.map((e) => <Cell key={e.char} char={e.char} label={e.label} onPick={pick} />)}</div>
              </section>
            ))}
          </>
        )}
      </div>
    </div>
  );
}

/** Reações embaixo do balão: um chip por emoji (com a contagem quando mais de uma pessoa reagiu). Clicar no seu tira a reação. */
export function ReactionChips({ reactions, onToggle, className }: { reactions: Reaction[]; onToggle?: (emoji: string) => void; className?: string }) {
  if (!reactions.length) return null;
  return (
    <div className={cn("flex flex-wrap gap-1", className)}>
      {reactions.map((r) => (
        <button
          key={r.emoji}
          type="button"
          onClick={onToggle ? () => onToggle(r.emoji) : undefined}
          disabled={!onToggle}
          aria-label={`${r.emoji} ${r.count > 1 ? `${r.count} reações` : "1 reação"}${r.mine ? " (sua)" : ""}`}
          aria-pressed={r.mine}
          className={cn(
            // Sem contorno (nem verde): a pílula é só o fundo do tema (claro ou escuro) com uma sombra leve; a minha fica um tom mais forte.
            "inline-flex h-[22px] items-center gap-0.5 rounded-full px-1.5 text-[13px] leading-none text-fg shadow-sm shadow-black/15",
            r.mine ? "bg-hover" : "bg-panel",
            onToggle ? "cursor-pointer hover:bg-hover" : "cursor-default",
          )}
        >
          <span className={EMOJI_FONT}>{r.emoji}</span>
          {r.count > 1 && <span className="text-[11px] tabular-nums text-muted">{r.count}</span>}
        </button>
      ))}
    </div>
  );
}
