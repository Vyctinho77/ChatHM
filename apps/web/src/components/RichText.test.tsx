import { describe, expect, it, vi } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { RichText } from "./RichText";
import { FormattedText } from "./FormattedText";

vi.mock("@/features/conversations/store", () => ({ useContactOpener: () => ({ openPhone: vi.fn() }) }));

const render = (text: string, term?: string) => renderToStaticMarkup(<MemoryRouter><RichText text={text} term={term} /></MemoryRouter>);

describe("formatted chat text", () => {
  it("renders semantic formatting and preserves the search highlight", () => {
    const html = render("*Olá José* e _bom dia_", "José");
    expect(html).toContain('<strong class="font-bold">');
    expect(html).toContain('>José</mark>');
    expect(html).toContain('<em class="italic">');
    expect(html).toContain('bom dia');
    expect(html).not.toContain("*Olá");
  });

  it("keeps formatted URLs, emails and phones interactive without changing the address", () => {
    const html = render("*https://exemplo.com/a_b* _nome_sobrenome@empresa.com_ *(92) 98800-1111*");
    expect(html).toContain('href="https://exemplo.com/a_b"');
    expect(html).toContain('href="mailto:nome_sobrenome@empresa.com"');
    expect(html).toContain('title="Abrir conversa com este número"');
    expect(html).toContain('rel="noopener noreferrer"');
  });

  it("renders HTML as text and never creates executable content", () => {
    const html = render('*<img src=x onerror="alert(1)">*');
    expect(html).toContain("&lt;img");
    expect(html).not.toContain("<img");
    expect(html).not.toContain("<script");
  });

  it("formats quoted previews without nesting interactive elements", () => {
    const html = renderToStaticMarkup(<button><FormattedText text="*Guia disponível* em _https://exemplo.com/_" /></button>);
    expect(html).toContain('<strong class="font-bold">Guia disponível</strong>');
    expect(html).not.toContain("<a ");
    expect(html.match(/<button/g)).toHaveLength(1);
  });
});
