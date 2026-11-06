# Ícones do ChatHM

Inventário de 70 ícones genéricos distintos usados no código atual. A prioridade indica a ordem sugerida de criação, não a importância funcional.

## Padrão dos arquivos

- SVG vetorial, um arquivo por ícone, com `viewBox="0 0 24 24"`.
- Ícones desenhados em uma grade de 24 × 24 px; conferir legibilidade também a 16, 18 e 20 px.
- Estilo de contorno uniforme, traço de 1,75–2 px, pontas e junções arredondadas.
- Usar `stroke="currentColor"` e `fill="none"` nos ícones de contorno. Quando houver preenchimento intencional, usar `fill="currentColor"`.
- Fundo transparente, sem margens externas extras, sombras, gradientes ou texto incorporado.
- Cor controlada pelo app: preto no claro, branco no escuro e verde #8B9D40 nos destaques. Tons de contraste e cores de erro são definidos pelo tema.
- Um único SVG serve para claro/escuro, ativo/inativo e verde/vermelho; não exportar um arquivo por cor.
- Conferir alinhamento visual e peso consistente entre ícones de formatos diferentes.

## Tabela de criação

| Arquivo sugerido | Uso no app | Ícone atual (Lucide) | Prioridade |
|---|---|---|---|
| `conversas.svg` | Conversas no menu e atalhos de conversa | `MessageSquareText` | Alta |
| `filas.svg` | Filas e segmentos | `Layers` | Alta |
| `clientes.svg` | Clientes no menu | `Contact` | Alta |
| `agenda.svg` | Agenda e datas | `CalendarDays` | Alta |
| `etiquetas.svg` | Etiquetas | `Tag` | Alta |
| `equipe.svg` | Equipe no menu | `UsersRound` | Alta |
| `configuracoes.svg` | Configurações | `Settings` | Alta |
| `buscar.svg` | Busca de clientes e mensagens | `Search` | Média |
| `nova-conversa.svg` | Iniciar uma conversa | `MessageSquarePlus` | Média |
| `escrever-conversa.svg` | Criar conversa pela lista | `SquarePen` | Média |
| `enviar.svg` | Enviar mensagem | `SendHorizontal` | Média |
| `anexar.svg` | Anexar arquivo | `Paperclip` | Média |
| `imagem.svg` | Imagens e fotos | `Image` | Média |
| `documento.svg` | Documentos e PDFs | `FileText` | Média |
| `microfone.svg` | Gravar áudio | `Mic` | Média |
| `parar.svg` | Parar gravação | `Square` | Média |
| `reproduzir.svg` | Reproduzir áudio | `Play` | Média |
| `pausar.svg` | Pausar áudio | `Pause` | Média |
| `emoji.svg` | Abrir emojis | `Smile` | Média |
| `imagem-hd.svg` | Enviar imagem em alta definição | `Sparkles` | Média |
| `localizacao.svg` | Enviar e visualizar localização | `MapPin` | Média |
| `telefone.svg` | Telefone do contato | `Phone` | Média |
| `empresa.svg` | Empresa do contato | `Building2` | Média |
| `pessoa.svg` | Contato e perfil de uma pessoa | `UserRound` | Média |
| `assumir-transferir.svg` | Assumir ou transferir atendimento | `UserRoundPlus` | Média |
| `concluir-atendimento.svg` | Concluir atendimento | `CircleCheck` | Média |
| `marcacao.svg` | Confirmação e mensagem enviada: um visto | `Check` | Média |
| `marcacao-dupla.svg` | Mensagem entregue/lida: dois vistos | `CheckCheck` | Média |
| `relogio.svg` | Horário e mensagem pendente | `Clock` | Média |
| `falha-envio.svg` | Falha ao enviar mensagem | `AlertCircle` | Média |
| `bloqueado.svg` | Mídia bloqueada ou conteúdo indisponível | `Ban` | Média |
| `painel-contato.svg` | Abrir ou fechar detalhes do contato | `PanelRight` | Média |
| `baixar.svg` | Baixar mídia ou documento | `Download` | Média |
| `abrir-externo.svg` | Abrir localização ou link externo | `ExternalLink` | Média |
| `mais-opcoes.svg` | Menu de ações adicionais | `EllipsisVertical` | Média |
| `filtrar-lista.svg` | Filtro da lista de conversas | `ListFilter` | Média |
| `ajustar-filtros.svg` | Ajustar filtros das filas | `SlidersHorizontal` | Média |
| `zoom-mais.svg` | Ampliar imagem ou PDF | `ZoomIn` | Média |
| `zoom-menos.svg` | Reduzir imagem ou PDF | `ZoomOut` | Média |
| `ir-ao-final.svg` | Ir às mensagens mais recentes | `ArrowDown` | Média |
| `voltar.svg` | Voltar à tela anterior | `ArrowLeft` | Baixa |
| `anterior.svg` | Mês ou imagem anterior | `ChevronLeft` | Baixa |
| `proximo.svg` | Próximo mês, imagem ou tela | `ChevronRight` | Baixa |
| `expandir.svg` | Expandir ou próximo resultado abaixo | `ChevronDown` | Baixa |
| `recolher.svg` | Recolher ou resultado acima | `ChevronUp` | Baixa |
| `alternar-menu.svg` | Alternar menu do usuário | `ChevronsUpDown` | Baixa |
| `menu.svg` | Abrir menu no celular | `Menu` | Baixa |
| `fechar.svg` | Fechar painel, modal ou visualizador | `X` | Baixa |
| `adicionar.svg` | Adicionar item, etiqueta ou equipe | `Plus` | Baixa |
| `editar.svg` | Editar cadastro ou etiqueta | `Pencil` | Baixa |
| `excluir.svg` | Excluir item ou gravação | `Trash2` | Baixa |
| `copiar.svg` | Copiar código, link ou texto | `Copy` | Baixa |
| `atualizar.svg` | Atualizar ou gerar novamente | `RefreshCw` | Baixa |
| `carregando.svg` | Carregamento; prever rotação | `LoaderCircle` | Baixa |
| `mostrar.svg` | Mostrar informação protegida | `Eye` | Baixa |
| `ocultar.svg` | Ocultar informação protegida | `EyeOff` | Baixa |
| `tema-claro.svg` | Selecionar tema claro | `Sun` | Baixa |
| `tema-escuro.svg` | Selecionar tema escuro | `Moon` | Baixa |
| `tema-sistema.svg` | Seguir o tema do sistema | `Monitor` | Baixa |
| `seguranca-ativa.svg` | Autenticador ou proteção ativos | `ShieldCheck` | Baixa |
| `seguranca-inativa.svg` | Autenticador ou proteção desativados | `ShieldOff` | Baixa |
| `seguranca-aviso.svg` | Orientação sobre segurança da conexão | `ShieldAlert` | Baixa |
| `chave.svg` | Chaves, senhas e códigos de acesso | `KeyRound` | Baixa |
| `celular.svg` | Celular/autenticador na gestão de usuários | `Smartphone` | Baixa |
| `adicionar-usuario.svg` | Convidar ou adicionar integrante | `UserPlus` | Baixa |
| `grupo.svg` | Equipe nos detalhes de um compromisso | `Users` | Baixa |
| `informacao.svg` | Mensagem informativa | `Info` | Baixa |
| `aviso.svg` | Alerta ou confirmação de risco | `TriangleAlert` | Baixa |
| `etapa-pendente.svg` | Etapa de configuração não concluída | `Circle` | Baixa |
| `sair.svg` | Sair da conta | `LogOut` | Baixa |

## Identidade e marcas existentes

| Recurso | Ação |
|---|---|
| Logotipo ChatHM | Já fornecido pelo usuário e integrado; manter os arquivos claro e escuro. |
| Símbolo do WhatsApp | Já integrado como símbolo de marca; manter sua forma, sem redesenhar como ícone genérico. O app aplica verde quando conectado e vermelho quando desconectado. |
| Favicon/ícone do ChatHM | Já extraído do logotipo; manter. |

## Estados especiais

- **WhatsApp:** uma forma, estados verde/conectado e vermelho/desconectado definidos pelo app.
- **Mensagens:** preservar um visto (enviada), dois vistos (entregue), dois vistos com o destaque de leitura (lida), relógio (pendente) e círculo de erro (falha).
- **Carregamento:** o SVG deve permitir animação de rotação pelo app.
- **Reprodução e visibilidade:** entregar arquivos separados para reproduzir/pausar e mostrar/ocultar, pois a forma muda.
- **Acessibilidade:** os nomes acessíveis vêm dos botões e controles; não colocar textos ou avisos dentro do desenho.
