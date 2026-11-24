// Fonte de emojis Noto Color Emoji (Google, licença SIL OFL 1.1), servida pelo próprio ChatHM (a política de segurança só aceita fontes do próprio site).
//
// É o arquivo COMPLETO do pacote @fontsource/noto-color-emoji (5,8 MB), não os 11 pedaços por faixa de caracteres: conferido num navegador, os pedaços
// deixam ~400 dos 1.932 emojis em branco (👍 ❤️ 😐 ✋ ✌️...), e o arquivo completo desenha todos. O navegador só baixa a fonte quando um emoji
// com a classe "emoji-font" aparece na tela, e a guarda por um ano (nome com hash).
import fullFont from "@fontsource/noto-color-emoji/files/noto-color-emoji-emoji-400-normal.woff2?url";

let injected = false;

/** Registra a fonte de emojis (uma vez). Não baixa nada até aparecer um emoji que a use. */
export function ensureEmojiFont() {
  if (injected || typeof document === "undefined") return;
  injected = true;
  const style = document.createElement("style");
  style.dataset.chathm = "emoji-font";
  style.textContent = `@font-face{font-family:"Noto Color Emoji";font-style:normal;font-weight:400;font-display:swap;src:url(${fullFont}) format("woff2")}`;
  document.head.appendChild(style);
}

/** Classe CSS (index.css) para o texto que deve usar a fonte de emojis do ChatHM; cai no emoji do sistema enquanto carrega. */
export const EMOJI_FONT = "emoji-font";
