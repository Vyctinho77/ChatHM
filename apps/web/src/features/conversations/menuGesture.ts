/**
 * O elemento clicado pertence a um controle próprio (link, botão, campo) que mantém o comportamento do navegador?
 * Botões marcados com data-menu-surface (a imagem, que abre o visualizador) contam como parte da mensagem:
 * o menu de botão direito e o toque longo da mensagem valem também em cima deles.
 */
export function ownsGesture(target: { closest(selector: string): { hasAttribute(name: string): boolean } | null }, selector: string): boolean {
  const el = target.closest(selector);
  return !!el && !el.hasAttribute("data-menu-surface");
}
